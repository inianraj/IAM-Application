package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"crud/internal/auth"
	"crud/internal/authz"
	"crud/internal/domain"
)

// Org 1: admin (user 1), members (users 2 and 3). Org 2: admin (user 10).
// Users 20 and 21 have no organisation.
type directory struct {
	users  map[int64]domain.User
	groups map[int64][]string // group id -> permissions
}

func (d directory) userGetter() authz.UserGetter           { return userLookup(d.users) }
func (d directory) permissionStore() authz.PermissionStore { return permLookup(d.groups) }

type userLookup map[int64]domain.User

func (m userLookup) GetByID(_ context.Context, id int64) (domain.User, error) {
	if u, ok := m[id]; ok {
		return u, nil
	}
	return domain.User{}, domain.ErrUserNotFound
}

type permLookup map[int64][]string

func (m permLookup) Permissions(_ context.Context, groupID int64) ([]string, error) {
	return m[groupID], nil
}

var dir = directory{
	users: map[int64]domain.User{
		1:  {ID: 1, OrganisationID: 1, GroupID: 100},
		2:  {ID: 2, OrganisationID: 1, GroupID: 101},
		3:  {ID: 3, OrganisationID: 1, GroupID: 101},
		4:  {ID: 4, OrganisationID: 1, GroupID: 102}, // auditor: record:read:org only
		10: {ID: 10, OrganisationID: 2, GroupID: 200},
		20: {ID: 20}, // no organisation
		21: {ID: 21}, // no organisation
	},
	groups: map[int64][]string{
		100: allPermissions,
		101: domain.MemberPermissions(),
		102: {"record:read:org"},
		200: allPermissions,
	},
}

var az = authz.New(dir.userGetter(), dir.permissionStore())

// allPermissions is what an admin group holds: every row migration 011 seeds.
var allPermissions = []string{
	"record:create:own", "record:create:org", "record:read:own", "record:read:org",
	"record:update:own", "record:update:org", "record:delete:own", "record:delete:org",
	"user:read", "user:update:own", "user:update:org", "user:delete", "user:assign_group",
	"group:create", "group:read", "group:update", "group:delete", "group:manage_permissions",
	"organisation:read", "organisation:update", "organisation:delete",
}

func as(userID int64) context.Context {
	return auth.WithPrincipal(context.Background(), auth.Principal{User: dir.users[userID]})
}

// --- records ---

type recordStore struct {
	records map[int64]domain.Task
	writes  int
}

func (r *recordStore) Create(_ context.Context, t domain.Task) (domain.Task, error) {
	r.writes++
	t.ID = 50
	return t, nil
}
func (r *recordStore) GetByID(_ context.Context, id int64) (domain.Task, error) {
	if t, ok := r.records[id]; ok {
		return t, nil
	}
	return domain.Task{}, domain.ErrTaskNotFound
}
func (r *recordStore) ListByUser(context.Context, int64) ([]domain.Task, error) {
	return []domain.Task{}, nil
}
func (r *recordStore) ListByOrganisation(context.Context, int64) ([]domain.Task, error) {
	return []domain.Task{}, nil
}
func (r *recordStore) Update(_ context.Context, t domain.Task) (domain.Task, error) {
	r.writes++
	return t, nil
}
func (r *recordStore) Delete(context.Context, int64) error { r.writes++; return nil }

