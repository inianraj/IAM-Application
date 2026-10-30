// Package authz decides what the signed-in caller may do.
//
// Access is permission-based: a user holds roles directly and through their
// optional group, a role is a set of permissions, and every endpoint requires
// one. A user's permissions are the union over all those roles. A
// permission may be scoped: "record:update:own" covers the caller's own
// records, "record:update:org" any record of the caller's organisation.
//
// Resources of another organisation are reported with the resource's own "not
// found" error, so their existence isn't leaked; a resource in the caller's
// organisation that the caller's permissions don't allow gives
// domain.ErrForbidden. A caller without an organisation (legacy rows) has no
// group and no permissions, and may only act on their own resources.
package authz

import (
	"context"
	"slices"

	"crud/internal/auth"
	"crud/internal/domain"
)

// UserGetter loads users; it is the cached store in main.
type UserGetter interface {
	GetByID(ctx context.Context, id int64) (domain.User, error)
}

// PermissionStore returns the permissions of a user (their direct roles plus
// the roles of their group); it is the cached user store in main.
type PermissionStore interface {
	Permissions(ctx context.Context, userID int64) ([]string, error)
}

type Authorizer struct {
	users UserGetter
	perms PermissionStore
}

func New(users UserGetter, perms PermissionStore) *Authorizer {
	return &Authorizer{users: users, perms: perms}
}

// Caller returns the signed-in user, or domain.ErrUnauthenticated when the
// request did not pass the authentication middleware.
func (a *Authorizer) Caller(ctx context.Context) (domain.User, error) {
	p, ok := auth.FromContext(ctx)
	if !ok {
		return domain.User{}, domain.ErrUnauthenticated
	}
	return p.User, nil
}

// permissionsOf returns the caller's permissions, read fresh (through the
// cache) so a changed role, group or permission counts immediately. A caller
// without an organisation has none.
func (a *Authorizer) permissionsOf(ctx context.Context, caller domain.User) ([]string, error) {
	if caller.OrganisationID == 0 {
		return nil, nil
	}
	return a.perms.Permissions(ctx, caller.ID)
}

// CallerPermissions returns every permission the signed-in caller holds. It is
// how a grant can be refused when the caller could not have made it for
// themselves.
func (a *Authorizer) CallerPermissions(ctx context.Context) ([]string, error) {
	caller, err := a.Caller(ctx)
	if err != nil {
		return nil, err
	}
	return a.permissionsOf(ctx, caller)
}

// Require allows the caller to use permission perm on something in
// organisation orgID. Another organisation (or a caller without one) gets
// notFound; a missing permission gets domain.ErrForbidden.
func (a *Authorizer) Require(ctx context.Context, perm string, orgID int64, notFound error) error {
	caller, err := a.Caller(ctx)
	if err != nil {
		return err
	}
	if caller.OrganisationID == 0 || caller.OrganisationID != orgID {
		return notFound
	}

	perms, err := a.permissionsOf(ctx, caller)
	if err != nil {
		return err
	}
	if !slices.Contains(perms, perm) {
		return domain.ErrForbidden
	}
	return nil
}

// RequireScoped allows the caller to do resource:action on something owned by
// ownerID in organisation ownerOrgID: with the ":org" permission on anything
// in their organisation, with ":own" only on their own. Another organisation
// gets notFound, too little permission domain.ErrForbidden.
func (a *Authorizer) RequireScoped(ctx context.Context, resource, action string, ownerID, ownerOrgID int64, notFound error) error {
	caller, err := a.Caller(ctx)
	if err != nil {
		return err
	}

	// A caller without an organisation has no group and so no permissions;
	// they keep access to what is theirs.
	if caller.OrganisationID == 0 {
		if ownerID == caller.ID {
			return nil
		}
		return notFound
	}
	if caller.OrganisationID != ownerOrgID {
		return notFound
	}

	perms, err := a.permissionsOf(ctx, caller)
	if err != nil {
		return err
	}
	if slices.Contains(perms, domain.ScopedKey(resource, action, domain.ScopeOrg)) {
		return nil
	}
	if ownerID == caller.ID && slices.Contains(perms, domain.ScopedKey(resource, action, domain.ScopeOwn)) {
		return nil
	}
	return domain.ErrForbidden
}

// RequireScopedForUser is RequireScoped for a resource owned by the user
// userID (a profile, or that user's records). It loads the user and returns
// them; an unknown user gets notFound.
func (a *Authorizer) RequireScopedForUser(ctx context.Context, resource, action string, userID int64, notFound error) (domain.User, error) {
	target, err := a.users.GetByID(ctx, userID)
	if err != nil {
		return domain.User{}, err
	}
	if err := a.RequireScoped(ctx, resource, action, target.ID, target.OrganisationID, notFound); err != nil {
		return domain.User{}, err
	}
	return target, nil
}

// RequireForUser is Require for an action on the user userID that has no
// own/org scope (deleting them, moving them to another group). It loads the
// user and returns them.
func (a *Authorizer) RequireForUser(ctx context.Context, perm string, userID int64, notFound error) (domain.User, error) {
	target, err := a.users.GetByID(ctx, userID)
	if err != nil {
		return domain.User{}, err
	}
	if err := a.Require(ctx, perm, target.OrganisationID, notFound); err != nil {
		return domain.User{}, err
	}
	return target, nil
}

// RequireSelfOr allows the caller to act on themselves (userID), which also
// covers a caller without an organisation, or otherwise needs permission perm
// in the user's organisation orgID.
func (a *Authorizer) RequireSelfOr(ctx context.Context, perm string, userID, orgID int64, notFound error) error {
	caller, err := a.Caller(ctx)
	if err != nil {
		return err
	}
	if caller.ID == userID {
		return nil
	}
	return a.Require(ctx, perm, orgID, notFound)
}

// Has reports whether the caller holds permission perm. It is for optional
// extras on an endpoint that is already authorized.
func (a *Authorizer) Has(ctx context.Context, perm string) (bool, error) {
	caller, err := a.Caller(ctx)
	if err != nil {
		return false, err
	}
	perms, err := a.permissionsOf(ctx, caller)
	if err != nil {
		return false, err
	}
	return slices.Contains(perms, perm), nil
}
