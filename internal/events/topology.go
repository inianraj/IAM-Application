package events

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	ExchangeName = "crud.events"

	QueueWelcome      = "email.welcome"
	QueueWelcomeRetry = "email.welcome.retry"
	QueueWelcomeDLQ   = "email.welcome.dlq"

	// RetryDelayMillis is how long a failed message waits in the retry queue
	// before it is dead-lettered back to QueueWelcome.
	RetryDelayMillis = 30000
)

// Declare creates the exchange, queues and bindings. It is idempotent, so both
// the API and the worker call it on start.
func Declare(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(ExchangeName, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange: %w", err)
	}

	if _, err := ch.QueueDeclare(QueueWelcome, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare %s: %w", QueueWelcome, err)
	}
	if err := ch.QueueBind(QueueWelcome, TypeUserSignedUp, ExchangeName, false, nil); err != nil {
		return fmt.Errorf("bind %s: %w", QueueWelcome, err)
	}

	// Messages wait here, then expire back into the main queue through the
	// default exchange.
	retryArgs := amqp.Table{
		"x-message-ttl":             int32(RetryDelayMillis),
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": QueueWelcome,
	}
	if _, err := ch.QueueDeclare(QueueWelcomeRetry, true, false, false, false, retryArgs); err != nil {
		return fmt.Errorf("declare %s: %w", QueueWelcomeRetry, err)
	}

	if _, err := ch.QueueDeclare(QueueWelcomeDLQ, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare %s: %w", QueueWelcomeDLQ, err)
	}

	return nil
}