func TestRecordAuthorization(t *testing.T) {
	// Record 1 belongs to member 2; record 2 belongs to org 2's admin.
	newSvc := func() (*TaskService, *recordStore) {
		store := &recordStore{records: map[int64]domain.Task{
			1: {ID: 1, UserID: 2, OrganisationID: 1, Title: "mine"},
			2: {ID: 2, UserID: 10, OrganisationID: 2, Title: "other org"},
		}}
		return NewTaskService(store, az), store
	}

	tests := []struct {
		name   string
		caller int64
		op     func(*TaskService, context.Context) error
		want   error
	}{
		{"owner reads", 2, func(s *TaskService, c context.Context) error { _, e := s.Get(c, 1); return e }, nil},
		{"org admin reads a member's record", 1, func(s *TaskService, c context.Context) error { _, e := s.Get(c, 1); return e }, nil},
		{"another member reads it", 3, func(s *TaskService, c context.Context) error { _, e := s.Get(c, 1); return e }, domain.ErrForbidden},
		{"other org reads it", 10, func(s *TaskService, c context.Context) error { _, e := s.Get(c, 1); return e }, domain.ErrTaskNotFound},
		{"owner updates", 2, func(s *TaskService, c context.Context) error {
			_, e := s.Update(c, domain.Task{ID: 1, Title: "new"})
			return e
		}, nil},
		{"another member updates", 3, func(s *TaskService, c context.Context) error {
			_, e := s.Update(c, domain.Task{ID: 1, Title: "new"})
			return e
		}, domain.ErrForbidden},
		{"other org deletes", 10, func(s *TaskService, c context.Context) error { return s.Delete(c, 1) }, domain.ErrTaskNotFound},
		{"member deletes own", 2, func(s *TaskService, c context.Context) error { return s.Delete(c, 1) }, nil},
		{"member creates for self", 2, func(s *TaskService, c context.Context) error {
			_, e := s.Create(c, domain.Task{UserID: 2, Title: "t"})
			return e
		}, nil},
		{"member creates for someone else", 2, func(s *TaskService, c context.Context) error {
			_, e := s.Create(c, domain.Task{UserID: 3, Title: "t"})
			return e
		}, domain.ErrForbidden},
		{"admin creates for a member", 1, func(s *TaskService, c context.Context) error {
			_, e := s.Create(c, domain.Task{UserID: 2, Title: "t"})
			return e
		}, nil},
		{"create for a user of another org", 1, func(s *TaskService, c context.Context) error {
			_, e := s.Create(c, domain.Task{UserID: 10, Title: "t"})
			return e
		}, domain.ErrUserNotFound},
		{"list own records", 2, func(s *TaskService, c context.Context) error { _, e := s.ListByUser(c, 2); return e }, nil},
		{"member lists another member's", 2, func(s *TaskService, c context.Context) error { _, e := s.ListByUser(c, 3); return e }, domain.ErrForbidden},
		{"admin lists a member's", 1, func(s *TaskService, c context.Context) error { _, e := s.ListByUser(c, 3); return e }, nil},
		{"list another org's user", 1, func(s *TaskService, c context.Context) error { _, e := s.ListByUser(c, 10); return e }, domain.ErrUserNotFound},
		{"no session", 0, func(s *TaskService, _ context.Context) error {
			_, e := s.Get(context.Background(), 1)
			return e
		}, domain.ErrUnauthenticated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store := newSvc()
			ctx := context.Background()
			if tt.caller != 0 {
				ctx = as(tt.caller)
			}

			err := tt.op(svc, ctx)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if tt.want != nil && store.writes != 0 {
				t.Fatal("a refused request must not write")
			}
		})
	}
}

// --- groups ---

// Groups: 100 is org 1's built-in admin, 101 a custom group, 200 org 2's admin.
var testGroups = map[int64]domain.Group{
	100: {ID: 100, OrganisationID: 1, Name: "admin"},
	101: {ID: 101, OrganisationID: 1, Name: "staff"},
	200: {ID: 200, OrganisationID: 2, Name: "admin"},
}

// knownPermissions mimics the database: only keys in the permissions table can
// be stored, anything else is rejected as an invalid group.
func knownPermissions(keys []string) error {
	for _, k := range keys {
		if !slices.Contains(allPermissions, k) {
			return fmt.Errorf("%w: unknown permission %q", domain.ErrInvalidGroup, k)
		}
	}
	return nil
}

type groupStore struct {
	groups      map[int64]domain.Group
	writes      int
	createdWith []string
	setWith     []string
}

