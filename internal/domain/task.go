package domain

import "time"

type Task struct {
	ID     int64 `json:"id"`
	UserID int64 `json:"user_id"`
	// OrganisationID is the owner's organisation, stored with the record so
	// organisation-wide permissions can be checked without loading the owner.
	// It is 0 (omitted) for records of legacy users without an organisation.
	OrganisationID int64     `json:"organisation_id,omitempty"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
