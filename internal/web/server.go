package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path/filepath"

	"yt-web-downloader/internal/auth"
	"yt-web-downloader/internal/config"
	"yt-web-downloader/internal/jobs"
)

// brandingFiles are fixed, git-ignored paths under DataDir/branding that an
// operator can drop images into by hand (no rebuild/redeploy needed). Serving
// them straight off disk (rather than embedding) lets any of them be absent;
// http.ServeFile 404s and the frontend falls back to default styling/icons.
var brandingFiles = map[string]string{
	"/branding/background.svg":       "background.svg",
	"/branding/favicon.svg":          "favicon.svg",
	"/branding/apple-touch-icon.png": "apple-touch-icon.png",
}

//go:embed static
var staticFS embed.FS

type Server struct {
	cfg *config.Config
	mgr *jobs.Manager
}

func New(cfg *config.Config, mgr *jobs.Manager) http.Handler {
	s := &Server{cfg: cfg, mgr: mgr}
	secret := []byte(cfg.SessionSecret)

	api := http.NewServeMux()
	api.HandleFunc("GET /api/config", s.handleConfig)
	api.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	api.HandleFunc("GET /api/resolve", s.handleResolve)
	api.HandleFunc("POST /api/jobs", s.handleCreateJob)
	api.HandleFunc("GET /api/jobs", s.handleListJobs)
	api.HandleFunc("GET /api/jobs/{id}", s.handleGetJob)
	api.HandleFunc("GET /api/jobs/{id}/events", s.handleEvents)
	api.HandleFunc("GET /api/jobs/{id}/files/{name}", s.handleFile)
	api.HandleFunc("GET /api/jobs/{id}/zip", s.handleZip)
	api.HandleFunc("DELETE /api/jobs/{id}", s.handleDeleteJob)

	root := http.NewServeMux()
	root.HandleFunc("POST /api/login", s.handleLogin)
	root.HandleFunc("POST /api/logout", s.handleLogout)
	root.Handle("/api/", auth.Middleware(secret, api))
	for route, file := range brandingFiles {
		root.HandleFunc("GET "+route, s.handleBranding(file))
	}
	static, _ := fs.Sub(staticFS, "static")
	root.Handle("/", http.FileServer(http.FS(static)))
	return root
}

func (s *Server) handleBranding(file string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(s.cfg.DataDir, "branding", file))
	}
}
