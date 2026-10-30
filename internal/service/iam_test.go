package service

import (
	"context"
	"errors"
	"testing"

	"crud/internal/domain"
)

// An "auditor" (user 4, group 102) only holds record:read:org: it can read every
// record of its organisation and change nothing. Records: 1 belongs to member 2
// (org 1), 2 to org 2's admin.
func TestAuditorGroupCanReadButNotChangeRecords(t *testing.T) {
	store := &recordStore{records: map[int64]domain.Task{
		1: {ID: 1, UserID: 2, OrganisationID: 1, Title: "colleague's"},
		2: {ID: 2, UserID: 10, OrganisationID: 2, Title: "other org"},
	}}
	svc := NewTaskService(store, az)
	auditor := as(4)

	if _, err := svc.Get(auditor, 1); err != nil {
		t.Fatalf("read a colleague's record: %v", err)
	}
	if _, err := svc.Get(auditor, 2); !errors.Is(err, domain.ErrTaskNotFound) {
		t.Fatalf("read another organisation's record: %v", err)
	}
	if _, err := svc.ListByUser(auditor, 2); err != nil {
		t.Fatalf("list a colleague's records: %v", err)
	}
	if _, err := svc.ListByOrganisation(auditor); err != nil {
		t.Fatalf("list the organisation's records: %v", err)
	}

	if _, err := svc.Update(auditor, domain.Task{ID: 1, Title: "x"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("update: %v", err)
	}
	if err := svc.Delete(auditor, 1); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Create(auditor, domain.Task{UserID: 4, Title: "x"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("create: %v", err)
	}
	if store.writes != 0 {
		t.Fatalf("a refused request wrote %d times", store.writes)
	}
}

func TestListRecordsScopes(t *testing.T) {
	svc := NewTaskService(&recordStore{records: map[int64]domain.Task{}}, az)

	tests := []struct {
		name   string
		caller int64
		op     func(context.Context) error
		want   error
	}{
		{"member lists own", 2, func(c context.Context) error { _, e := svc.ListMine(c); return e }, nil},
		{"member lists the organisation", 2, func(c context.Context) error { _, e := svc.ListByOrganisation(c); return e }, domain.ErrForbidden},
		{"member lists a colleague", 2, func(c context.Context) error { _, e := svc.ListByUser(c, 3); return e }, domain.ErrForbidden},
		{"admin lists the organisation", 1, func(c context.Context) error { _, e := svc.ListByOrganisation(c); return e }, nil},
		{"admin lists a colleague", 1, func(c context.Context) error { _, e := svc.ListByUser(c, 2); return e }, nil},
		{"admin lists a user of another organisation", 1, func(c context.Context) error { _, e := svc.ListByUser(c, 10); return e }, domain.ErrUserNotFound},
		{"user without an organisation lists own", 20, func(c context.Context) error { _, e := svc.ListMine(c); return e }, nil},
		{"user without an organisation lists the organisation", 20, func(c context.Context) error { _, e := svc.ListByOrganisation(c); return e }, domain.ErrOrganisationNotFound},
		{"no session", 0, func(c context.Context) error { _, e := svc.ListMine(c); return e }, domain.ErrUnauthenticated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.caller != 0 {
				ctx = as(tt.caller)
			}
			if err := tt.op(ctx); !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
		})
	}
}

// A record is stored with its owner's organisation, and only someone who may
// create records for that owner can create it.
func TestCreateRecordStampsTheOwnersOrganisation(t *testing.T) {
	store := &recordStore{records: map[int64]domain.Task{}}
	svc := NewTaskService(store, az)

	created, err := svc.Create(as(1), domain.Task{UserID: 2, Title: "for a member"})
	if err != nil {
		t.Fatalf("admin creates for a member: %v", err)
	}
	if created.OrganisationID != 1 || created.UserID != 2 {
		t.Fatalf("stored record = %+v, want owner 2 in organisation 1", created)
	}

	if _, err := svc.Create(as(2), domain.Task{UserID: 3, Title: "x"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("member creates for a colleague: %v", err)
	}
	if _, err := svc.Create(as(2), domain.Task{UserID: 2, Title: "mine"}); err != nil {
		t.Fatalf("member creates own: %v", err)
	}
	if _, err := svc.Create(as(1), domain.Task{UserID: 10, Title: "x"}); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("admin creates for another organisation's user: %v", err)
	}
}

// A group is created with no roles unless some are chosen; choosing roles
// needs role:assign as well as group:create, and duplicates are dropped.
func TestCreateGroupRoles(t *testing.T) {
	store := &groupStore{groups: testGroups}
	svc := NewGroupService(store, &roleStore{}, az)

	if _, err := svc.Create(as(1), domain.Group{OrganisationID: 1, Name: "ops"}, nil); err != nil {
		t.Fatal(err)
	}
	if len(store.createdWith) != 0 {
		t.Fatalf("default roles = %v, want none", store.createdWith)
	}

	if _, err := svc.Create(as(1), domain.Group{OrganisationID: 1, Name: "auditors"}, []int64{13, 12, 13}); err != nil {
		t.Fatal(err)
	}
	if len(store.createdWith) != 2 || store.createdWith[0] != 13 || store.createdWith[1] != 12 {
		t.Fatalf("stored roles = %v, want duplicates removed in first-seen order", store.createdWith)
	}
}

// Roles are what a user may do: the owner role is a direct role of user 1, the
// group of user 2 gives the member role, and a user holds the union.
func TestUserPermissionsAreTheUnionOfDirectAndGroupRoles(t *testing.T) {
	store := &userDirStore{}
	svc := newUserService(store)

	if _, err := svc.GetPermissions(as(2), 2); err != nil {
		t.Fatalf("a user reads their own permissions: %v", err)
	}
	if _, err := svc.GetPermissions(as(2), 3); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("a member reads a colleague's permissions: %v", err)
	}
	if _, err := svc.GetPermissions(as(1), 3); err != nil {
		t.Fatalf("an owner reads a colleague's permissions: %v", err)
	}
	if _, err := svc.GetPermissions(as(1), 10); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("another organisation's user: %v", err)
	}
}
