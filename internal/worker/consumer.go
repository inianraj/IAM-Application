package worker

import (
	"context"
	"fmt"
	"log/slog"
	"maps"

	amqp "github.com/rabbitmq/amqp091-go"

	"crud/internal/events"
)

const attemptHeader = "x-attempt"

// Run consumes events.QueueWelcome until ctx is cancelled or the channel
// closes. Messages are handled one at a time and acked only after the outcome
// (including any move to the retry queue or DLQ) is safely recorded.
func Run(ctx context.Context, ch *amqp.Channel, h WelcomeEmail, logger *slog.Logger) error {
	if err := events.Declare(ch); err != nil {
		return err
	}
	if err := ch.Qos(1, 0, false); err != nil {
		return fmt.Errorf("set prefetch: %w", err)
	}

	deliveries, err := ch.Consume(events.QueueWelcome, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume %s: %w", events.QueueWelcome, err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("delivery channel closed")
			}
			process(ctx, ch, h, d, logger)
		}
	}
}

func process(ctx context.Context, ch *amqp.Channel, h WelcomeEmail, d amqp.Delivery, logger *slog.Logger) {
	attempt := attemptOf(d)
	outcome, err := h.Handle(ctx, d.Body, attempt)

	switch outcome {
	case Ack:
		logger.Info("welcome email sent", "message_id", d.MessageId)
	case Retry:
		logger.Warn("welcome email failed, will retry", "message_id", d.MessageId, "attempt", attempt+1, "error", err)
	default:
		logger.Error("welcome email dead-lettered", "message_id", d.MessageId, "attempt", attempt+1, "error", err)
	}

	var target string
	switch outcome {
	case Retry:
		target = events.QueueWelcomeRetry
	case DeadLetter:
		target = events.QueueWelcomeDLQ
	}

	if target != "" {
		if err := republish(ctx, ch, target, d, attempt+1); err != nil {
			// Keep the message: put it back on the queue instead of losing it.
			logger.Error("could not move message; requeueing", "message_id", d.MessageId, "target", target, "error", err)
			_ = d.Nack(false, true)
			return
		}
	}

	if err := d.Ack(false); err != nil {
		logger.Error("ack failed", "message_id", d.MessageId, "error", err)
	}
}

// republish copies d to a queue through the default exchange.
func republish(ctx context.Context, ch *amqp.Channel, queue string, d amqp.Delivery, attempt int) error {
	headers := amqp.Table{}
	maps.Copy(headers, d.Headers)
	headers[attemptHeader] = int32(attempt)

	return ch.PublishWithContext(ctx, "", queue, false, false, amqp.Publishing{
		ContentType:  d.ContentType,
		DeliveryMode: amqp.Persistent,
		MessageId:    d.MessageId,
		Timestamp:    d.Timestamp,
		Type:         d.Type,
		Headers:      headers,
		Body:         d.Body,
	})
}

func attemptOf(d amqp.Delivery) int {
	switch v := d.Headers[attemptHeader].(type) {
	case int32:
		return int(v)
	case int64:
		return int(v)
	case int:
		return v
	}
	return 0
}
