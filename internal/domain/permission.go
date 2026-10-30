package domain

// A permission is a string "resource:action" or "resource:action:scope". The
// scope says how far it reaches: ScopeOwn is the actor's own item, ScopeOrg is
// any item in the actor's organisation. Resources that have no owner
// (groups, the organisation) have no scope.
//
// The permissions table is the only list of permissions: migration 011 seeds
// it (migration 012 swapped group:manage_permissions for the role ones), a new
// permission is added by a new migration, and role_permissions can only
// reference its rows. The code holds just the names of the permissions its
// checks require, which are the constants below and ScopedKey.
const (
	ResourceRecord       = "record"
	ResourceUser         = "user"
	ResourceGroup        = "group"
	ResourceOrganisation = "organisation"
	ResourceRole         = "role"

	ActionCreate = "create"
	ActionRead   = "read"
	ActionUpdate = "update"
	ActionDelete = "delete"

	ScopeOwn = "own"
	ScopeOrg = "org"
)

// Permissions that are checked on their own (no scope).
const (
	PermUserRead          = "user:read"
	PermUserDelete        = "user:delete"
	PermUserAssignGroup   = "user:assign_group"
	PermGroupCreate       = "group:create"
	PermGroupRead         = "group:read"
	PermGroupUpdate       = "group:update"
	PermGroupDelete       = "group:delete"
	PermRoleCreate        = "role:create"
	PermRoleRead          = "role:read"
	PermRoleUpdate        = "role:update"
	PermRoleDelete        = "role:delete"
	PermRoleAssign        = "role:assign"
	PermOrganisationRead  = "organisation:read"
	PermOrganisationEdit  = "organisation:update"
	PermOrganisationErase = "organisation:delete"
)

// ScopedKey builds the key of a scoped permission, e.g. "record:read:own".
func ScopedKey(resource, action, scope string) string {
	return resource + ":" + action + ":" + scope
}

// Permission is one row of the permissions table, as GET /permissions lists it.
type Permission struct {
	Key         string `json:"key" example:"record:read:own"`
	Resource    string `json:"resource" example:"record"`
	Action      string `json:"action" example:"read"`
	Scope       string `json:"scope,omitempty" example:"own"`
	Description string `json:"description" example:"Read your own records"`
}

// MemberPermissions are what the built-in member role has, and so what a user
// gets by default: manage your own records, read the organisation's users and
// groups, change your own name.
func MemberPermissions() []string {
	return []string{
		ScopedKey(ResourceRecord, ActionCreate, ScopeOwn),
		ScopedKey(ResourceRecord, ActionRead, ScopeOwn),
		ScopedKey(ResourceRecord, ActionUpdate, ScopeOwn),
		ScopedKey(ResourceRecord, ActionDelete, ScopeOwn),
		PermUserRead,
		ScopedKey(ResourceUser, ActionUpdate, ScopeOwn),
		PermGroupRead,
		PermOrganisationRead,
	}
}

// GuestPermissions are what the built-in guest role has: read your own
// records and see the organisation.
func GuestPermissions() []string {
	return []string{
		ScopedKey(ResourceRecord, ActionRead, ScopeOwn),
		PermOrganisationRead,
	}
}

// UniquePermissions returns the keys without duplicates, in first-seen order
// (never nil). Whether each key exists is decided by the database: a key that
// is not in the permissions table is rejected when it is stored.
func UniquePermissions(keys []string) []string {
	out := make([]string, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}
