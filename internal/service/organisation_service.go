package service

import (
	"context"
	"fmt"
	"strings"

	"crud/internal/authz"
	"crud/internal/domain"
)

type OrganisationStore interface {
	Create(ctx context.Context, o domain.Organisation, admin domain.User) (domain.Organisation, domain.User, error)
	GetByID(ctx context.Context, id int64) (domain.Organisation, error)
	List(ctx context.Context) ([]domain.Organisation, error)
	Update(ctx context.Context, o domain.Organisation) (domain.Organisation, error)
	Delete(ctx context.Context, id int64) (domain.DeletedOrganisation, error)
}

// OrganisationService applies the organisation rules. Create and List are
// public (an organisation must exist before anyone can be created in it); the
// other operations act on the caller's own organisation: reading it needs
// membership, changing or deleting it needs an admin.
type OrganisationService struct {
	store OrganisationStore
	authz *authz.Authorizer
}

func NewOrganisationService(store OrganisationStore, az *authz.Authorizer) *OrganisationService {
	return &OrganisationService{store: store, authz: az}
}

// Create makes the organisation together with its first admin user, who is
// created from admin.Name and admin.Email (the email must not be registered
// yet). No session is needed: the route is public.
func (s *OrganisationService) Create(ctx context.Context, o domain.Organisation, admin domain.User) (domain.Organisation, domain.User, error) {
	if err := o.Validate(); err != nil {
		return domain.Organisation{}, domain.User{}, err
	}
	if err := admin.Validate(); err != nil {
		return domain.Organisation{}, domain.User{}, err
	}

	admin.SignupMethod = domain.SignupMethodEmail

	return s.store.Create(ctx, o, admin)
}

// currentOrganisationID is the organisation of the signed-in user, read from
// the user the authentication middleware loaded for this request (not from a
// client-supplied id). A user without an organisation has none.
func (s *OrganisationService) currentOrganisationID(ctx context.Context) (int64, error) {
	caller, err := s.authz.Caller(ctx)
	if err != nil {
		return 0, err
	}
	if caller.OrganisationID == 0 {
		return 0, domain.ErrOrganisationNotFound
	}
	return caller.OrganisationID, nil
}

// GetCurrent returns the caller's own organisation.
func (s *OrganisationService) GetCurrent(ctx context.Context) (domain.Organisation, error) {
	id, err := s.currentOrganisationID(ctx)
	if err != nil {
		return domain.Organisation{}, err
	}
	if err := s.authz.Require(ctx, domain.PermOrganisationRead, id, domain.ErrOrganisationNotFound); err != nil {
		return domain.Organisation{}, err
	}
	return s.store.GetByID(ctx, id)
}

func (s *OrganisationService) List(ctx context.Context) ([]domain.Organisation, error) {
	return s.store.List(ctx)
}

// UpdateCurrent renames the caller's organisation. Admins only.
func (s *OrganisationService) UpdateCurrent(ctx context.Context, name string) (domain.Organisation, error) {
	id, err := s.currentOrganisationID(ctx)
	if err != nil {
		return domain.Organisation{}, err
	}
	if err := s.authz.Require(ctx, domain.PermOrganisationEdit, id, domain.ErrOrganisationNotFound); err != nil {
		return domain.Organisation{}, err
	}

	o := domain.Organisation{ID: id, Name: name}
	if err := o.Validate(); err != nil {
		return domain.Organisation{}, err
	}
	return s.store.Update(ctx, o)
}

// DeleteCurrent permanently deletes the caller's organisation together with
// its groups, users and the users' records. Admins only, and confirmName must
// be the organisation's exact name.
func (s *OrganisationService) DeleteCurrent(ctx context.Context, confirmName string) (domain.DeletedOrganisation, error) {
	id, err := s.currentOrganisationID(ctx)
	if err != nil {
		return domain.DeletedOrganisation{}, err
	}
	if err := s.authz.Require(ctx, domain.PermOrganisationErase, id, domain.ErrOrganisationNotFound); err != nil {
		return domain.DeletedOrganisation{}, err
	}

	org, err := s.store.GetByID(ctx, id)
	if err != nil {
		return domain.DeletedOrganisation{}, err
	}
	if strings.TrimSpace(confirmName) != org.Name {
		return domain.DeletedOrganisation{}, fmt.Errorf("%w: confirm_name must match the organisation name", domain.ErrInvalidOrganisation)
	}

	return s.store.Delete(ctx, id)
}
