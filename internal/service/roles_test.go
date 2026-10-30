package service

import (
	"context"
	"errors"
	"slices"
	"testing"

	"crud/internal/domain"
)

// Users: 1 is an owner (every permission), 5 an admin (everything but
// organisation:delete), 2 a member, 4 an auditor (record:read:org only).
func TestRoleAuthorization(t *testing.T) {
	tests := []struct {
		name   string
		caller int64
		op     func(*RoleService, context.Context) error
		want   error
	}{
		{"owner creates a role", 1, func(s *RoleService, c context.Context) error {
			_, e := s.Create(c, domain.Role{Name: "Dev"}, []string{"record:read:org"})
			return e
		}, nil},
		{"member creates a role", 2, func(s *RoleService, c context.Context) error {
			_, e := s.Create(c, domain.Role{Name: "dev"}, nil)
			return e
		}, domain.ErrForbidden},
		{"user without an organisation creates a role", 20, func(s *RoleService, c context.Context) error {
			_, e := s.Create(c, domain.Role{Name: "dev"}, nil)
			return e
		}, domain.ErrOrganisationNotFound},
		{"role with an invalid name", 1, func(s *RoleService, c context.Context) error {
			_, e := s.Create(c, domain.Role{Name: "1 bad"}, nil)
			return e
		}, domain.ErrInvalidRole},
		{"role with an unknown permission", 1, func(s *RoleService, c context.Context) error {
			_, e := s.Create(c, domain.Role{Name: "dev"}, []string{"record:fly:org"})
			return e
		}, domain.ErrInvalidRole},
		{"admin cannot put organisation:delete on a role", 5, func(s *RoleService, c context.Context) error {
			_, e := s.Create(c, domain.Role{Name: "dev"}, []string{domain.PermOrganisationErase})
			return e
		}, domain.ErrForbidden},
		{"admin creates a role with permissions it holds", 5, func(s *RoleService, c context.Context) error {
			_, e := s.Create(c, domain.Role{Name: "dev"}, []string{"group:delete"})
			return e
		}, nil},

		{"member lists roles", 2, func(s *RoleService, c context.Context) error { _, e := s.List(c); return e }, domain.ErrForbidden},
		{"owner lists roles", 1, func(s *RoleService, c context.Context) error { _, e := s.List(c); return e }, nil},
		{"owner reads a role", 1, func(s *RoleService, c context.Context) error { _, e := s.Get(c, 13); return e }, nil},
		{"owner reads another org's role", 1, func(s *RoleService, c context.Context) error { _, e := s.Get(c, 20); return e }, domain.ErrRoleNotFound},
		{"owner lists the catalogue", 1, func(s *RoleService, c context.Context) error { _, e := s.ListPermissions(c); return e }, nil},
		{"member lists the catalogue", 2, func(s *RoleService, c context.Context) error { _, e := s.ListPermissions(c); return e }, domain.ErrForbidden},

		{"owner renames a custom role", 1, func(s *RoleService, c context.Context) error {
			_, e := s.Update(c, domain.Role{ID: 13, Name: "reviewer"})
			return e
		}, nil},
		{"owner renames a built-in role", 1, func(s *RoleService, c context.Context) error {
			_, e := s.Update(c, domain.Role{ID: 12, Name: "staff"})
			return e
		}, domain.ErrBuiltinRole},
		{"member renames a role", 2, func(s *RoleService, c context.Context) error {
			_, e := s.Update(c, domain.Role{ID: 13, Name: "reviewer"})
			return e
		}, domain.ErrForbidden},
		{"owner renames another org's role", 1, func(s *RoleService, c context.Context) error {
			_, e := s.Update(c, domain.Role{ID: 20, Name: "reviewer"})
			return e
		}, domain.ErrRoleNotFound},

		{"owner deletes a custom role", 1, func(s *RoleService, c context.Context) error { return s.Delete(c, 13) }, nil},
		{"owner deletes the owner role", 1, func(s *RoleService, c context.Context) error { return s.Delete(c, 10) }, domain.ErrBuiltinRole},
		{"member deletes a role", 2, func(s *RoleService, c context.Context) error { return s.Delete(c, 13) }, domain.ErrForbidden},
		{"owner deletes another org's role", 1, func(s *RoleService, c context.Context) error { return s.Delete(c, 20) }, domain.ErrRoleNotFound},

		{"owner sets a role's permissions", 1, func(s *RoleService, c context.Context) error {
			_, e := s.SetPermissions(c, 13, []string{"record:read:org", "record:read:org", "user:read"})
			return e
		}, nil},
		{"owner sets permissions of a built-in role", 1, func(s *RoleService, c context.Context) error {
			_, e := s.SetPermissions(c, 11, []string{"user:read"})
			return e
		}, domain.ErrBuiltinRole},
		{"admin adds organisation:delete to a role", 5, func(s *RoleService, c context.Context) error {
			_, e := s.SetPermissions(c, 13, []string{"record:read:org", domain.PermOrganisationErase})
			return e
		}, domain.ErrForbidden},
		{"owner sets an unknown permission", 1, func(s *RoleService, c context.Context) error {
			_, e := s.SetPermissions(c, 13, []string{"nope"})
			return e
		}, domain.ErrInvalidRole},
		{"member sets a role's permissions", 2, func(s *RoleService, c context.Context) error {
			_, e := s.SetPermissions(c, 13, []string{"user:read"})
			return e
		}, domain.ErrForbidden},
		{"invalid role id", 1, func(s *RoleService, c context.Context) error { return s.Delete(c, 0) }, domain.ErrInvalidRoleID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &roleStore{}
			err := tt.op(NewRoleService(store, az), as(tt.caller))
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if tt.want != nil && store.writes != 0 {
				t.Fatal("a refused request must not write")
			}
		})
	}
}