func (g *groupStore) Create(_ context.Context, grp domain.Group, permissions []string) (domain.Group, error) {
	if err := knownPermissions(permissions); err != nil {
		return domain.Group{}, err
	}
	g.writes++
	g.createdWith = permissions
	grp.ID = 300
	return grp, nil
}
func (g *groupStore) GetByID(_ context.Context, id int64) (domain.Group, error) {
	if grp, ok := g.groups[id]; ok {
		return grp, nil
	}
	return domain.Group{}, domain.ErrGroupNotFound
}
func (g *groupStore) ListByOrganisation(context.Context, int64) ([]domain.Group, error) {
	return []domain.Group{}, nil
}
func (g *groupStore) Update(_ context.Context, grp domain.Group) (domain.Group, error) {
	g.writes++
	return grp, nil
}
func (g *groupStore) Delete(context.Context, int64) error { g.writes++; return nil }
func (g *groupStore) ListMembers(context.Context, int64) ([]domain.GroupMember, error) {
	return []domain.GroupMember{}, nil
}
func (g *groupStore) Permissions(_ context.Context, id int64) ([]string, error) {
	return dir.groups[id], nil
}
func (g *groupStore) SetPermissions(_ context.Context, _ int64, permissions []string) error {
	if err := knownPermissions(permissions); err != nil {
		return err
	}
	g.writes++
	g.setWith = permissions
	return nil
}
func (g *groupStore) ListPermissions(context.Context) ([]domain.Permission, error) {
	return []domain.Permission{{Key: "user:read", Resource: "user", Action: "read", Description: "Read users"}}, nil
}

