package domain

import "time"

type User struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	// GroupID is the user's group, which always belongs to OrganisationID (the
	// database enforces it with a composite foreign key). A user without an
	// organisation has both as 0 (NULL in the database, omitted from JSON).
	GroupID        int64 `json:"group_id,omitempty"`
	OrganisationID int64 `json:"organisation_id,omitempty"`
	// SignupMethod is how the user was created (SignupMethod* constants).
	SignupMethod string    `json:"signup_method" example:"email"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Password is the plain password on its way in (create only) and
	// PasswordHash the bcrypt hash stored in the NOT NULL users.password_hash
	// column. Neither is ever written to JSON, so neither reaches an API
	// response or the Redis cache.
	Password     string `json:"-"`
	PasswordHash string `json:"-"`
}

// The values allowed by the users_signup_method_check constraint.
const (
	// SignupMethodEmail users sign in with their email and password.
	SignupMethodEmail  = "email"
	SignupMethodGoogle = "google"
	SignupMethodGitHub = "github"
)

// GroupMember is a user listed under a group.
type GroupMember struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}
