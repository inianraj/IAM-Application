package events

import (
	"context"
	"log/slog"
	"time"

	"crud/internal/domain"
)

// publishTimeout bounds how long a signup can wait on the broker.
const publishTimeout = 2 * time.Second

// UserStore is the persistence contract the decorator wraps (same shape as service.UserStore).
type UserStore interface {
	Create(ctx context.Context, u domain.User, roleIDs []int64) (domain.User, error)
	GetByID(ctx context.Context, id int64) (domain.User, error)
	ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.User, error)
	Update(ctx context.Context, u domain.User) (domain.User, error)
	Delete(ctx context.Context, id int64) error
	AssignGroup(ctx context.Context, userID, groupID int64) error

	Permissions(ctx context.Context, userID int64) ([]string, error)
	Roles(ctx context.Context, userID int64) ([]domain.Role, error)
	SetRoles(ctx context.Context, userID int64, roleIDs []int64) error
}

// EventUserStore publishes user.signed_up after a successful Create. A failed
// publish is logged and never fails the signup.
type EventUserStore struct {
	UserStore
	pub    Publisher
	logger *slog.Logger
}

func NewUserStore(next UserStore, pub Publisher, logger *slog.Logger) *EventUserStore {
	return &EventUserStore{UserStore: next, pub: pub, logger: logger}
}

func (s *EventUserStore) Create(ctx context.Context, u domain.User, roleIDs []int64) (domain.User, error) {
	created, err := s.UserStore.Create(ctx, u, roleIDs)
	if err != nil {
		return domain.User{}, err
	}

	// The user is already saved, so publish even if the client has gone away.
	pubCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), publishTimeout)
	defer cancel()

	event, err := NewEvent(TypeUserSignedUp, UserSignedUp{
		UserID: created.ID,
		Name:   created.Name,
		Email:  created.Email,
	})
	if err == nil {
		err = s.pub.Publish(pubCtx, TypeUserSignedUp, event)
	}
	if err != nil {
		s.logger.Error("publishing user.signed_up failed; no welcome email will be sent", "user_id", created.ID, "error", err)
	}

	return created, nil
}
