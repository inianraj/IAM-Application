package authz

import (
	"context"
	"errors"
	"testing"

	"crud/internal/auth"
	"crud/internal/domain"
)

type users map[int64]domain.User

func (u users) GetByID(_ context.Context, id int64) (domain.User, error) {
	if x, ok := u[id]; ok {
		return x, nil
	}
	return domain.User{}, domain.ErrUserNotFound
}

type perms map[int64][]string

func (p perms) Permissions(_ context.Context, groupID int64) ([]string, error) {
	return p[groupID], nil
}

// Org 1: user 1 admin (group 100), user 2 member (101), user 3 auditor (102).
// Org 2: user 10 admin (200). User 20 has no organisation.
var (
	directory = users{
		1:  {ID: 1, OrganisationID: 1, GroupID: 100},
		2:  {ID: 2, OrganisationID: 1, GroupID: 101},
		3:  {ID: 3, OrganisationID: 1, GroupID: 102},
		10: {ID: 10, OrganisationID: 2, GroupID: 200},
		20: {ID: 20},
	}
	groups = perms{
		100: allPermissions,
		101: domain.MemberPermissions(),
		102: {"record:read:org"},
		200: allPermissions,
	}
	az = New(directory, groups)
)

// allPermissions is what an admin group holds: every row migration 011 seeds.
var allPermissions = []string{
	"record:create:own", "record:create:org", "record:read:own", "record:read:org",
	"record:update:own", "record:update:org", "record:delete:own", "record:delete:org",
	"user:read", "user:update:own", "user:update:org", "user:delete", "user:assign_group",
	"group:create", "group:read", "group:update", "group:delete", "group:manage_permissions",
	"organisation:read", "organisation:update", "organisation:delete",
}

func as(userID int64) context.Context {
	return auth.WithPrincipal(context.Background(), auth.Principal{User: directory[userID]})
}

var errNotFound = errors.New("not found")

func TestRequireNeedsASession(t *testing.T) {
	if err := az.Require(context.Background(), domain.PermGroupRead, 1, errNotFound); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("got %v", err)
	}
	if err := az.RequireScoped(context.Background(), "record", "read", 1, 1, errNotFound); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("got %v", err)
	}
}

func TestRequire(t *testing.T) {
	tests := []struct {
		name   string
		caller int64
		perm   string
		org    int64
		want   error
	}{
		{"member holds group:read", 2, domain.PermGroupRead, 1, nil},
		{"member lacks group:create", 2, domain.PermGroupCreate, 1, domain.ErrForbidden},
		{"admin holds group:create", 1, domain.PermGroupCreate, 1, nil},
		{"admin of another organisation", 1, domain.PermGroupCreate, 2, errNotFound},
		{"auditor lacks group:read", 3, domain.PermGroupRead, 1, domain.ErrForbidden},
		{"no organisation", 20, domain.PermGroupRead, 1, errNotFound},
		{"no organisation, organisation 0", 20, domain.PermGroupRead, 0, errNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := az.Require(as(tt.caller), tt.perm, tt.org, errNotFound); !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestRequireScoped(t *testing.T) {
	tests := []struct {
		name     string
		caller   int64
		action   string
		ownerID  int64
		ownerOrg int64
		want     error
	}{
		{"member reads own", 2, domain.ActionRead, 2, 1, nil},
		{"member reads a colleague's", 2, domain.ActionRead, 3, 1, domain.ErrForbidden},
		{"admin reads a colleague's", 1, domain.ActionRead, 2, 1, nil},
		{"auditor reads a colleague's", 3, domain.ActionRead, 2, 1, nil},
		{"auditor updates a colleague's", 3, domain.ActionUpdate, 2, 1, domain.ErrForbidden},
		{"auditor updates own: no own permission either", 3, domain.ActionUpdate, 3, 1, domain.ErrForbidden},
		{"admin updates a colleague's", 1, domain.ActionUpdate, 2, 1, nil},
		{"admin of another organisation", 1, domain.ActionRead, 10, 2, errNotFound},
		{"member of another organisation", 2, domain.ActionRead, 10, 2, errNotFound},
		{"no organisation, own", 20, domain.ActionRead, 20, 0, nil},
		{"no organisation, someone else's", 20, domain.ActionRead, 21, 0, errNotFound},
		{"no organisation, a member's", 20, domain.ActionRead, 2, 1, errNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := az.RequireScoped(as(tt.caller), domain.ResourceRecord, tt.action, tt.ownerID, tt.ownerOrg, errNotFound)
			if !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestRequireForUserAndScopedForUser(t *testing.T) {
	// An unknown target is reported by the user store, before any permission.
	if _, err := az.RequireForUser(as(1), domain.PermUserDelete, 99, errNotFound); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("unknown user: %v", err)
	}

	got, err := az.RequireForUser(as(1), domain.PermUserDelete, 2, errNotFound)
	if err != nil || got.ID != 2 {
		t.Fatalf("admin deletes member: %+v, %v", got, err)
	}
	if _, err := az.RequireForUser(as(2), domain.PermUserDelete, 3, errNotFound); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("member deletes colleague: %v", err)
	}
	if _, err := az.RequireForUser(as(1), domain.PermUserDelete, 10, errNotFound); !errors.Is(err, errNotFound) {
		t.Fatalf("admin deletes user of another organisation: %v", err)
	}

	if _, err := az.RequireScopedForUser(as(2), domain.ResourceUser, domain.ActionUpdate, 2, errNotFound); err != nil {
		t.Fatalf("member updates self: %v", err)
	}
	if _, err := az.RequireScopedForUser(as(2), domain.ResourceUser, domain.ActionUpdate, 3, errNotFound); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("member updates colleague: %v", err)
	}
}

func TestRequireSelfOr(t *testing.T) {
	// A user may always read themselves, even with no organisation or no permission.
	for _, id := range []int64{2, 3, 20} {
		if err := az.RequireSelfOr(as(id), domain.PermUserRead, id, directory[id].OrganisationID, errNotFound); err != nil {
			t.Fatalf("user %d reads self: %v", id, err)
		}
	}
	if err := az.RequireSelfOr(as(2), domain.PermUserRead, 3, 1, errNotFound); err != nil {
		t.Fatalf("member holds user:read: %v", err)
	}
	if err := az.RequireSelfOr(as(3), domain.PermUserRead, 2, 1, errNotFound); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("auditor lacks user:read: %v", err)
	}
	if err := az.RequireSelfOr(as(2), domain.PermUserRead, 10, 2, errNotFound); !errors.Is(err, errNotFound) {
		t.Fatalf("another organisation: %v", err)
	}
}

func TestHas(t *testing.T) {
	if ok, err := az.Has(as(1), domain.PermGroupManagePerms); err != nil || !ok {
		t.Fatalf("admin: %v, %v", ok, err)
	}
	if ok, err := az.Has(as(2), domain.PermGroupManagePerms); err != nil || ok {
		t.Fatalf("member: %v, %v", ok, err)
	}
	if ok, err := az.Has(as(20), domain.PermGroupRead); err != nil || ok {
		t.Fatalf("no organisation: %v, %v", ok, err)
	}
}
