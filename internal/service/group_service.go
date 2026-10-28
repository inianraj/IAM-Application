package service

import (
	"context"

	"crud/internal/authz"
	"crud/internal/domain"
)

type GroupStore interface {
	Create(ctx context.Context, g domain.Group, permissions []string) (domain.Group, error)
	GetByID(ctx context.Context, id int64) (domain.Group, error)
	ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.Group, error)
	Update(ctx context.Context, g domain.Group) (domain.Group, error)
	Delete(ctx context.Context, id int64) error
	ListMembers(ctx context.Context, groupID int64) ([]domain.GroupMember, error)

	Permissions(ctx context.Context, groupID int64) ([]string, error)
	SetPermissions(ctx context.Context, groupID int64, permissions []string) error
	ListPermissions(ctx context.Context) ([]domain.Permission, error)
}

// GroupService applies the group rules. Every operation needs a group:*
// permission in the organisation the group belongs to, and the two built-in
// groups (admin, member) cannot be renamed, deleted or given other
// permissions. A group can't be moved to another organisation.
type GroupService struct {
	store GroupStore
	authz *authz.Authorizer
}

func NewGroupService(store GroupStore, az *authz.Authorizer) *GroupService {
	return &GroupService{store: store, authz: az}
}

func checkGroupID(id int64) error {
	if id <= 0 {
		return domain.ErrInvalidGroupID
	}
	return nil
}

// Create makes a group with the given permissions, or with the member set when
// permissions is nil. Choosing permissions needs group:manage_permissions in
// addition to group:create.
func (s *GroupService) Create(ctx context.Context, g domain.Group, permissions []string) (domain.Group, error) {
	if g.OrganisationID <= 0 {
		return domain.Group{}, domain.ErrInvalidOrganisationID
	}
	if err := s.authz.Require(ctx, domain.PermGroupCreate, g.OrganisationID, domain.ErrOrganisationNotFound); err != nil {
		return domain.Group{}, err
	}
	if err := g.Validate(); err != nil {
		return domain.Group{}, err
	}

	if permissions == nil {
		permissions = domain.MemberPermissions()
	} else {
		if err := s.authz.Require(ctx, domain.PermGroupManagePerms, g.OrganisationID, domain.ErrOrganisationNotFound); err != nil {
			return domain.Group{}, err
		}
		// Unknown keys are rejected by the database when they are stored.
		permissions = domain.UniquePermissions(permissions)
	}

	return s.store.Create(ctx, g, permissions)
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
	if domain.IsBuiltinGroup(existing.Name) {
		return domain.Group{}, domain.ErrBuiltinGroup
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
	if domain.IsBuiltinGroup(existing.Name) {
		return domain.ErrBuiltinGroup
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

// GetPermissions returns the permissions of a group of the caller's
// organisation.
func (s *GroupService) GetPermissions(ctx context.Context, groupID int64) ([]string, error) {
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
	return s.store.Permissions(ctx, groupID)
}

// SetPermissions replaces the permissions of a custom group. Every key must be
// in the catalogue; the built-in groups cannot be changed.
func (s *GroupService) SetPermissions(ctx context.Context, groupID int64, permissions []string) ([]string, error) {
	if err := checkGroupID(groupID); err != nil {
		return nil, err
	}

	g, err := s.store.GetByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Require(ctx, domain.PermGroupManagePerms, g.OrganisationID, domain.ErrGroupNotFound); err != nil {
		return nil, err
	}
	if domain.IsBuiltinGroup(g.Name) {
		return nil, domain.ErrBuiltinGroup
	}

	// Unknown keys are rejected by the database, which rolls the change back.
	unique := domain.UniquePermissions(permissions)
	if err := s.store.SetPermissions(ctx, groupID, unique); err != nil {
		return nil, err
	}
	return unique, nil
}

// ListPermissions returns every permission a group can be given, to build the
// list a group's permissions are picked from. It needs
// group:manage_permissions.
func (s *GroupService) ListPermissions(ctx context.Context) ([]domain.Permission, error) {
	caller, err := s.authz.Caller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Require(ctx, domain.PermGroupManagePerms, caller.OrganisationID, domain.ErrOrganisationNotFound); err != nil {
		return nil, err
	}
	return s.store.ListPermissions(ctx)
}
