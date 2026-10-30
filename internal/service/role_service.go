package service

import (
	"context"
	"fmt"
	"slices"

	"crud/internal/authz"
	"crud/internal/domain"
)

type RoleStore interface {
	Create(ctx context.Context, r domain.Role, permissions []string) (domain.Role, error)
	GetByID(ctx context.Context, id int64) (domain.Role, error)
	ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.Role, error)
	Update(ctx context.Context, r domain.Role) (domain.Role, error)
	Delete(ctx context.Context, id int64) error

	Permissions(ctx context.Context, roleID int64) ([]string, error)
	SetPermissions(ctx context.Context, roleID int64, permissions []string) error
	HolderIDs(ctx context.Context, roleID int64) ([]int64, error)
	ListPermissions(ctx context.Context) ([]domain.Permission, error)
}

// RoleService applies the role rules. Roles belong to the caller's own
// organisation, every operation needs a role:* permission, and the four
// built-in roles (owner, admin, member, guest) cannot be renamed, changed or
// deleted. A role can only be given permissions the caller holds.
type RoleService struct {
	store RoleStore
	authz *authz.Authorizer
}

func NewRoleService(store RoleStore, az *authz.Authorizer) *RoleService {
	return &RoleService{store: store, authz: az}
}

func checkRoleID(id int64) error {
	if id <= 0 {
		return domain.ErrInvalidRoleID
	}
	return nil
}

// callerOrganisation is the organisation of the signed-in user.
func (s *RoleService) callerOrganisation(ctx context.Context) (int64, error) {
	caller, err := s.authz.Caller(ctx)
	if err != nil {
		return 0, err
	}
	if caller.OrganisationID == 0 {
		return 0, domain.ErrOrganisationNotFound
	}
	return caller.OrganisationID, nil
}

// checkGrantable makes sure every key exists (the catalogue, 400 otherwise)
// and that the caller holds each key in changed, the permissions this call
// adds to or takes from a role.
func (s *RoleService) checkGrantable(ctx context.Context, keys, changed []string) error {
	catalogue, err := s.store.ListPermissions(ctx)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if !slices.ContainsFunc(catalogue, func(p domain.Permission) bool { return p.Key == k }) {
			return fmt.Errorf("%w: unknown permission %q", domain.ErrInvalidRole, k)
		}
	}

	held, err := s.authz.CallerPermissions(ctx)
	if err != nil {
		return err
	}
	for _, k := range changed {
		if !slices.Contains(held, k) {
			return domain.ErrForbidden
		}
	}
	return nil
}

// Create makes a role in the caller's organisation with the given permissions
// (none when empty). It needs role:create, and the caller must hold every
// permission they put on the role.
func (s *RoleService) Create(ctx context.Context, role domain.Role, permissions []string) (domain.Role, error) {
	orgID, err := s.callerOrganisation(ctx)
	if err != nil {
		return domain.Role{}, err
	}
	if err := s.authz.Require(ctx, domain.PermRoleCreate, orgID, domain.ErrOrganisationNotFound); err != nil {
		return domain.Role{}, err
	}

	role.OrganisationID = orgID
	if err := role.Validate(); err != nil {
		return domain.Role{}, err
	}

	permissions = domain.UniquePermissions(permissions)
	if err := s.checkGrantable(ctx, permissions, permissions); err != nil {
		return domain.Role{}, err
	}

	return s.store.Create(ctx, role, permissions)
}

func (s *RoleService) Get(ctx context.Context, id int64) (domain.Role, error) {
	if err := checkRoleID(id); err != nil {
		return domain.Role{}, err
	}

	role, err := s.store.GetByID(ctx, id)
	if err != nil {
		return domain.Role{}, err
	}
	if err := s.authz.Require(ctx, domain.PermRoleRead, role.OrganisationID, domain.ErrRoleNotFound); err != nil {
		return domain.Role{}, err
	}
	return role, nil
}

// List returns the roles of the caller's organisation.
func (s *RoleService) List(ctx context.Context) ([]domain.Role, error) {
	orgID, err := s.callerOrganisation(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Require(ctx, domain.PermRoleRead, orgID, domain.ErrOrganisationNotFound); err != nil {
		return nil, err
	}
	return s.store.ListByOrganisation(ctx, orgID)
}

// editable loads the role and checks that the caller may change it with perm;
// the built-in roles never can be.
func (s *RoleService) editable(ctx context.Context, id int64, perm string) (domain.Role, error) {
	if err := checkRoleID(id); err != nil {
		return domain.Role{}, err
	}

	role, err := s.store.GetByID(ctx, id)
	if err != nil {
		return domain.Role{}, err
	}
	if err := s.authz.Require(ctx, perm, role.OrganisationID, domain.ErrRoleNotFound); err != nil {
		return domain.Role{}, err
	}
	if domain.IsBuiltinRole(role.Name) {
		return domain.Role{}, domain.ErrBuiltinRole
	}
	return role, nil
}

// Update renames a custom role.
func (s *RoleService) Update(ctx context.Context, role domain.Role) (domain.Role, error) {
	existing, err := s.editable(ctx, role.ID, domain.PermRoleUpdate)
	if err != nil {
		return domain.Role{}, err
	}

	role.OrganisationID = existing.OrganisationID
	if err := role.Validate(); err != nil {
		return domain.Role{}, err
	}
	return s.store.Update(ctx, role)
}

// Delete removes a custom role that no user or group has (409 otherwise).
func (s *RoleService) Delete(ctx context.Context, id int64) error {
	if _, err := s.editable(ctx, id, domain.PermRoleDelete); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

// GetPermissions returns the permission keys of a role of the caller's
// organisation.
func (s *RoleService) GetPermissions(ctx context.Context, id int64) ([]string, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.store.Permissions(ctx, id)
}

// SetPermissions replaces the permissions of a custom role. Every key must be
// in the catalogue, and the caller must hold each permission this call adds
// or removes. It takes effect for the holders on their next request.
func (s *RoleService) SetPermissions(ctx context.Context, id int64, permissions []string) ([]string, error) {
	if _, err := s.editable(ctx, id, domain.PermRoleUpdate); err != nil {
		return nil, err
	}

	old, err := s.store.Permissions(ctx, id)
	if err != nil {
		return nil, err
	}
	unique := domain.UniquePermissions(permissions)
	if err := s.checkGrantable(ctx, unique, changedKeys(old, unique)); err != nil {
		return nil, err
	}

	if err := s.store.SetPermissions(ctx, id, unique); err != nil {
		return nil, err
	}
	return unique, nil
}

// ListPermissions returns every permission a role can be given, to build the
// list a role's permissions are picked from. It needs role:read.
func (s *RoleService) ListPermissions(ctx context.Context) ([]domain.Permission, error) {
	orgID, err := s.callerOrganisation(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Require(ctx, domain.PermRoleRead, orgID, domain.ErrOrganisationNotFound); err != nil {
		return nil, err
	}
	return s.store.ListPermissions(ctx)
}
