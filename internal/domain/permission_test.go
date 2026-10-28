package domain

import (
	"os"
	"regexp"
	"slices"
	"testing"
)

// seededPermissions reads the keys that migration 011 inserts into the
// permissions table, which is the only list of permissions.
func seededPermissions(t *testing.T) []string {
	t.Helper()

	sql, err := os.ReadFile("../../migrations/011_iam_permissions.sql")
	if err != nil {
		t.Fatal(err)
	}

	var keys []string
	for _, m := range regexp.MustCompile(`(?m)^\s*\('([a-z_:]+)',\s*'`).FindAllStringSubmatch(string(sql), -1) {
		keys = append(keys, m[1])
	}
	return keys
}

// Every permission the code checks must be a row the database can grant. A
// constant that is not seeded would make an endpoint nobody can be allowed to use.
func TestCheckedPermissionsAreSeeded(t *testing.T) {
	seeded := seededPermissions(t)
	if len(seeded) != 21 {
		t.Fatalf("migration 011 seeds %d permissions, want 21 (update this test when a migration adds one)", len(seeded))
	}
	if len(UniquePermissions(seeded)) != len(seeded) {
		t.Fatal("duplicate key in the seed")
	}

	checked := []string{
		PermUserRead, PermUserDelete, PermUserAssignGroup,
		PermGroupCreate, PermGroupRead, PermGroupUpdate, PermGroupDelete, PermGroupManagePerms,
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
			t.Errorf("permission %q is checked in the code but not seeded by migration 011", k)
		}
	}
	// Everything seeded is checked somewhere: 11 unscoped + 8 record + 2 user update.
	if len(checked) != len(seeded) {
		t.Errorf("code checks %d permissions, migration seeds %d", len(checked), len(seeded))
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
		PermGroupDelete, PermGroupManagePerms, PermOrganisationEdit, PermOrganisationErase,
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

func TestIsBuiltinGroup(t *testing.T) {
	for name, want := range map[string]bool{"admin": true, "member": true, "staff": false, "": false} {
		if IsBuiltinGroup(name) != want {
			t.Fatalf("IsBuiltinGroup(%q) != %v", name, want)
		}
	}
}
