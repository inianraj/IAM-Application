package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"crud/internal/events"
	"crud/internal/mail"
)

// Outcome is what the consumer should do with a message after handling it.
type Outcome int

const (
	Ack        Outcome = iota // done: delivered
	Retry                     // transient failure: send to the retry queue
	DeadLetter                // permanent failure or attempts exhausted: send to the DLQ
)

func (o Outcome) String() string {
	switch o {
	case Ack:
		return "ack"
	case Retry:
		return "retry"
	default:
		return "dead-letter"
	}
}

// WelcomeEmail turns user.signed_up events into welcome emails.
type WelcomeEmail struct {
	Sender      mail.Sender
	MaxAttempts int
}

// Handle processes one message body. attempt is the number of earlier failed
// attempts (0 on first delivery).
func (w WelcomeEmail) Handle(ctx context.Context, body []byte, attempt int) (Outcome, error) {
	var event events.Event
	if err := json.Unmarshal(body, &event); err != nil {
		return DeadLetter, fmt.Errorf("malformed event: %w", err)
	}
	if event.Type != events.TypeUserSignedUp {
		return DeadLetter, fmt.Errorf("unexpected event type %q", event.Type)
	}

	var data events.UserSignedUp
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return DeadLetter, fmt.Errorf("malformed event data: %w", err)
	}
	if data.Email == "" || strings.ContainsAny(data.Name+data.Email, "\r\n") {
		return DeadLetter, fmt.Errorf("invalid recipient for user %d", data.UserID)
	}

	err := w.Sender.Send(ctx, data.Email, "Welcome!", "Hi "+data.Name+",\n\nWelcome aboard. Your account has been created.\n")
	switch {
	case err == nil:
		return Ack, nil
	case mail.IsPermanent(err):
		return DeadLetter, err
	case attempt+1 >= w.MaxAttempts:
		return DeadLetter, fmt.Errorf("giving up after %d attempts: %w", attempt+1, err)
	default:
		return Retry, err
	}
}
