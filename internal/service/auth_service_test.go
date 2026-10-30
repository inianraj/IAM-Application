package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"crud/internal/domain"
	"crud/internal/session"
)

type fakeUsers struct {
	users map[string]domain.User
	err   error
}

func (f *fakeUsers) GetByEmail(_ context.Context, email string) (domain.User, error) {
	if f.err != nil {
		return domain.User{}, f.err
	}
	u, ok := f.users[email]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}

type fakeSessions struct {
	created []session.Data
	deleted []string
	err     error
}

func (f *fakeSessions) Create(_ context.Context, d session.Data) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.created = append(f.created, d)
	return "token-123", nil
}
func (f *fakeSessions) Get(context.Context, string) (session.Data, error) {
	return session.Data{}, session.ErrNotFound
}
func (f *fakeSessions) Delete(_ context.Context, token string) error {
	f.deleted = append(f.deleted, token)
	return f.err
}

type fakeLimiter struct {
	failures map[string]int
	max      int
	err      error
	resets   int
}

func (f *fakeLimiter) Blocked(_ context.Context, email, _ string) (bool, error) {
	return f.failures[email] >= f.max, f.err
}
func (f *fakeLimiter) Fail(_ context.Context, email, _ string) error {
	f.failures[email]++
	return nil
}
func (f *fakeLimiter) Reset(_ context.Context, email, _ string) error {
	delete(f.failures, email)
	f.resets++
	return nil
}

const annPassword = "correct-horse"

func newAuth() (*AuthService, *fakeSessions, *fakeLimiter) {
	annHash, err := domain.HashPassword(annPassword)
	if err != nil {
		panic(err)
	}
	users := &fakeUsers{users: map[string]domain.User{
		"ann@corp.com":  {ID: 1, Email: "ann@corp.com", OrganisationID: 4, SignupMethod: domain.SignupMethodEmail, PasswordHash: annHash},
		"gina@corp.com": {ID: 2, Email: "gina@corp.com", OrganisationID: 4, SignupMethod: domain.SignupMethodGoogle},
		"gary@corp.com": {ID: 3, Email: "gary@corp.com", OrganisationID: 4, SignupMethod: domain.SignupMethodGitHub},
	}}
	sessions := &fakeSessions{}
	limiter := &fakeLimiter{failures: map[string]int{}, max: 5}

	return NewAuthService(users, sessions, limiter), sessions, limiter
}

var meta = LoginMeta{IP: "10.0.0.1", UserAgent: "test"}

func TestLoginSuccess(t *testing.T) {
	svc, sessions, limiter := newAuth()
	limiter.failures["ann@corp.com"] = 2

	token, user, err := svc.Login(context.Background(), "  Ann@Corp.com ", annPassword, meta)
	if err != nil {
		t.Fatal(err)
	}
	if token != "token-123" || user.ID != 1 {
		t.Fatalf("token = %q, user = %+v", token, user)
	}
	if len(sessions.created) != 1 || sessions.created[0].UserID != 1 ||
		sessions.created[0].OrganisationID != 4 || sessions.created[0].IP != "10.0.0.1" {
		t.Fatalf("session data = %+v", sessions.created)
	}
	if limiter.resets != 1 || limiter.failures["ann@corp.com"] != 0 {
		t.Fatal("a successful login must clear the failure counter")
	}
}

func TestLoginRejections(t *testing.T) {
	tests := []struct {
		name         string
		email        string
		password     string
		wantErr      error
		wantContains string
		wantFailures int
	}{
		{"unknown email", "nobody@corp.com", annPassword, domain.ErrInvalidCredentials, "", 1},
		{"wrong password", "ann@corp.com", "wrong-password", domain.ErrInvalidCredentials, "", 1},
		{"google account", "gina@corp.com", annPassword, domain.ErrWrongSignupMethod, "created with google, sign in with google", 0},
		{"github account", "gary@corp.com", annPassword, domain.ErrWrongSignupMethod, "sign in with github", 0},
		{"missing email", "", annPassword, domain.ErrInvalidUser, "", 0},
		{"missing password", "ann@corp.com", "", domain.ErrInvalidUser, "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, sessions, limiter := newAuth()

			token, _, err := svc.Login(context.Background(), tt.email, tt.password, meta)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantContains != "" && !strings.Contains(err.Error(), tt.wantContains) {
				t.Fatalf("message %q should contain %q", err.Error(), tt.wantContains)
			}
			if token != "" || len(sessions.created) != 0 {
				t.Fatal("no session may be created on failure")
			}
			if got := limiter.failures[strings.ToLower(tt.email)]; got != tt.wantFailures {
				t.Fatalf("recorded failures = %d, want %d", got, tt.wantFailures)
			}
		})
	}
}

func TestLoginIsBlockedAfterTooManyUnknownEmails(t *testing.T) {
	svc, sessions, _ := newAuth()

	for i := 0; i < 5; i++ {
		if _, _, err := svc.Login(context.Background(), "nobody@corp.com", annPassword, meta); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("attempt %d: %v", i+1, err)
		}
	}

	_, _, err := svc.Login(context.Background(), "nobody@corp.com", annPassword, meta)
	if !errors.Is(err, domain.ErrTooManyAttempts) {
		t.Fatalf("6th attempt: got %v, want ErrTooManyAttempts", err)
	}
	if len(sessions.created) != 0 {
		t.Fatal("a blocked login must not create a session")
	}
}

func TestLoginFailsClosedWhenRedisIsDown(t *testing.T) {
	svc, sessions, limiter := newAuth()

	limiter.err = errors.New("redis down")
	if _, _, err := svc.Login(context.Background(), "ann@corp.com", annPassword, meta); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("limiter failure: got %v, want ErrUnavailable", err)
	}

	limiter.err = nil
	sessions.err = errors.New("redis down")
	if _, _, err := svc.Login(context.Background(), "ann@corp.com", annPassword, meta); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("session failure: got %v, want ErrUnavailable", err)
	}
}

func TestLogout(t *testing.T) {
	svc, sessions, _ := newAuth()

	if err := svc.Logout(context.Background(), "token-123"); err != nil {
		t.Fatal(err)
	}
	if len(sessions.deleted) != 1 || sessions.deleted[0] != "token-123" {
		t.Fatalf("deleted = %v", sessions.deleted)
	}

	sessions.err = errors.New("redis down")
	if err := svc.Logout(context.Background(), "x"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("got %v, want ErrUnavailable", err)
	}
}
