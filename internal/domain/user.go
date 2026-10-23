package domain

import "time"

type User struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	// GroupID is the user's group, which always belongs to OrganisationID (the
	// database enforces it with a composite foreign key).
	GroupID        int64     `json:"group_id"`
	OrganisationID int64     `json:"organisation_id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// GroupMember is a user listed under a group.
type GroupMember struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}
