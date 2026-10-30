package service

import (
	"context"

	"crud/internal/authz"
	"crud/internal/domain"
)

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

// GroupReader is what the user service needs to know about groups: which
// organisation one is in and which roles it gives.
type GroupReader interface {
	GetByID(ctx context.Context, id int64) (domain.Group, error)
	Roles(ctx context.Context, groupID int64) ([]domain.Role, error)
}

type UserService struct {
	store  UserStore
	groups GroupReader
	roles  RoleReader
	authz  *authz.Authorizer
}

func NewUserService(store UserStore, groups GroupReader, roles RoleReader, az *authz.Authorizer) *UserService {
	return &UserService{store: store, groups: groups, roles: roles, authz: az}
}

func checkUserID(id int64) error {
	if id <= 0 {
		return domain.ErrInvalidUserID
	}
	return nil
}

// Create adds a user to the caller's own organisation with no group (signup
// method "email", with the given password). The caller needs the user:assign_group
// permission, which admins have; there is no public way to join an
// organisation. The user gets the member role unless roleIDs say otherwise;
// choosing roles needs role:assign, and the caller must hold the permissions
// of each role they hand out.
func (s *UserService) Create(ctx context.Context, u domain.User, roleIDs []int64) (domain.User, error) {
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
	if err := u.SetPassword(u.Password); err != nil {
		return domain.User{}, err
	}

	roleIDs, err = checkRoleIDs(roleIDs)
	if err != nil {
		return domain.User{}, err
	}
	if len(roleIDs) > 0 {
		if err := s.authz.Require(ctx, domain.PermRoleAssign, caller.OrganisationID, domain.ErrOrganisationNotFound); err != nil {
			return domain.User{}, err
		}
		if err := ensureGrantable(ctx, s.authz, s.roles, roleIDs); err != nil {
			return domain.User{}, err
		}
	}

	u.OrganisationID = caller.OrganisationID
	u.SignupMethod = domain.SignupMethodEmail

	return s.store.Create(ctx, u, roleIDs)
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
// organisation, and the caller must hold the permissions of every role the
// user has directly (an admin cannot delete an owner). The last owner of an
// organisation cannot be deleted.
func (s *UserService) Delete(ctx context.Context, id int64) error {
	if err := checkUserID(id); err != nil {
		return err
	}
	if _, err := s.authz.RequireForUser(ctx, domain.PermUserDelete, id, domain.ErrUserNotFound); err != nil {
		return err
	}

	held, err := s.store.Roles(ctx, id)
	if err != nil {
		return err
	}
	if err := ensureGrantable(ctx, s.authz, s.roles, changedRoleIDs(held, nil)); err != nil {
		return err
	}

	return s.store.Delete(ctx, id)
}

// AssignGroup moves the user to a group, which must belong to the
// organisation stored on the user, or out of their group when groupID is 0.
// It needs the user:assign_group permission, and the caller must hold the
// permissions of the roles the user gains or loses with the move.
func (s *UserService) AssignGroup(ctx context.Context, userID, groupID int64) error {
	if err := checkUserID(userID); err != nil {
		return err
	}
	if groupID < 0 {
		return domain.ErrInvalidGroupID
	}
	target, err := s.authz.RequireForUser(ctx, domain.PermUserAssignGroup, userID, domain.ErrUserNotFound)
	if err != nil {
		return err
	}

	// The roles of the old and the new group, whichever the user stops or
	// starts having. A group of another organisation is left to the store,
	// which refuses it.
	var leaving, joining []domain.Role
	if target.GroupID != 0 {
		if leaving, err = s.groups.Roles(ctx, target.GroupID); err != nil {
			return err
		}
	}
	if groupID != 0 {
		g, err := s.groups.GetByID(ctx, groupID)
		if err != nil {
			return err
		}
		if g.OrganisationID == target.OrganisationID {
			if joining, err = s.groups.Roles(ctx, groupID); err != nil {
				return err
			}
		}
	}
	var joiningIDs []int64
	for _, r := range joining {
		joiningIDs = append(joiningIDs, r.ID)
	}
	if err := ensureGrantable(ctx, s.authz, s.roles, changedRoleIDs(leaving, joiningIDs)); err != nil {
		return err
	}

	return s.store.AssignGroup(ctx, userID, groupID)
}

// GetRoles returns the roles the user holds directly (not the ones their group
// gives). It needs role:read, except for yourself.
func (s *UserService) GetRoles(ctx context.Context, id int64) ([]domain.Role, error) {
	if _, err := s.readable(ctx, id); err != nil {
		return nil, err
	}
	return s.store.Roles(ctx, id)
}

// GetPermissions returns everything the user may do: their direct roles plus
// the roles of their group. It needs role:read, except for yourself.
func (s *UserService) GetPermissions(ctx context.Context, id int64) ([]string, error) {
	if _, err := s.readable(ctx, id); err != nil {
		return nil, err
	}
	return s.store.Permissions(ctx, id)
}

// readable loads the user and checks that the caller may see their roles.
func (s *UserService) readable(ctx context.Context, id int64) (domain.User, error) {
	if err := checkUserID(id); err != nil {
		return domain.User{}, err
	}

	u, err := s.store.GetByID(ctx, id)
	if err != nil {
		return domain.User{}, err
	}
	if err := s.authz.RequireSelfOr(ctx, domain.PermRoleRead, u.ID, u.OrganisationID, domain.ErrUserNotFound); err != nil {
		return domain.User{}, err
	}
	return u, nil
}

// SetRoles replaces the roles the user holds directly. It needs role:assign,
// the caller must hold the permissions of each role this call adds or removes
// (so only an owner can give or take the owner role), and an organisation
// always keeps at least one owner. The change applies on the user's next
// request.
func (s *UserService) SetRoles(ctx context.Context, userID int64, roleIDs []int64) ([]domain.Role, error) {
	if err := checkUserID(userID); err != nil {
		return nil, err
	}
	if _, err := s.authz.RequireForUser(ctx, domain.PermRoleAssign, userID, domain.ErrUserNotFound); err != nil {
		return nil, err
	}

	roleIDs, err := checkRoleIDs(roleIDs)
	if err != nil {
		return nil, err
	}
	old, err := s.store.Roles(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := ensureGrantable(ctx, s.authz, s.roles, changedRoleIDs(old, roleIDs)); err != nil {
		return nil, err
	}

	if err := s.store.SetRoles(ctx, userID, roleIDs); err != nil {
		return nil, err
	}
	return s.store.Roles(ctx, userID)
}

// List returns the users of the caller's organisation. It needs user:read.
func (s *UserService) List(ctx context.Context) ([]domain.User, error) {
	caller, err := s.authz.Caller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Require(ctx, domain.PermUserRead, caller.OrganisationID, domain.ErrOrganisationNotFound); err != nil {
		return nil, err
	}
	return s.store.ListByOrganisation(ctx, caller.OrganisationID)
}
