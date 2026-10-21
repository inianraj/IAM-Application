package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"crud/internal/config"
	"crud/internal/database"
	"crud/internal/handler"
	"crud/internal/repository"
	"crud/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Load also reads .env into the environment for database.Open.
	cfg := config.Load()

	startupCtx, cancelStartup := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)

	db, err := database.Open(startupCtx)
	cancelStartup()

	if err != nil {
		logger.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Initialize application layers.

	recordRepo := repository.NewTaskRepository(db)
	recordService := service.NewTaskService(recordRepo)
	recordHandler := handler.NewRecordHandler(recordService)

	// Register HTTP routes.
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/records", recordHandler.Create)
	mux.HandleFunc("GET /api/v1/records", recordHandler.GetAll)
	mux.HandleFunc("GET /api/v1/records/{id}", recordHandler.GetByID)
	mux.HandleFunc("PUT /api/v1/records/{id}", recordHandler.Update)
	mux.HandleFunc("DELETE /api/v1/records/{id}", recordHandler.Delete)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	serverErrors := make(chan error, 1)

	go func() {
		logger.Info("starting HTTP server", "address", cfg.HTTPAddr)

		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}

		close(serverErrors)
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")

	case err, ok := <-serverErrors:
		if ok && err != nil {
			logger.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
		return
	}

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		cfg.ShutdownTimeout,
	)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)

		if closeErr := server.Close(); closeErr != nil {
			logger.Error("forced shutdown failed", "error", closeErr)
		}

		os.Exit(1)
	}

	logger.Info("server stopped")
}
