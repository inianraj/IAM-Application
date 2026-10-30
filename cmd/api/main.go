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
	"crud/internal/auth"
	"crud/internal/authz"
	"crud/internal/cache"
	"crud/internal/config"
	"crud/internal/database"
	"crud/internal/events"
	"crud/internal/handler"
	"crud/internal/repository"
	"crud/internal/router"
	"crud/internal/service"
	"crud/internal/session"
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

	userRepo := repository.NewUserRepository(db)
	groupRepo := repository.NewGroupRepository(db)
	roleRepo := repository.NewRoleRepository(db)

	var recordStore service.TaskStore = repository.NewTaskRepository(db)
	var userStore service.UserStore = userRepo
	var groupStore service.GroupStore = groupRepo
	var roleStore service.RoleStore = roleRepo
	var organisationStore service.OrganisationStore = repository.NewOrganisationRepository(db)

	// Redis is required: it holds the login sessions, and the API refuses to
	// run without them. The same connection serves the read cache.
	if cfg.RedisAddr == "" {
		logger.Error("REDIS_ADDR is required for sessions")
		os.Exit(1)
	}

	redisCtx, cancelRedis := context.WithTimeout(context.Background(), 5*time.Second)
	rdb, err := cache.Open(redisCtx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	cancelRedis()

	if err != nil {
		logger.Error("redis initialization failed", "error", err)
		os.Exit(1)
	}
	defer rdb.Close()

	recordStore = cache.NewTaskStore(recordStore, rdb, cfg.CacheTTL, logger)
	userStore = cache.NewUserStore(userStore, rdb, cfg.CacheTTL, logger)
	groupStore = cache.NewGroupStore(groupStore, rdb, cfg.CacheTTL, logger)
	roleStore = cache.NewRoleStore(roleStore, rdb, cfg.CacheTTL, logger)
	organisationStore = cache.NewOrganisationStore(organisationStore, groupRepo, rdb, cfg.CacheTTL, logger)
	logger.Info("redis enabled", "address", cfg.RedisAddr, "cache_ttl", cfg.CacheTTL, "session_ttl", cfg.SessionTTL)

	// Events are optional: without RABBITMQ_URL, or if the broker is
	// unreachable, signups still work but no welcome email is queued.
	if cfg.RabbitMQURL != "" {
		conn, err := events.Open(cfg.RabbitMQURL)
		if err == nil {
			defer conn.Close()

			var pub *events.AMQPPublisher
			pub, err = events.NewAMQPPublisher(conn)
			if err == nil {
				defer pub.Close()
				userStore = events.NewUserStore(userStore, pub, logger)
				logger.Info("rabbitmq events enabled")
			}
		}
		if err != nil {
			logger.Warn("rabbitmq unavailable, running without events", "error", err)
		}
	}

	// Authentication and authorization read users and groups through the
	// cached stores, so they always see current data.
	authorizer := authz.New(userStore, userStore)
	sessions := session.NewRedisStore(rdb, cfg.SessionTTL)
	cookie := auth.CookieConfig{Secure: cfg.CookieSecure, TTL: cfg.SessionTTL}

	userService := service.NewUserService(userStore, groupStore, roleStore, authorizer)
	userHandler := handler.NewUserHandler(userService)

	organisationService := service.NewOrganisationService(organisationStore, authorizer)
	organisationHandler := handler.NewOrganisationHandler(organisationService)

	authService := service.NewAuthService(userRepo, sessions, session.NewLoginLimiter(rdb))
	authHandler := handler.NewAuthHandler(authService, userService, organisationService, cookie)

	recordService := service.NewTaskService(recordStore, authorizer)
	recordHandler := handler.NewRecordHandler(recordService)

	groupService := service.NewGroupService(groupStore, roleStore, authorizer)
	groupHandler := handler.NewGroupHandler(groupService)

	roleService := service.NewRoleService(roleStore, authorizer)
	roleHandler := handler.NewRoleHandler(roleService)

	authMiddleware := auth.NewMiddleware(sessions, userStore, cookie, router.PublicRoutes, logger)

	routes := router.New(router.Handlers{
		Auth:          authHandler,
		Records:       recordHandler,
		Users:         userHandler,
		Groups:        groupHandler,
		Roles:         roleHandler,
		Organisations: organisationHandler,
		Middleware: []func(http.Handler) http.Handler{
			auth.OriginGuard(cfg.AllowedOrigins),
			authMiddleware.Wrap,
		},
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