func TestSetPermissionsStoresUniqueKeys(t *testing.T) {
	store := &roleStore{}
	got, err := NewRoleService(store, az).SetPermissions(as(1), 13, []string{"user:read", "record:read:org", "user:read"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"user:read", "record:read:org"}) || !slices.Equal(store.setTo, got) {
		t.Fatalf("returned %v, stored %v; want duplicates removed in first-seen order", got, store.setTo)
	}
}

// Roles can only be handed out by someone who holds everything the role
// allows, so an admin cannot make anyone an owner, and cannot demote one.
func TestSetUserRoles(t *testing.T) {
	tests := []struct {
		name   string
		caller int64
		target int64
		roles  []int64
		want   error
	}{
		{"owner makes a member an admin", 1, 2, []int64{12, 11}, nil},
		{"owner makes a member an owner", 1, 2, []int64{12, 10}, nil},
		{"owner removes all roles of a member", 1, 2, []int64{}, nil},
		{"admin makes a member an admin", 5, 2, []int64{12, 11}, nil},
		{"admin cannot make anyone an owner", 5, 2, []int64{12, 10}, domain.ErrForbidden},
		{"admin cannot make themselves an owner", 5, 5, []int64{11, 10}, domain.ErrForbidden},
		{"admin cannot take the owner role away", 5, 1, []int64{}, domain.ErrForbidden},
		{"admin leaves an owner's roles as they are", 5, 1, []int64{10}, nil},
		{"member cannot assign roles", 2, 3, []int64{11}, domain.ErrForbidden},
		{"auditor cannot assign roles", 4, 3, []int64{11}, domain.ErrForbidden},
		{"a role of another organisation", 1, 2, []int64{20}, domain.ErrRoleNotFound},
		{"an unknown role", 1, 2, []int64{99}, domain.ErrRoleNotFound},
		{"an invalid role id", 1, 2, []int64{-1}, domain.ErrInvalidRoleID},
		{"a user of another organisation", 1, 10, []int64{12}, domain.ErrUserNotFound},
		{"an unknown user", 1, 99, []int64{12}, domain.ErrUserNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &userDirStore{}
			_, err := newUserService(store).SetRoles(as(tt.caller), tt.target, tt.roles)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if tt.want != nil && store.writes != 0 {
				t.Fatal("a refused request must not write")
			}
		})
	}
}

// A user created without roles gets none passed down, which the repository
// turns into the member role; chosen roles need role:assign and permissions
// the caller holds.
func TestCreateUserRoles(t *testing.T) {
	user := domain.User{Name: "Ann", Email: "ann@corp.com", Password: "ann-pass-123"}

	store := &userDirStore{}
	if _, err := newUserService(store).Create(as(1), user, nil); err != nil || len(store.createdBy) != 0 {
		t.Fatalf("default: roles %v, error %v", store.createdBy, err)
	}

	store = &userDirStore{}
	if _, err := newUserService(store).Create(as(1), user, []int64{11, 11, 13}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(store.createdBy, []int64{11, 13}) {
		t.Fatalf("roles passed to the store = %v, want duplicates removed", store.createdBy)
	}

	tests := []struct {
		name   string
		caller int64
		roles  []int64
		want   error
	}{
		{"admin cannot create an owner", 5, []int64{10}, domain.ErrForbidden},
		{"admin creates a guest-like auditor", 5, []int64{13}, nil},
		{"a role of another organisation", 1, []int64{20}, domain.ErrRoleNotFound},
		{"an invalid role id", 1, []int64{0}, domain.ErrInvalidRoleID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &userDirStore{}
			_, err := newUserService(store).Create(as(tt.caller), user, tt.roles)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if tt.want != nil && store.writes != 0 {
				t.Fatal("a refused request must not write")
			}
		})
	}
}

// Putting a user in a group gives them the group's roles, so it needs the
// permissions of those roles; the same goes for taking them out of one.
func TestAssignGroupGrantsTheGroupsRoles(t *testing.T) {
	tests := []struct {
		name   string
		caller int64
		user   int64
		group  int64
		want   error
	}{
		{"owner moves a member to a group", 1, 2, 102, nil},
		{"owner takes a member out of their group", 1, 2, 0, nil},
		{"admin moves a member to a group with the admin role", 5, 2, 100, nil},
		{"invalid group id", 1, 2, -1, domain.ErrInvalidGroupID},
		{"unknown group", 1, 2, 999, domain.ErrGroupNotFound},
		{"member cannot assign", 2, 3, 100, domain.ErrForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &userDirStore{}
			err := newUserService(store).AssignGroup(as(tt.caller), tt.user, tt.group)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if tt.want != nil && store.writes != 0 {
				t.Fatal("a refused request must not write")
			}
		})
	}
}

// An admin cannot delete an owner (the owner role has permissions the admin
// lacks), but can delete a member.
func TestDeleteUserNeedsTheTargetsRolesToBeGrantable(t *testing.T) {
	svc := newUserService(&userDirStore{})

	if err := svc.Delete(as(5), 1); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("admin deletes an owner: %v", err)
	}
	if err := svc.Delete(as(5), 2); err != nil {
		t.Fatalf("admin deletes a member: %v", err)
	}
	if err := svc.Delete(as(1), 5); err != nil {
		t.Fatalf("owner deletes an admin: %v", err)
	}
}
