// Command web serves the static frontend in the web/ folder. It is a separate
// server from the API on purpose, so the UI has its own origin like it would in
// a real deployment (see ALLOWED_ORIGINS in CLAUDE.md).
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", ":3000", "address to listen on")
	dir := flag.String("dir", "web", "folder with the frontend files")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if info, err := os.Stat(*dir); err != nil || !info.IsDir() {
		logger.Error("frontend folder not found (run from the repository root or pass -dir)", "dir", *dir)
		os.Exit(1)
	}

	files := http.FileServer(http.Dir(*dir))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always fetch the latest files while developing.
		w.Header().Set("Cache-Control", "no-store")
		files.ServeHTTP(w, r)
	})

	server := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	logger.Info("serving the frontend", "address", *addr, "dir", *dir)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("web server failed", "error", err)
		os.Exit(1)
	}
}
