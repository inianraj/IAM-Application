package service

import (
	"context"

	"crud/internal/domain"
)

type UserStore interface {
	Create(ctx context.Context, u domain.User) (domain.User, error)
	GetByID(ctx context.Context, id int64) (domain.User, error)
	Update(ctx context.Context, u domain.User) (domain.User, error)
	Delete(ctx context.Context, id int64) error
	AssignGroup(ctx context.Context, userID, groupID int64) error
}

type UserService struct {
	store UserStore
}

func NewUserService(store UserStore) *UserService {
	return &UserService{store: store}
}

func checkUserID(id int64) error {
	if id <= 0 {
		return domain.ErrInvalidUserID
	}
	return nil
}

// Signup validates the user, then stores them in the default group of
// organisation u.OrganisationID.
func (s *UserService) Signup(ctx context.Context, u domain.User) (domain.User, error) {
	if u.OrganisationID <= 0 {
		return domain.User{}, domain.ErrInvalidOrganisationID
	}
	if err := u.Validate(); err != nil {
		return domain.User{}, err
	}

	return s.store.Create(ctx, u)
}

func (s *UserService) Get(ctx context.Context, id int64) (domain.User, error) {
	if err := checkUserID(id); err != nil {
		return domain.User{}, err
	}
	return s.store.GetByID(ctx, id)
}

// Update changes the user's name and email.
func (s *UserService) Update(ctx context.Context, u domain.User) (domain.User, error) {
	if err := checkUserID(u.ID); err != nil {
		return domain.User{}, err
	}
	if err := u.Validate(); err != nil {
		return domain.User{}, err
	}

	return s.store.Update(ctx, u)
}

func (s *UserService) Delete(ctx context.Context, id int64) error {
	if err := checkUserID(id); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

// AssignGroup moves the user to a group, which must belong to the
// organisation stored on the user.
func (s *UserService) AssignGroup(ctx context.Context, userID, groupID int64) error {
	if err := checkUserID(userID); err != nil {
		return err
	}
	if groupID <= 0 {
		return domain.ErrInvalidGroupID
	}

	return s.store.AssignGroup(ctx, userID, groupID)
}
