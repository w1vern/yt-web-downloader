package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"yt-web-downloader/internal/config"
	"yt-web-downloader/internal/jobs"
	"yt-web-downloader/internal/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	mgr := jobs.NewManager(cfg)
	if err := mgr.Rebuild(); err != nil {
		log.Fatalf("rebuild: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mgr.Start(ctx)
	go mgr.RunJanitor(ctx)

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: web.New(cfg, mgr)}
	go func() {
		log.Printf("yt-web-downloader listening on :%s (data=%s, ttl=%s, workers=%d)",
			cfg.Port, cfg.DataDir, cfg.FileTTL, cfg.MaxConcurrentJobs)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
}
