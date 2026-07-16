package web

import (
	"embed"
	"io/fs"
	"net/http"

	"yt-web-downloader/internal/auth"
	"yt-web-downloader/internal/config"
	"yt-web-downloader/internal/jobs"
)

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
	static, _ := fs.Sub(staticFS, "static")
	root.Handle("/", http.FileServer(http.FS(static)))
	return root
}
