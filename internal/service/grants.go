package service

import (
	"context"
	"slices"

	"crud/internal/authz"
	"crud/internal/domain"
)

// RoleReader is what the services need to look at roles before handing them
// out; the role store satisfies it.
type RoleReader interface {
	GetByID(ctx context.Context, id int64) (domain.Role, error)
	Permissions(ctx context.Context, roleID int64) ([]string, error)
}

// checkRoleIDs rejects a non-positive id and drops duplicates (first-seen
// order, never nil).
func checkRoleIDs(ids []int64) ([]int64, error) {
	unique := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, domain.ErrInvalidRoleID
		}
		if !slices.Contains(unique, id) {
			unique = append(unique, id)
		}
	}
	return unique, nil
}

// changedRoleIDs returns the ids that are in exactly one of old and next: the
// roles a change adds or removes.
func changedRoleIDs(old []domain.Role, next []int64) []int64 {
	var changed []int64
	for _, r := range old {
		if !slices.Contains(next, r.ID) {
			changed = append(changed, r.ID)
		}
	}
	for _, id := range next {
		if !slices.ContainsFunc(old, func(r domain.Role) bool { return r.ID == id }) {
			changed = append(changed, id)
		}
	}
	return changed
}

// changedKeys is changedRoleIDs for permission keys.
func changedKeys(old, next []string) []string {
	var changed []string
	for _, k := range old {
		if !slices.Contains(next, k) {
			changed = append(changed, k)
		}
	}
	for _, k := range next {
		if !slices.Contains(old, k) {
			changed = append(changed, k)
		}
	}
	return changed
}

// ensureGrantable allows the caller to hand out (or take away) the roles: each
// must belong to the caller's organisation, and the caller must hold every
// permission the role has. So an admin cannot give anyone, themselves
// included, more than an admin has (the owner role, for one), and cannot take
// a role away from someone it could not have given it to.
func ensureGrantable(ctx context.Context, az *authz.Authorizer, roles RoleReader, roleIDs []int64) error {
	if len(roleIDs) == 0 {
		return nil
	}
	caller, err := az.Caller(ctx)
	if err != nil {
		return err
	}
	held, err := az.CallerPermissions(ctx)
	if err != nil {
		return err
	}

	for _, id := range roleIDs {
		role, err := roles.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if role.OrganisationID != caller.OrganisationID {
			return domain.ErrRoleNotFound
		}

		perms, err := roles.Permissions(ctx, id)
		if err != nil {
			return err
		}
		for _, p := range perms {
			if !slices.Contains(held, p) {
				return domain.ErrForbidden
			}
		}
	}
	return nil
}
