package cache

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"crud/internal/domain"
)

// fakeUsers counts how often the permissions are read from "Postgres".
type fakeUsers struct {
	UserStore
	reads int
}

func (f *fakeUsers) Permissions(context.Context, int64) ([]string, error) {
	f.reads++
	return []string{"user:read"}, nil
}
func (f *fakeUsers) GetByID(_ context.Context, id int64) (domain.User, error) {
	return domain.User{ID: id, GroupID: 7}, nil
}
func (f *fakeUsers) SetRoles(context.Context, int64, []int64) error  { return nil }
func (f *fakeUsers) AssignGroup(context.Context, int64, int64) error { return nil }
func (f *fakeUsers) Delete(context.Context, int64) error             { return nil }

type fakeGroups struct{ GroupStore }

func (fakeGroups) SetRoles(context.Context, int64, []int64) error { return nil }
func (fakeGroups) ListMembers(context.Context, int64) ([]domain.GroupMember, error) {
	return []domain.GroupMember{{ID: 1}, {ID: 2}}, nil
}

type fakeRoles struct{ RoleStore }

func (fakeRoles) SetPermissions(context.Context, int64, []string) error { return nil }
func (fakeRoles) HolderIDs(context.Context, int64) ([]int64, error)     { return []int64{1, 2}, nil }

func newRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return rdb, mr
}

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// Permissions are read on every request, so they are cached per user; every
// change that alters what a user may do drops that key.
func TestUserPermissionsAreCachedAndInvalidated(t *testing.T) {
	rdb, mr := newRedis(t)
	next := &fakeUsers{}
	store := NewUserStore(next, rdb, time.Minute, quiet)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if got, err := store.Permissions(ctx, 5); err != nil || len(got) != 1 {
			t.Fatalf("permissions = %v, %v", got, err)
		}
	}
	if next.reads != 1 {
		t.Fatalf("read from the database %d times, want 1", next.reads)
	}
	if !mr.Exists(userPermissionsKey(5)) {
		t.Fatal("permissions were not cached under perms:user:5")
	}

	changes := map[string]func() error{
		"SetRoles":    func() error { return store.SetRoles(ctx, 5, []int64{1}) },
		"AssignGroup": func() error { return store.AssignGroup(ctx, 5, 8) },
		"Delete":      func() error { return store.Delete(ctx, 5) },
	}
	for name, change := range changes {
		if _, err := store.Permissions(ctx, 5); err != nil {
			t.Fatal(err)
		}
		if !mr.Exists(userPermissionsKey(5)) {
			t.Fatalf("%s: key should be cached before the change", name)
		}
		if err := change(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if mr.Exists(userPermissionsKey(5)) {
			t.Fatalf("%s: the user's cached permissions survived the change", name)
		}
	}
}

// Changing a group's roles changes what each of its members may do.
func TestGroupSetRolesDropsTheMembersPermissions(t *testing.T) {
	rdb, mr := newRedis(t)
	store := NewGroupStore(fakeGroups{}, rdb, time.Minute, quiet)
	ctx := context.Background()

	for _, id := range []int64{1, 2, 3} {
		mr.Set(userPermissionsKey(id), `["user:read"]`)
	}
	if err := store.SetRoles(ctx, 7, []int64{4}); err != nil {
		t.Fatal(err)
	}

	for _, id := range []int64{1, 2} {
		if mr.Exists(userPermissionsKey(id)) {
			t.Fatalf("member %d kept cached permissions", id)
		}
	}
	if !mr.Exists(userPermissionsKey(3)) {
		t.Fatal("a user outside the group lost their cached permissions")
	}
}

// Changing a role's permissions changes what every holder may do.
func TestRoleSetPermissionsDropsTheHoldersPermissions(t *testing.T) {
	rdb, mr := newRedis(t)
	store := NewRoleStore(fakeRoles{}, rdb, time.Minute, quiet)

	for _, id := range []int64{1, 2, 3} {
		mr.Set(userPermissionsKey(id), `["user:read"]`)
	}
	if err := store.SetPermissions(context.Background(), 9, []string{"user:read"}); err != nil {
		t.Fatal(err)
	}

	for _, id := range []int64{1, 2} {
		if mr.Exists(userPermissionsKey(id)) {
			t.Fatalf("holder %d kept cached permissions", id)
		}
	}
	if !mr.Exists(userPermissionsKey(3)) {
		t.Fatal("a user without the role lost their cached permissions")
	}
}
