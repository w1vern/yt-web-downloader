package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"yt-web-downloader/internal/ytdlp"
)

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	j, ok := s.mgr.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, unsub := j.Subscribe()
	defer unsub()

	send := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		fl.Flush()
	}
	st, errMsg, _, _ := j.Snapshot()
	send(ytdlp.Event{Type: "state", State: string(st), Error: errMsg})

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			send(ev)
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}
