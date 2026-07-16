package jobs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Manifest is the on-disk record of a finished (or errored) job, persisted
// as <jobDir>/manifest.json. It is the sole source of truth used to rebuild
// the in-memory registry on startup.
type Manifest struct {
	ID         string     `json:"id"`
	URL        string     `json:"url"`
	Title      string     `json:"title"`
	Mode       string     `json:"mode"`
	Status     string     `json:"status"` // "done"|"error"
	Error      string     `json:"error,omitempty"`
	Files      []FileInfo `json:"files"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt time.Time  `json:"finished_at"`
}

const manifestFileName = "manifest.json"

// writeManifest atomically persists m to dir/manifest.json via a temp file
// + rename, so readers never observe a partially written file.
func writeManifest(dir string, m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	final := filepath.Join(dir, manifestFileName)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

// loadManifest reads and parses dir/manifest.json.
func loadManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, manifestFileName))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
