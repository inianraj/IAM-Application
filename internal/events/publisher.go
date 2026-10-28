package events

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Publisher sends events to the broker.
type Publisher interface {
	Publish(ctx context.Context, routingKey string, e Event) error
}

// Open connects to RabbitMQ.
func Open(url string) (*amqp.Connection, error) {
	conn, err := amqp.DialConfig(url, amqp.Config{
		Heartbeat: 10 * time.Second,
		Dial:      amqp.DefaultDial(3 * time.Second),
	})
	if err != nil {
		return nil, fmt.Errorf("connect to RabbitMQ: %w", err)
	}
	return conn, nil
}

// AMQPPublisher publishes persistent messages on one confirm-mode channel.
// Channels are not goroutine-safe, hence the mutex.
type AMQPPublisher struct {
	mu sync.Mutex
	ch *amqp.Channel
}

// NewAMQPPublisher declares the topology and puts a channel in confirm mode.
func NewAMQPPublisher(conn *amqp.Connection) (*AMQPPublisher, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("open channel: %w", err)
	}

	if err := Declare(ch); err != nil {
		_ = ch.Close()
		return nil, err
	}
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		return nil, fmt.Errorf("enable publisher confirms: %w", err)
	}

	return &AMQPPublisher{ch: ch}, nil
}

func (p *AMQPPublisher) Publish(ctx context.Context, routingKey string, e Event) error {
	body, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	confirm, err := p.ch.PublishWithDeferredConfirmWithContext(ctx, ExchangeName, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    e.ID,
		Timestamp:    e.OccurredAt,
		Type:         e.Type,
		Body:         body,
	})
	if err != nil {
		return fmt.Errorf("publish: %w", err)
	}

	acked, err := confirm.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("wait for broker confirm: %w", err)
	}
	if !acked {
		return fmt.Errorf("broker rejected the message")
	}
	return nil
}

func (p *AMQPPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ch.Close()
}
