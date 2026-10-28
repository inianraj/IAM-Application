package events

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"
)

const TypeUserSignedUp = "user.signed_up"

// Event is the JSON envelope published to RabbitMQ.
type Event struct {
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	OccurredAt time.Time       `json:"occurred_at"`
	Data       json.RawMessage `json:"data"`
}

// UserSignedUp is the Data of a TypeUserSignedUp event.
type UserSignedUp struct {
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
}

// NewEvent wraps data in an envelope with a fresh random ID.
func NewEvent(eventType string, data any) (Event, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return Event{}, err
	}

	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return Event{}, err
	}

	return Event{
		Type:       eventType,
		ID:         hex.EncodeToString(id[:]),
		OccurredAt: time.Now().UTC(),
		Data:       raw,
	}, nil
}
