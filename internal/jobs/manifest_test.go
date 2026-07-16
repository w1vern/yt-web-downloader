package jobs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"yt-web-downloader/internal/config"
)

func TestManifestRoundtrip(t *testing.T) {
	dir := t.TempDir()
	m := Manifest{ID: "ab12", URL: "u", Title: "T", Mode: "audio", Status: "done",
		Files: []FileInfo{{Name: "a.ogg", Size: 42}}, CreatedAt: time.Now(), FinishedAt: time.Now()}
	if err := writeManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	got, err := loadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "ab12" || len(got.Files) != 1 || got.Files[0].Size != 42 {
		t.Errorf("bad manifest: %+v", got)
	}
}

func TestRebuild(t *testing.T) {
	data := t.TempDir()
	cfg := &config.Config{DataDir: data, MaxConcurrentJobs: 1, FileTTL: time.Hour}
	mgr := NewManager(cfg)
	jdir := filepath.Join(mgr.JobsDir(), "dead01")
	os.MkdirAll(jdir, 0o755)
	writeManifest(jdir, Manifest{ID: "dead01", Title: "Old", Status: "done", FinishedAt: time.Now()})
	os.MkdirAll(filepath.Join(mgr.JobsDir(), "orphan"), 0o755) // no manifest → ignored by Rebuild

	if err := mgr.Rebuild(); err != nil {
		t.Fatal(err)
	}
	j, ok := mgr.Get("dead01")
	if !ok || j.State != Done || j.Title != "Old" {
		t.Fatalf("rebuild failed: %+v ok=%v", j, ok)
	}
	if mgr.Known("orphan") {
		t.Error("orphan dir must not enter registry")
	}
}
