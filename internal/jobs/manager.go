package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"yt-web-downloader/internal/config"
	"yt-web-downloader/internal/ytdlp"
)

const queueCap = 100

// Manager owns the in-memory job registry and the work queue consumed by
// worker goroutines. It is the sole source of state for the package: jobs
// live in memory plus a manifest.json under <DataDir>/jobs/<id>/, rebuilt
// on startup via Rebuild.
type Manager struct {
	cfg *config.Config

	mu   sync.Mutex
	jobs map[string]*Job

	queue chan *Job
}

// NewManager creates a Manager with an empty registry and a queue of
// capacity 100.
func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		cfg:   cfg,
		jobs:  make(map[string]*Job),
		queue: make(chan *Job, queueCap),
	}
}

// JobsDir returns <DataDir>/jobs.
func (m *Manager) JobsDir() string {
	return filepath.Join(m.cfg.DataDir, "jobs")
}

// Rebuild scans JobsDir for job directories and loads any with a readable
// manifest.json into the registry. Directories without a loadable manifest
// (e.g. still in progress when the process died, or garbage) are ignored.
func (m *Manager) Rebuild() error {
	dir := m.JobsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		jdir := filepath.Join(dir, e.Name())
		man, err := loadManifest(jdir)
		if err != nil {
			continue
		}
		state := Failed
		if man.Status == "done" {
			state = Done
		}
		j := &Job{
			ID:         man.ID,
			Opts:       ytdlp.Options{URL: man.URL, Mode: man.Mode},
			Title:      man.Title,
			Dir:        jdir,
			CreatedAt:  man.CreatedAt,
			State:      state,
			Err:        man.Error,
			FinishedAt: man.FinishedAt,
			Files:      man.Files,
		}
		m.jobs[j.ID] = j
	}
	return nil
}

// Start spawns cfg.MaxConcurrentJobs worker goroutines that pull jobs off
// the queue and run them. Workers exit when ctx is cancelled or the queue
// channel is closed.
func (m *Manager) Start(ctx context.Context) {
	for i := 0; i < m.cfg.MaxConcurrentJobs; i++ {
		go m.worker(ctx)
	}
}

func (m *Manager) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case j, ok := <-m.queue:
			if !ok {
				return
			}
			state, _, _, _ := j.Snapshot()
			if state != Queued {
				// job was deleted (or otherwise moved on) while queued
				continue
			}
			m.runJob(ctx, j)
		}
	}
}

// Create allocates a new job directory, registers the job, and enqueues it
// for processing. The job is added to the registry before the (non-
// blocking) enqueue attempt; if the queue is full an error is returned.
func (m *Manager) Create(opts ytdlp.Options, title string) (*Job, error) {
	idBytes := make([]byte, 8)
	if _, err := rand.Read(idBytes); err != nil {
		return nil, err
	}
	id := hex.EncodeToString(idBytes)

	dir := filepath.Join(m.JobsDir(), id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	j := &Job{
		ID:        id,
		Opts:      opts,
		Title:     title,
		Dir:       dir,
		CreatedAt: time.Now(),
		State:     Queued,
	}

	m.mu.Lock()
	m.jobs[id] = j
	m.mu.Unlock()

	select {
	case m.queue <- j:
	default:
		return nil, fmt.Errorf("job queue is full")
	}

	return j, nil
}

// Get returns the job with the given id, if known.
func (m *Manager) Get(id string) (*Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	return j, ok
}

// List returns all known jobs sorted by CreatedAt descending.
func (m *Manager) List() []*Job {
	m.mu.Lock()
	out := make([]*Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, j)
	}
	m.mu.Unlock()

	sort.Slice(out, func(a, b int) bool {
		return out[a].CreatedAt.After(out[b].CreatedAt)
	})
	return out
}

// Known reports whether id is present in the registry.
func (m *Manager) Known(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.jobs[id]
	return ok
}

// Delete removes a job: if running, it cancels the job and waits (up to
// 10s, polling every 200ms) for it to reach a terminal state; if queued, it
// marks the job Failed so the worker skips it on dequeue. The job directory
// is then removed (retried, since Windows may briefly hold file locks) and
// the job is dropped from the registry.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	j, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("job %q not found", id)
	}

	state, _, _, _ := j.Snapshot()
	switch state {
	case Queued:
		j.setState(Failed, "deleted")
	case Running:
		j.mu.Lock()
		cancel := j.cancel
		j.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		deadline := time.Now().Add(10 * time.Second)
		for {
			st, _, _, _ := j.Snapshot()
			if st == Done || st == Failed {
				break
			}
			if time.Now().After(deadline) {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
	}

	var rmErr error
	for attempt := 0; attempt < 3; attempt++ {
		rmErr = os.RemoveAll(j.Dir)
		if rmErr == nil {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	m.mu.Lock()
	delete(m.jobs, id)
	m.mu.Unlock()

	return rmErr
}
