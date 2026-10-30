package domain

import (
	"errors"
	"os"
	"regexp"
	"slices"
	"testing"
)

// seededPermissions replays the migrations that seed the permissions table,
// which is the only list of permissions: 011 inserts 21 rows and 012 swaps
// group:manage_permissions for the five role permissions.
func seededPermissions(t *testing.T) []string {
	t.Helper()

	row := regexp.MustCompile(`(?m)^\s*\('([a-z_:]+)',\s*'`)
	var keys []string
	for _, file := range []string{"011_iam_permissions.sql", "012_roles.sql"} {
		sql, err := os.ReadFile("../../migrations/" + file)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range row.FindAllStringSubmatch(string(sql), -1) {
			keys = append(keys, m[1])
		}
	}

	// Migration 012 deletes this one.
	return slices.DeleteFunc(keys, func(k string) bool { return k == "group:manage_permissions" })
}

// Every permission the code checks must be a row the database can grant. A
// constant that is not seeded would make an endpoint nobody can be allowed to use.
func TestCheckedPermissionsAreSeeded(t *testing.T) {
	seeded := seededPermissions(t)
	if len(seeded) != 25 {
		t.Fatalf("migrations seed %d permissions, want 25 (update this test when a migration adds one)", len(seeded))
	}
	if len(UniquePermissions(seeded)) != len(seeded) {
		t.Fatal("duplicate key in the seed")
	}

	checked := []string{
		PermUserRead, PermUserDelete, PermUserAssignGroup,
		PermGroupCreate, PermGroupRead, PermGroupUpdate, PermGroupDelete,
		PermRoleCreate, PermRoleRead, PermRoleUpdate, PermRoleDelete, PermRoleAssign,
		PermOrganisationRead, PermOrganisationEdit, PermOrganisationErase,
	}
	for _, resource := range []string{ResourceRecord, ResourceUser} {
		for _, action := range []string{ActionCreate, ActionRead, ActionUpdate, ActionDelete} {
			if resource == ResourceUser && action != ActionUpdate {
				continue // users are only updated with a scope
			}
			for _, scope := range []string{ScopeOwn, ScopeOrg} {
				checked = append(checked, ScopedKey(resource, action, scope))
			}
		}
	}

	for _, k := range checked {
		if !slices.Contains(seeded, k) {
			t.Errorf("permission %q is checked in the code but not seeded by the migrations", k)
		}
	}
	// Everything seeded is checked somewhere: 15 unscoped + 8 record + 2 user update.
	if len(checked) != len(seeded) {
		t.Errorf("code checks %d permissions, migrations seed %d", len(checked), len(seeded))
	}
}

func TestMemberPermissions(t *testing.T) {
	member := MemberPermissions()
	if len(member) != 8 || len(UniquePermissions(member)) != 8 {
		t.Fatalf("member has %v, want 8 distinct permissions", member)
	}

	seeded := seededPermissions(t)
	for _, k := range member {
		if !slices.Contains(seeded, k) {
			t.Fatalf("member permission %q is not seeded", k)
		}
	}

	// Only admins delete users, assign groups or manage groups/organisation.
	for _, k := range []string{PermUserDelete, PermUserAssignGroup, PermGroupCreate, PermGroupUpdate,
		PermGroupDelete, PermRoleAssign, PermRoleCreate, PermOrganisationEdit, PermOrganisationErase,
		"record:read:org", "record:update:org", "user:update:org"} {
		if slices.Contains(member, k) {
			t.Fatalf("member must not have %q", k)
		}
	}
}

func TestUniquePermissions(t *testing.T) {
	got := UniquePermissions([]string{"user:read", "record:read:org", "user:read"})
	if len(got) != 2 || got[0] != "user:read" || got[1] != "record:read:org" {
		t.Fatalf("got %v, want duplicates removed in first-seen order", got)
	}

	empty := UniquePermissions(nil)
	if empty == nil || len(empty) != 0 {
		t.Fatalf("empty input: %#v, want an empty non-nil slice", empty)
	}
}

func TestGuestPermissions(t *testing.T) {
	guest := GuestPermissions()
	seeded := seededPermissions(t)
	for _, k := range guest {
		if !slices.Contains(seeded, k) {
			t.Fatalf("guest permission %q is not seeded", k)
		}
		if !slices.Contains(MemberPermissions(), k) {
			t.Fatalf("guest has %q, which member does not: guest must be the smaller set", k)
		}
	}
	if len(guest) != 2 {
		t.Fatalf("guest has %v, want record:read:own and organisation:read", guest)
	}
}

func TestIsBuiltinRole(t *testing.T) {
	for name, want := range map[string]bool{"owner": true, "admin": true, "member": true, "guest": true, "staff": false, "": false} {
		if IsBuiltinRole(name) != want {
			t.Fatalf("IsBuiltinRole(%q) != %v", name, want)
		}
	}
}

func TestRoleValidate(t *testing.T) {
	r := Role{OrganisationID: 1, Name: "  Developer "}
	if err := r.Validate(); err != nil || r.Name != "developer" {
		t.Fatalf("got %+v, %v; want the name trimmed and lower-cased", r, err)
	}

	for _, bad := range []Role{{OrganisationID: 1, Name: ""}, {OrganisationID: 1, Name: "9lives"}, {OrganisationID: 1, Name: "a b"}} {
		if err := bad.Validate(); !errors.Is(err, ErrInvalidRole) {
			t.Fatalf("%+v: got %v, want ErrInvalidRole", bad, err)
		}
	}
	if err := (&Role{Name: "dev"}).Validate(); !errors.Is(err, ErrInvalidOrganisationID) {
		t.Fatalf("no organisation: %v", err)
	}
}
