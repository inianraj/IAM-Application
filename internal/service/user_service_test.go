package service

import (
	"context"
	"errors"
	"testing"

	"crud/internal/domain"
)

type fakeUserStore struct {
	created []domain.User
	err     error
}

func (f *fakeUserStore) Create(_ context.Context, u domain.User) (domain.User, error) {
	if f.err != nil {
		return domain.User{}, f.err
	}
	f.created = append(f.created, u)
	u.ID = int64(len(f.created))
	return u, nil
}
func (f *fakeUserStore) GetByID(context.Context, int64) (domain.User, error) {
	return domain.User{}, nil
}
func (f *fakeUserStore) Update(_ context.Context, u domain.User) (domain.User, error) { return u, nil }
func (f *fakeUserStore) Delete(context.Context, int64) error                          { return nil }
func (f *fakeUserStore) AssignGroup(context.Context, int64, int64) error              { return nil }

// Users are added by an admin: the new user joins the caller's own
// organisation as a member, with a normalized email and no password.
func TestCreateAddsAMemberToTheCallersOrganisation(t *testing.T) {
	store := &fakeUserStore{}
	svc := NewUserService(store, az)

	u, err := svc.Create(as(1), domain.User{Name: " Ann ", Email: "Ann@Corp.com"})
	if err != nil {
		t.Fatal(err)
	}
	if u.ID == 0 {
		t.Fatal("expected the created user back")
	}

	got := store.created[0]
	if got.SignupMethod != domain.SignupMethodEmail || got.Email != "ann@corp.com" || got.Name != "Ann" {
		t.Fatalf("stored user = %+v", got)
	}
	if got.OrganisationID != 1 {
		t.Fatalf("organisation = %d, want the caller's (1)", got.OrganisationID)
	}
}

func TestCreateNeedsALoginAndThePermission(t *testing.T) {
	tests := []struct {
		name    string
		caller  int64
		wantErr error
	}{
		{"no session", 0, domain.ErrUnauthenticated},
		{"member", 2, domain.ErrForbidden},
		{"auditor", 4, domain.ErrForbidden},
		{"admin of another organisation still joins their own", 10, nil},
		{"user without an organisation", 20, domain.ErrOrganisationNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeUserStore{}
			ctx := context.Background()
			if tt.caller != 0 {
				ctx = as(tt.caller)
			}

			_, err := NewUserService(store, az).Create(ctx, domain.User{Name: "Ann", Email: "ann@corp.com"})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil && len(store.created) != 0 {
				t.Fatal("a refused request must not write")
			}
			if tt.wantErr == nil && store.created[0].OrganisationID != dir.users[tt.caller].OrganisationID {
				t.Fatalf("organisation = %d, want the caller's", store.created[0].OrganisationID)
			}
		})
	}
}

func TestCreateRejectsBadInputBeforeTouchingTheStore(t *testing.T) {
	tests := []struct {
		name string
		user domain.User
	}{
		{"personal email", domain.User{Name: "Ann", Email: "ann@gmail.com"}},
		{"missing name", domain.User{Email: "ann@corp.com"}},
		{"missing email", domain.User{Name: "Ann"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeUserStore{}
			_, err := NewUserService(store, az).Create(as(1), tt.user)
			if !errors.Is(err, domain.ErrInvalidUser) {
				t.Fatalf("expected ErrInvalidUser, got %v", err)
			}
			if len(store.created) != 0 {
				t.Fatal("store must not be called")
			}
		})
	}
}

func TestCreatePropagatesStoreErrors(t *testing.T) {
	store := &fakeUserStore{err: domain.ErrEmailTaken}
	_, err := NewUserService(store, az).Create(as(1), domain.User{Name: "Ann", Email: "ann@corp.com"})
	if !errors.Is(err, domain.ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}