func TestGroupAuthorization(t *testing.T) {
	tests := []struct {
		name   string
		caller int64
		op     func(*GroupService, context.Context) error
		want   error
	}{
		{"member reads a group of the org", 2, func(s *GroupService, c context.Context) error { _, e := s.Get(c, 101); return e }, nil},
		{"member reads another org's group", 2, func(s *GroupService, c context.Context) error { _, e := s.Get(c, 200); return e }, domain.ErrGroupNotFound},
		{"member lists members of own org's group", 2, func(s *GroupService, c context.Context) error { _, e := s.ListMembers(c, 101); return e }, nil},
		{"member lists members of another org's group", 2, func(s *GroupService, c context.Context) error { _, e := s.ListMembers(c, 200); return e }, domain.ErrGroupNotFound},
		{"member lists own org's groups", 2, func(s *GroupService, c context.Context) error { _, e := s.ListByOrganisation(c, 1); return e }, nil},
		{"member lists another org's groups", 2, func(s *GroupService, c context.Context) error { _, e := s.ListByOrganisation(c, 2); return e }, domain.ErrOrganisationNotFound},
		{"admin creates a group", 1, func(s *GroupService, c context.Context) error {
			_, e := s.Create(c, domain.Group{OrganisationID: 1, Name: "ops"}, nil)
			return e
		}, nil},
		{"member creates a group", 2, func(s *GroupService, c context.Context) error {
			_, e := s.Create(c, domain.Group{OrganisationID: 1, Name: "ops"}, nil)
			return e
		}, domain.ErrForbidden},
		{"admin creates a group in another org", 1, func(s *GroupService, c context.Context) error {
			_, e := s.Create(c, domain.Group{OrganisationID: 2, Name: "ops"}, nil)
			return e
		}, domain.ErrOrganisationNotFound},
		{"admin renames a group", 1, func(s *GroupService, c context.Context) error {
			_, e := s.Update(c, domain.Group{ID: 101, OrganisationID: 1, Name: "crew"})
			return e
		}, nil},
		{"member renames a group", 2, func(s *GroupService, c context.Context) error {
			_, e := s.Update(c, domain.Group{ID: 101, OrganisationID: 1, Name: "crew"})
			return e
		}, domain.ErrForbidden},
		{"admin moves a group to another org", 1, func(s *GroupService, c context.Context) error {
			_, e := s.Update(c, domain.Group{ID: 101, OrganisationID: 2, Name: "crew"})
			return e
		}, domain.ErrOrganisationNotFound},
		{"admin edits another org's group", 1, func(s *GroupService, c context.Context) error {
			_, e := s.Update(c, domain.Group{ID: 200, OrganisationID: 2, Name: "crew"})
			return e
		}, domain.ErrGroupNotFound},
		{"admin deletes a group", 1, func(s *GroupService, c context.Context) error { return s.Delete(c, 101) }, nil},
		{"member deletes a group", 2, func(s *GroupService, c context.Context) error { return s.Delete(c, 101) }, domain.ErrForbidden},
		{"admin deletes another org's group", 1, func(s *GroupService, c context.Context) error { return s.Delete(c, 200) }, domain.ErrGroupNotFound},
		{"admin renames the built-in admin group", 1, func(s *GroupService, c context.Context) error {
			_, e := s.Update(c, domain.Group{ID: 100, OrganisationID: 1, Name: "boss"})
			return e
		}, domain.ErrBuiltinGroup},
		{"admin deletes the built-in admin group", 1, func(s *GroupService, c context.Context) error { return s.Delete(c, 100) }, domain.ErrBuiltinGroup},
		{"member reads a group's permissions", 2, func(s *GroupService, c context.Context) error { _, e := s.GetPermissions(c, 101); return e }, nil},
		{"member reads another org's group permissions", 2, func(s *GroupService, c context.Context) error { _, e := s.GetPermissions(c, 200); return e }, domain.ErrGroupNotFound},
		{"admin sets a group's permissions", 1, func(s *GroupService, c context.Context) error {
			_, e := s.SetPermissions(c, 101, []string{"record:read:org", "record:read:org"})
			return e
		}, nil},
		{"member sets a group's permissions", 2, func(s *GroupService, c context.Context) error {
			_, e := s.SetPermissions(c, 101, []string{"record:read:org"})
			return e
		}, domain.ErrForbidden},
		{"admin sets permissions of the built-in member group", 1, func(s *GroupService, c context.Context) error {
			_, e := s.SetPermissions(c, 100, []string{"record:read:org"})
			return e
		}, domain.ErrBuiltinGroup},
		{"admin sets an unknown permission", 1, func(s *GroupService, c context.Context) error {
			_, e := s.SetPermissions(c, 101, []string{"record:fly:org"})
			return e
		}, domain.ErrInvalidGroup},
		{"admin sets another org's group", 1, func(s *GroupService, c context.Context) error {
			_, e := s.SetPermissions(c, 200, []string{"record:read:org"})
			return e
		}, domain.ErrGroupNotFound},
		{"admin lists the permission catalogue", 1, func(s *GroupService, c context.Context) error { _, e := s.ListPermissions(c); return e }, nil},
		{"member lists the permission catalogue", 2, func(s *GroupService, c context.Context) error { _, e := s.ListPermissions(c); return e }, domain.ErrForbidden},
		{"user without an organisation lists the catalogue", 20, func(s *GroupService, c context.Context) error { _, e := s.ListPermissions(c); return e }, domain.ErrOrganisationNotFound},
		{"admin creates a group with chosen permissions", 1, func(s *GroupService, c context.Context) error {
			_, e := s.Create(c, domain.Group{OrganisationID: 1, Name: "auditors"}, []string{"record:read:org"})
			return e
		}, nil},
		{"admin creates a group with an unknown permission", 1, func(s *GroupService, c context.Context) error {
			_, e := s.Create(c, domain.Group{OrganisationID: 1, Name: "auditors"}, []string{"nope"})
			return e
		}, domain.ErrInvalidGroup},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &groupStore{groups: testGroups}
			err := tt.op(NewGroupService(store, az), as(tt.caller))
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if tt.want != nil && store.writes != 0 {
				t.Fatal("a refused request must not write")
			}
		})
	}
}

// --- users and organisations ---

