package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"crud/internal/config"
	"crud/internal/events"
	"crud/internal/mail"
	"crud/internal/worker"
)

const maxAttempts = 3

// The worker consumes user.signed_up events and sends welcome emails.
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg := config.Load()
	if cfg.RabbitMQURL == "" {
		logger.Error("RABBITMQ_URL is required")
		os.Exit(1)
	}

	conn, err := events.Open(cfg.RabbitMQURL)
	if err != nil {
		logger.Error("rabbitmq connection failed", "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		logger.Error("open channel failed", "error", err)
		os.Exit(1)
	}
	defer ch.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	handler := worker.WelcomeEmail{
		Sender:      mail.NewSMTPSender(cfg.SMTPAddr, cfg.MailFrom),
		MaxAttempts: maxAttempts,
	}

	logger.Info("worker started", "queue", events.QueueWelcome, "smtp", cfg.SMTPAddr)

	if err := worker.Run(ctx, ch, handler, logger); err != nil {
		logger.Error("worker stopped", "error", err)
		os.Exit(1)
	}

	logger.Info("worker stopped")
}
