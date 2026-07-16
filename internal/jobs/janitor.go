package jobs

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

func shouldDelete(known bool, st State, finishedAt time.Time, ttl time.Duration, now time.Time) bool {
	if !known {
		return true
	}
	if st != Done && st != Failed {
		return false
	}
	return now.Sub(finishedAt) > ttl
}

// RunJanitor deletes expired job dirs and orphan dirs every minute until ctx is done.
func (m *Manager) RunJanitor(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.sweep(time.Now())
		}
	}
}

func (m *Manager) sweep(now time.Time) {
	entries, err := os.ReadDir(m.JobsDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		j, known := m.Get(id)
		var st State
		var fin time.Time
		if known {
			st, _, fin, _ = j.Snapshot()
		}
		if shouldDelete(known, st, fin, m.cfg.FileTTL, now) {
			if known {
				_ = m.Delete(id)
			} else {
				_ = os.RemoveAll(filepath.Join(m.JobsDir(), id))
			}
		}
	}
}
