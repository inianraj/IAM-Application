package service

import (
	"context"

	"crud/internal/authz"
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
	authz *authz.Authorizer
}

func NewUserService(store UserStore, az *authz.Authorizer) *UserService {
	return &UserService{store: store, authz: az}
}

func checkUserID(id int64) error {
	if id <= 0 {
		return domain.ErrInvalidUserID
	}
	return nil
}

// Create adds a user to the caller's own organisation, in the basic "member"
// group (signup method "email", no password). The caller needs the
// user:assign_group permission, which only admins have; there is no public way
// to join an organisation.
func (s *UserService) Create(ctx context.Context, u domain.User) (domain.User, error) {
	caller, err := s.authz.Caller(ctx)
	if err != nil {
		return domain.User{}, err
	}
	if err := s.authz.Require(ctx, domain.PermUserAssignGroup, caller.OrganisationID, domain.ErrOrganisationNotFound); err != nil {
		return domain.User{}, err
	}
	if err := u.Validate(); err != nil {
		return domain.User{}, err
	}

	u.OrganisationID = caller.OrganisationID
	u.SignupMethod = domain.SignupMethodEmail

	return s.store.Create(ctx, u)
}

// Get returns the caller themselves, or a user of the caller's organisation
// when the caller has the user:read permission.
func (s *UserService) Get(ctx context.Context, id int64) (domain.User, error) {
	if err := checkUserID(id); err != nil {
		return domain.User{}, err
	}

	u, err := s.store.GetByID(ctx, id)
	if err != nil {
		return domain.User{}, err
	}
	if err := s.authz.RequireSelfOr(ctx, domain.PermUserRead, u.ID, u.OrganisationID, domain.ErrUserNotFound); err != nil {
		return domain.User{}, err
	}
	return u, nil
}

// Update changes the user's name; the email is the login identity and never
// changes. It needs user:update:own for yourself, user:update:org for others.
func (s *UserService) Update(ctx context.Context, u domain.User) (domain.User, error) {
	if err := checkUserID(u.ID); err != nil {
		return domain.User{}, err
	}
	if _, err := s.authz.RequireScopedForUser(ctx, domain.ResourceUser, domain.ActionUpdate, u.ID, domain.ErrUserNotFound); err != nil {
		return domain.User{}, err
	}
	if err := u.ValidateName(); err != nil {
		return domain.User{}, err
	}

	return s.store.Update(ctx, u)
}

// Delete removes a user. It needs the user:delete permission in the user's
// organisation, which only admins have.
func (s *UserService) Delete(ctx context.Context, id int64) error {
	if err := checkUserID(id); err != nil {
		return err
	}
	if _, err := s.authz.RequireForUser(ctx, domain.PermUserDelete, id, domain.ErrUserNotFound); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

// AssignGroup moves the user to a group, which must belong to the
// organisation stored on the user. It needs the user:assign_group permission.
func (s *UserService) AssignGroup(ctx context.Context, userID, groupID int64) error {
	if err := checkUserID(userID); err != nil {
		return err
	}
	if groupID <= 0 {
		return domain.ErrInvalidGroupID
	}
	if _, err := s.authz.RequireForUser(ctx, domain.PermUserAssignGroup, userID, domain.ErrUserNotFound); err != nil {
		return err
	}

	return s.store.AssignGroup(ctx, userID, groupID)
}
