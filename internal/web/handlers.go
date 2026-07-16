package web

import (
	"archive/zip"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"yt-web-downloader/internal/auth"
	"yt-web-downloader/internal/jobs"
	"yt-web-downloader/internal/ytdlp"
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

type loginReq struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	loginOK := subtle.ConstantTimeCompare([]byte(req.Login), []byte(s.cfg.Login))
	passOK := subtle.ConstantTimeCompare([]byte(req.Password), []byte(s.cfg.Password))
	if loginOK&passOK != 1 {
		time.Sleep(500 * time.Millisecond)
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	secret := []byte(s.cfg.SessionSecret)
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    auth.NewToken(secret, time.Now().Add(30*24*time.Hour)),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 3600,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"has_api_key": s.cfg.GoogleAPIKey != "",
	})
}

func (s *Server) handleResolve(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		writeErr(w, http.StatusBadRequest, "url is required")
		return
	}
	info, err := ytdlp.Resolve(r.Context(), rawURL, s.cfg.ProxyURL, s.cfg.CookiesFile)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}

type createJobReq struct {
	Title   string        `json:"title"`
	Options ytdlp.Options `json:"options"`
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var req createJobReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := req.Options.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Options.TagPlaylist && s.cfg.GoogleAPIKey == "" {
		writeErr(w, http.StatusBadRequest, "GOOGLE_API_KEY not configured")
		return
	}

	j, err := s.mgr.Create(req.Options, req.Title)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, jobView(j))
}

type fileView struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type jobViewT struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	State      string     `json:"state"`
	Error      string     `json:"error"`
	Mode       string     `json:"mode"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Files      []fileView `json:"files"`
	FileCount  int        `json:"file_count"`
}

func jobView(j *jobs.Job) jobViewT {
	st, errMsg, finishedAt, files := j.Snapshot()

	fv := make([]fileView, len(files))
	for i, f := range files {
		fv[i] = fileView{Name: f.Name, Size: f.Size}
	}

	var finished *time.Time
	if !finishedAt.IsZero() {
		finished = &finishedAt
	}

	return jobViewT{
		ID:         j.ID,
		Title:      j.Title,
		State:      string(st),
		Error:      errMsg,
		Mode:       j.Opts.Mode,
		CreatedAt:  j.CreatedAt,
		FinishedAt: finished,
		Files:      fv,
		FileCount:  len(fv),
	}
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	list := s.mgr.List()
	views := make([]jobViewT, len(list))
	for i, j := range list {
		views[i] = jobView(j)
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": views})
}

type jobDetailView struct {
	jobViewT
	Log []string `json:"log"`
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	j, ok := s.mgr.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, jobDetailView{jobView(j), j.LogTail()})
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	j, ok := s.mgr.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	name := r.PathValue("name")
	if name != filepath.Base(name) || strings.Contains(name, "..") {
		writeErr(w, http.StatusBadRequest, "invalid file name")
		return
	}
	full := filepath.Join(j.Dir, name)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+sanitizeFilename(name)+"\"")
	http.ServeFile(w, r, full)
}

func (s *Server) handleZip(w http.ResponseWriter, r *http.Request) {
	j, ok := s.mgr.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	st, _, _, files := j.Snapshot()
	if st != jobs.Done || len(files) == 0 {
		writeErr(w, http.StatusConflict, "job has no files")
		return
	}
	name := sanitizeFilename(j.Title)
	if name == "" {
		name = j.ID
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.zip"`)
	zw := zip.NewWriter(w)
	for _, f := range files {
		hdr := &zip.FileHeader{Name: f.Name, Method: zip.Store}
		hdr.Modified = time.Now()
		fw, err := zw.CreateHeader(hdr)
		if err != nil {
			return
		}
		src, err := os.Open(filepath.Join(j.Dir, f.Name))
		if err != nil {
			continue
		}
		_, cpErr := io.Copy(fw, src)
		src.Close()
		if cpErr != nil {
			return // client gone mid-stream
		}
	}
	zw.Close()
}

// sanitizeFilename strips characters invalid on Windows/zip.
func sanitizeFilename(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return '_'
		}
		return r
	}, strings.TrimSpace(s))
}

func (s *Server) handleDeleteJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.mgr.Get(id); !ok {
		http.NotFound(w, r)
		return
	}
	if err := s.mgr.Delete(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