func TestUserAuthorization(t *testing.T) {
	tests := []struct {
		name   string
		caller int64
		op     func(*UserService, context.Context) error
		want   error
	}{
		{"read a colleague", 2, func(s *UserService, c context.Context) error { _, e := s.Get(c, 3); return e }, nil},
		{"read a user of another org", 2, func(s *UserService, c context.Context) error { _, e := s.Get(c, 10); return e }, domain.ErrUserNotFound},
		{"update self", 2, func(s *UserService, c context.Context) error {
			_, e := s.Update(c, domain.User{ID: 2, Name: "Bo"})
			return e
		}, nil},
		{"admin updates a member", 1, func(s *UserService, c context.Context) error {
			_, e := s.Update(c, domain.User{ID: 2, Name: "Bo"})
			return e
		}, nil},
		{"member updates a colleague", 2, func(s *UserService, c context.Context) error {
			_, e := s.Update(c, domain.User{ID: 3, Name: "Bo"})
			return e
		}, domain.ErrForbidden},
		{"admin updates a user of another org", 1, func(s *UserService, c context.Context) error {
			_, e := s.Update(c, domain.User{ID: 10, Name: "Bo"})
			return e
		}, domain.ErrUserNotFound},
		{"member deletes a colleague", 2, func(s *UserService, c context.Context) error { return s.Delete(c, 3) }, domain.ErrForbidden},
		{"admin deletes a member", 1, func(s *UserService, c context.Context) error { return s.Delete(c, 3) }, nil},
		{"admin assigns a group", 1, func(s *UserService, c context.Context) error { return s.AssignGroup(c, 2, 100) }, nil},
		{"member assigns themselves to admin", 2, func(s *UserService, c context.Context) error { return s.AssignGroup(c, 2, 100) }, domain.ErrForbidden},
		{"admin assigns a user of another org", 1, func(s *UserService, c context.Context) error { return s.AssignGroup(c, 10, 200) }, domain.ErrUserNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := userDirStore{}
			err := tt.op(NewUserService(store, az), as(tt.caller))
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

// orgStore records which organisation id the service wrote to, so the tests can
// check that only the caller's own organisation is ever touched.
type orgStore struct {
	writes  int
	wroteID int64
}

func (o *orgStore) Create(_ context.Context, org domain.Organisation, admin domain.User) (domain.Organisation, domain.User, error) {
	return org, admin, nil
}
func (o *orgStore) GetByID(_ context.Context, id int64) (domain.Organisation, error) {
	return domain.Organisation{ID: id, Name: "org"}, nil
}
func (o *orgStore) List(context.Context) ([]domain.Organisation, error) {
	return []domain.Organisation{}, nil
}
func (o *orgStore) Update(_ context.Context, org domain.Organisation) (domain.Organisation, error) {
	o.writes++
	o.wroteID = org.ID
	return org, nil
}
func (o *orgStore) Delete(_ context.Context, id int64) (domain.DeletedOrganisation, error) {
	o.writes++
	o.wroteID = id
	return domain.DeletedOrganisation{}, nil
}

// Get, update and delete take no id: they act on the caller's own organisation.
func TestOrganisationAuthorization(t *testing.T) {
	tests := []struct {
		name      string
		caller    int64
		op        func(*OrganisationService, context.Context) error
		want      error
		wantWrite int64 // organisation id the store must have been written with
	}{
		{"member reads own org", 2, func(s *OrganisationService, c context.Context) error { _, e := s.GetCurrent(c); return e }, nil, 0},
		{"admin of another org reads their own", 10, func(s *OrganisationService, c context.Context) error { _, e := s.GetCurrent(c); return e }, nil, 0},
		{"user without an org reads", 20, func(s *OrganisationService, c context.Context) error { _, e := s.GetCurrent(c); return e }, domain.ErrOrganisationNotFound, 0},
		{"admin renames own org", 1, func(s *OrganisationService, c context.Context) error { _, e := s.UpdateCurrent(c, "New"); return e }, nil, 1},
		{"admin of org 2 renames: org 2 is changed, never org 1", 10, func(s *OrganisationService, c context.Context) error { _, e := s.UpdateCurrent(c, "New"); return e }, nil, 2},
		{"member renames", 2, func(s *OrganisationService, c context.Context) error { _, e := s.UpdateCurrent(c, "New"); return e }, domain.ErrForbidden, 0},
		{"user without an org renames", 20, func(s *OrganisationService, c context.Context) error { _, e := s.UpdateCurrent(c, "New"); return e }, domain.ErrOrganisationNotFound, 0},
		{"admin renames to a blank name", 1, func(s *OrganisationService, c context.Context) error { _, e := s.UpdateCurrent(c, " "); return e }, domain.ErrInvalidOrganisation, 0},
		{"member deletes", 2, func(s *OrganisationService, c context.Context) error { _, e := s.DeleteCurrent(c, "org"); return e }, domain.ErrForbidden, 0},
		{"user without an org deletes", 20, func(s *OrganisationService, c context.Context) error { _, e := s.DeleteCurrent(c, "org"); return e }, domain.ErrOrganisationNotFound, 0},
		{"admin deletes with the wrong name", 1, func(s *OrganisationService, c context.Context) error { _, e := s.DeleteCurrent(c, "Org"); return e }, domain.ErrInvalidOrganisation, 0},
		{"admin deletes with no name", 1, func(s *OrganisationService, c context.Context) error { _, e := s.DeleteCurrent(c, ""); return e }, domain.ErrInvalidOrganisation, 0},
		{"admin deletes own org with the right name", 1, func(s *OrganisationService, c context.Context) error { _, e := s.DeleteCurrent(c, " org "); return e }, nil, 1},
		{"admin of org 2 deletes: org 2, never org 1", 10, func(s *OrganisationService, c context.Context) error { _, e := s.DeleteCurrent(c, "org"); return e }, nil, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &orgStore{}
			err := tt.op(NewOrganisationService(store, az), as(tt.caller))
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if store.wroteID != tt.wantWrite {
				t.Fatalf("store written with organisation %d, want %d (writes = %d)", store.wroteID, tt.wantWrite, store.writes)
			}
			if tt.wantWrite == 0 && store.writes != 0 {
				t.Fatal("a refused request must not write")
			}
		})
	}
}

// Without a session there is no current organisation.
func TestOrganisationCurrentNeedsASession(t *testing.T) {
	svc := NewOrganisationService(&orgStore{}, az)

	if _, err := svc.GetCurrent(context.Background()); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("get: %v", err)
	}
	if _, err := svc.UpdateCurrent(context.Background(), "x"); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("update: %v", err)
	}
	if _, err := svc.DeleteCurrent(context.Background(), "org"); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("delete: %v", err)
	}
}

// Creating and listing organisations is public: it works with no session.
func TestOrganisationCreateAndListArePublic(t *testing.T) {
	svc := NewOrganisationService(&orgStore{}, az)

	if _, _, err := svc.Create(context.Background(), domain.Organisation{Name: "Acme"}, domain.User{Name: "Ann", Email: "ann@corp.com"}); err != nil {
		t.Fatalf("create without a session: %v", err)
	}
	if _, err := svc.List(context.Background()); err != nil {
		t.Fatalf("list without a session: %v", err)
	}
}

// userDirStore is a UserStore backed by the shared test directory.
type userDirStore struct{}

func (userDirStore) Create(_ context.Context, u domain.User) (domain.User, error) { return u, nil }
func (userDirStore) GetByID(_ context.Context, id int64) (domain.User, error) {
	if u, ok := dir.users[id]; ok {
		return u, nil
	}
	return domain.User{}, domain.ErrUserNotFound
}
func (userDirStore) Update(_ context.Context, u domain.User) (domain.User, error) { return u, nil }
func (userDirStore) Delete(context.Context, int64) error                          { return nil }
func (userDirStore) AssignGroup(context.Context, int64, int64) error              { return nil }
