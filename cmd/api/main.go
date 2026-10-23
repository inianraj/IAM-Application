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

	_ "crud/docs"
	"crud/internal/config"
	"crud/internal/database"
	"crud/internal/handler"
	"crud/internal/repository"
	"crud/internal/router"
	"crud/internal/service"
)

// @title       Records CRUD API
// @version     1.0
// @description CRUD API for records stored in PostgreSQL.
// @BasePath    /api/v1
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

	userService := service.NewUserService(repository.NewUserRepository(db))
	userHandler := handler.NewUserHandler(userService)

	groupService := service.NewGroupService(repository.NewGroupRepository(db))
	groupHandler := handler.NewGroupHandler(groupService)

	organisationService := service.NewOrganisationService(repository.NewOrganisationRepository(db))
	organisationHandler := handler.NewOrganisationHandler(organisationService)

	routes := router.New(router.Handlers{
		Records:       recordHandler,
		Users:         userHandler,
		Groups:        groupHandler,
		Organisations: organisationHandler,
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           routes,
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
