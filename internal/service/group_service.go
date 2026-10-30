package service

import (
	"context"

	"crud/internal/authz"
	"crud/internal/domain"
)

type GroupStore interface {
	Create(ctx context.Context, g domain.Group, roleIDs []int64) (domain.Group, error)
	GetByID(ctx context.Context, id int64) (domain.Group, error)
	ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.Group, error)
	Update(ctx context.Context, g domain.Group) (domain.Group, error)
	Delete(ctx context.Context, id int64) error
	ListMembers(ctx context.Context, groupID int64) ([]domain.GroupMember, error)

	Roles(ctx context.Context, groupID int64) ([]domain.Role, error)
	SetRoles(ctx context.Context, groupID int64, roleIDs []int64) error
}

// GroupService applies the group rules. Every operation needs a group:*
// permission in the organisation the group belongs to; choosing the roles of a
// group needs role:assign, and a caller can only hand out roles whose
// permissions they hold. A group can't be moved to another organisation.
type GroupService struct {
	store GroupStore
	roles RoleReader
	authz *authz.Authorizer
}

func NewGroupService(store GroupStore, roles RoleReader, az *authz.Authorizer) *GroupService {
	return &GroupService{store: store, roles: roles, authz: az}
}

func checkGroupID(id int64) error {
	if id <= 0 {
		return domain.ErrInvalidGroupID
	}
	return nil
}

// Create makes a group, with the given roles or with none. Choosing roles
// needs role:assign in addition to group:create.
func (s *GroupService) Create(ctx context.Context, g domain.Group, roleIDs []int64) (domain.Group, error) {
	if g.OrganisationID <= 0 {
		return domain.Group{}, domain.ErrInvalidOrganisationID
	}
	if err := s.authz.Require(ctx, domain.PermGroupCreate, g.OrganisationID, domain.ErrOrganisationNotFound); err != nil {
		return domain.Group{}, err
	}
	if err := g.Validate(); err != nil {
		return domain.Group{}, err
	}

	roleIDs, err := checkRoleIDs(roleIDs)
	if err != nil {
		return domain.Group{}, err
	}
	if len(roleIDs) > 0 {
		if err := s.authz.Require(ctx, domain.PermRoleAssign, g.OrganisationID, domain.ErrOrganisationNotFound); err != nil {
			return domain.Group{}, err
		}
		if err := ensureGrantable(ctx, s.authz, s.roles, roleIDs); err != nil {
			return domain.Group{}, err
		}
	}

	return s.store.Create(ctx, g, roleIDs)
}

func (s *GroupService) Get(ctx context.Context, id int64) (domain.Group, error) {
	if err := checkGroupID(id); err != nil {
		return domain.Group{}, err
	}

	g, err := s.store.GetByID(ctx, id)
	if err != nil {
		return domain.Group{}, err
	}
	if err := s.authz.Require(ctx, domain.PermGroupRead, g.OrganisationID, domain.ErrGroupNotFound); err != nil {
		return domain.Group{}, err
	}
	return g, nil
}

func (s *GroupService) ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.Group, error) {
	if organisationID <= 0 {
		return nil, domain.ErrInvalidOrganisationID
	}
	if err := s.authz.Require(ctx, domain.PermGroupRead, organisationID, domain.ErrOrganisationNotFound); err != nil {
		return nil, err
	}
	return s.store.ListByOrganisation(ctx, organisationID)
}

func (s *GroupService) Update(ctx context.Context, g domain.Group) (domain.Group, error) {
	if err := checkGroupID(g.ID); err != nil {
		return domain.Group{}, err
	}
	if g.OrganisationID <= 0 {
		return domain.Group{}, domain.ErrInvalidOrganisationID
	}

	existing, err := s.store.GetByID(ctx, g.ID)
	if err != nil {
		return domain.Group{}, err
	}
	if err := s.authz.Require(ctx, domain.PermGroupUpdate, existing.OrganisationID, domain.ErrGroupNotFound); err != nil {
		return domain.Group{}, err
	}
	// The caller may change groups of the group's organisation, so requiring
	// the target organisation to be the same forbids moving it elsewhere.
	if err := s.authz.Require(ctx, domain.PermGroupUpdate, g.OrganisationID, domain.ErrOrganisationNotFound); err != nil {
		return domain.Group{}, err
	}

	if err := g.Validate(); err != nil {
		return domain.Group{}, err
	}
	return s.store.Update(ctx, g)
}

func (s *GroupService) Delete(ctx context.Context, id int64) error {
	if err := checkGroupID(id); err != nil {
		return err
	}

	existing, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.authz.Require(ctx, domain.PermGroupDelete, existing.OrganisationID, domain.ErrGroupNotFound); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

func (s *GroupService) ListMembers(ctx context.Context, groupID int64) ([]domain.GroupMember, error) {
	if err := checkGroupID(groupID); err != nil {
		return nil, err
	}

	g, err := s.store.GetByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Require(ctx, domain.PermGroupRead, g.OrganisationID, domain.ErrGroupNotFound); err != nil {
		return nil, err
	}
	return s.store.ListMembers(ctx, groupID)
}

// GetRoles returns the roles of a group of the caller's organisation.
func (s *GroupService) GetRoles(ctx context.Context, groupID int64) ([]domain.Role, error) {
	if err := checkGroupID(groupID); err != nil {
		return nil, err
	}

	g, err := s.store.GetByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Require(ctx, domain.PermGroupRead, g.OrganisationID, domain.ErrGroupNotFound); err != nil {
		return nil, err
	}
	return s.store.Roles(ctx, groupID)
}

// SetRoles replaces the roles of a group. It needs role:assign, every role
// must be in the group's organisation, and the caller must hold the
// permissions of each role this call adds or removes. The members get the
// change on their next request.
func (s *GroupService) SetRoles(ctx context.Context, groupID int64, roleIDs []int64) ([]domain.Role, error) {
	if err := checkGroupID(groupID); err != nil {
		return nil, err
	}

	g, err := s.store.GetByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Require(ctx, domain.PermRoleAssign, g.OrganisationID, domain.ErrGroupNotFound); err != nil {
		return nil, err
	}

	roleIDs, err = checkRoleIDs(roleIDs)
	if err != nil {
		return nil, err
	}
	old, err := s.store.Roles(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if err := ensureGrantable(ctx, s.authz, s.roles, changedRoleIDs(old, roleIDs)); err != nil {
		return nil, err
	}

	if err := s.store.SetRoles(ctx, groupID, roleIDs); err != nil {
		return nil, err
	}
	return s.store.Roles(ctx, groupID)
}
