package jobs

import (
	"context"
	"sync"
	"time"

	"yt-web-downloader/internal/ytdlp"
)

// State is the lifecycle state of a Job.
type State string

const (
	Queued  State = "queued"
	Running State = "running"
	Done    State = "done"
	Failed  State = "error"
)

// FileInfo describes one output file produced by a job.
type FileInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

const logBufCap = 300
const subChanCap = 64

// Job represents a single download job, in-memory state plus pub/sub for
// live progress events.
type Job struct {
	ID        string
	Opts      ytdlp.Options
	Title     string
	Dir       string // absolute path of job dir
	CreatedAt time.Time

	mu sync.Mutex
	// guarded by mu:
	State         State
	Err           string
	FinishedAt    time.Time
	Files         []FileInfo
	PlaylistDates map[string]string

	logBuf []string
	subs   map[chan ytdlp.Event]struct{}
	cancel context.CancelFunc
}

// publish appends log lines to the ring buffer and fans the event out to all
// subscribers without blocking; a subscriber with a full buffer misses the
// event.
func (j *Job) publish(ev ytdlp.Event) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if ev.Type == "log" {
		j.logBuf = append(j.logBuf, ev.Line)
		if over := len(j.logBuf) - logBufCap; over > 0 {
			j.logBuf = j.logBuf[over:]
		}
	}

	for ch := range j.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// Subscribe registers a new listener for job events. The returned cancel
// func unsubscribes and closes the channel; it is safe to call multiple
// times.
func (j *Job) Subscribe() (<-chan ytdlp.Event, func()) {
	ch := make(chan ytdlp.Event, subChanCap)

	j.mu.Lock()
	if j.subs == nil {
		j.subs = make(map[chan ytdlp.Event]struct{})
	}
	j.subs[ch] = struct{}{}
	j.mu.Unlock()

	var once sync.Once
	cancelFn := func() {
		once.Do(func() {
			j.mu.Lock()
			delete(j.subs, ch)
			j.mu.Unlock()
			close(ch)
		})
	}
	return ch, cancelFn
}

// LogTail returns a snapshot copy of the log ring buffer.
func (j *Job) LogTail() []string {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]string, len(j.logBuf))
	copy(out, j.logBuf)
	return out
}

// setState updates the job's state/error (and FinishedAt for terminal
// states), then publishes a "state" event to subscribers.
func (j *Job) setState(s State, errMsg string) {
	j.mu.Lock()
	j.State = s
	j.Err = errMsg
	if s == Done || s == Failed {
		j.FinishedAt = time.Now()
	}
	j.mu.Unlock()

	j.publish(ytdlp.Event{Type: "state", State: string(s), Error: errMsg})
}

// Snapshot returns a mutex-guarded read of the mutable job fields.
func (j *Job) Snapshot() (State, string, time.Time, []FileInfo) {
	j.mu.Lock()
	defer j.mu.Unlock()
	files := make([]FileInfo, len(j.Files))
	copy(files, j.Files)
	return j.State, j.Err, j.FinishedAt, files
}
