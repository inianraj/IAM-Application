package service

import (
	"context"
	"errors"
	"testing"

	"crud/internal/domain"
)

// Users 20 and 21 have no organisation (see the directory in
// authorization_test.go).

func TestUserWithoutOrganisationCanReadOnlyThemselves(t *testing.T) {
	svc := NewUserService(userDirStore{}, az)

	tests := []struct {
		name    string
		caller  int64
		target  int64
		wantErr error
	}{
		{"reads themselves", 20, 20, nil},
		{"another user without an organisation", 20, 21, domain.ErrUserNotFound},
		{"a user of an organisation", 20, 1, domain.ErrUserNotFound},
		{"an organisation member reads a user without one", 2, 20, domain.ErrUserNotFound},
		{"an organisation member reads themselves", 2, 2, nil},
		{"an organisation member reads a colleague", 2, 3, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Get(as(tt.caller), tt.target)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestUserWithoutOrganisationHasNoOrganisationRights(t *testing.T) {
	ctx := as(20)

	// No organisation, so no group and no permission, in any organisation.
	for _, orgID := range []int64{0, 1} {
		if err := az.Require(ctx, domain.PermGroupRead, orgID, domain.ErrOrganisationNotFound); !errors.Is(err, domain.ErrOrganisationNotFound) {
			t.Fatalf("group:read in organisation %d: %v", orgID, err)
		}
	}

	// Their own records are theirs; another user's are not.
	if err := az.RequireScoped(ctx, domain.ResourceRecord, domain.ActionRead, 20, 0, domain.ErrTaskNotFound); err != nil {
		t.Fatalf("own records: %v", err)
	}
	if err := az.RequireScoped(ctx, domain.ResourceRecord, domain.ActionRead, 21, 0, domain.ErrTaskNotFound); !errors.Is(err, domain.ErrTaskNotFound) {
		t.Fatalf("another user's records: %v", err)
	}
	if err := az.RequireScoped(ctx, domain.ResourceRecord, domain.ActionRead, 1, 1, domain.ErrTaskNotFound); !errors.Is(err, domain.ErrTaskNotFound) {
		t.Fatalf("a member's records: %v", err)
	}
	if _, err := az.RequireForUser(ctx, domain.PermUserAssignGroup, 20, domain.ErrUserNotFound); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("assign group: %v", err)
	}
}

type adminOrgStore struct {
	orgStore
	gotAdmin domain.User
	calls    int
	err      error
}

func (a *adminOrgStore) Create(_ context.Context, o domain.Organisation, admin domain.User) (domain.Organisation, domain.User, error) {
	a.calls++
	if a.err != nil {
		return domain.Organisation{}, domain.User{}, a.err
	}
	a.gotAdmin = admin
	o.ID = 7
	admin.ID = 70
	admin.OrganisationID = 7
	return o, admin, nil
}

func TestCreateOrganisationCreatesItsAdmin(t *testing.T) {
	ctx := context.Background()
	acme := domain.Organisation{Name: "Acme"}
	alice := domain.User{Name: " Alice ", Email: " Alice@Acme.com "}

	t.Run("passes a normalized email-method admin to the store", func(t *testing.T) {
		store := &adminOrgStore{}
		org, admin, err := NewOrganisationService(store, az).Create(ctx, acme, alice)
		if err != nil || org.ID != 7 || admin.ID != 70 {
			t.Fatalf("org = %+v, admin = %+v, err = %v", org, admin, err)
		}
		got := store.gotAdmin
		if got.Name != "Alice" || got.Email != "alice@acme.com" || got.SignupMethod != domain.SignupMethodEmail {
			t.Fatalf("admin sent to the store = %+v", got)
		}
	})

	t.Run("invalid input never reaches the store", func(t *testing.T) {
		tests := []struct {
			name  string
			org   domain.Organisation
			admin domain.User
			want  error
		}{
			{"blank organisation name", domain.Organisation{Name: " "}, alice, domain.ErrInvalidOrganisation},
			{"missing admin name", acme, domain.User{Email: "a@acme.com"}, domain.ErrInvalidUser},
			{"missing admin email", acme, domain.User{Name: "Alice"}, domain.ErrInvalidUser},
			{"malformed admin email", acme, domain.User{Name: "Alice", Email: "alice@acme"}, domain.ErrInvalidUser},
			{"personal admin email", acme, domain.User{Name: "Alice", Email: "alice@gmail.com"}, domain.ErrInvalidUser},
		}
		for _, tt := range tests {
			store := &adminOrgStore{}
			_, _, err := NewOrganisationService(store, az).Create(ctx, tt.org, tt.admin)
			if !errors.Is(err, tt.want) {
				t.Fatalf("%s: got %v, want %v", tt.name, err, tt.want)
			}
			if store.calls != 0 {
				t.Fatalf("%s: store must not be called", tt.name)
			}
		}
	})

	t.Run("store errors propagate", func(t *testing.T) {
		for _, want := range []error{domain.ErrEmailTaken, domain.ErrOrganisationExists} {
			store := &adminOrgStore{err: want}
			_, _, err := NewOrganisationService(store, az).Create(ctx, acme, alice)
			if !errors.Is(err, want) {
				t.Fatalf("got %v, want %v", err, want)
			}
		}
	})
}
