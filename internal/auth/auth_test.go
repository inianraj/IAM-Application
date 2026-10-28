package auth

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"crud/internal/domain"
	"crud/internal/session"
)

type fakeSessions struct {
	data map[string]session.Data
	err  error
}

func (f *fakeSessions) Create(context.Context, session.Data) (string, error) { return "", nil }
func (f *fakeSessions) Delete(context.Context, string) error                 { return nil }
func (f *fakeSessions) Get(_ context.Context, token string) (session.Data, error) {
	if f.err != nil {
		return session.Data{}, f.err
	}
	d, ok := f.data[token]
	if !ok {
		return session.Data{}, session.ErrNotFound
	}
	return d, nil
}

type fakeUsers struct {
	users map[int64]domain.User
	err   error
}

func (f *fakeUsers) GetByID(_ context.Context, id int64) (domain.User, error) {
	if f.err != nil {
		return domain.User{}, f.err
	}
	u, ok := f.users[id]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}

var public = []string{"POST /api/v1/auth/login", "GET /swagger/"}

func setup() (http.Handler, *fakeSessions, *fakeUsers, *Principal) {
	sessions := &fakeSessions{data: map[string]session.Data{"good": {UserID: 1}, "ghost": {UserID: 99}}}
	users := &fakeUsers{users: map[int64]domain.User{1: {ID: 1, OrganisationID: 4, GroupID: 9}}}

	var seen Principal
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p, ok := FromContext(r.Context()); ok {
			seen = p
		}
		w.WriteHeader(http.StatusOK)
	})

	mw := NewMiddleware(sessions, users, CookieConfig{}, public, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return mw.Wrap(next), sessions, users, &seen
}

func do(h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestMiddlewareAcceptsValidSessionAndSetsPrincipal(t *testing.T) {
	h, _, _, seen := setup()

	rec := do(h, "GET", "/api/v1/users/1", "good")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if seen.User.ID != 1 || seen.User.OrganisationID != 4 {
		t.Fatalf("principal = %+v", seen)
	}
}

func TestMiddlewareRejectsMissingUnknownAndDeletedUsers(t *testing.T) {
	h, _, _, _ := setup()

	tests := []struct{ name, token string }{
		{"no cookie", ""},
		{"unknown token", "bogus"},
		{"user deleted after login", "ghost"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(h, "GET", "/api/v1/users/1", tt.token)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), `"error"`) {
				t.Fatalf("body = %s", rec.Body.String())
			}
		})
	}

	// A dead session also clears the stale cookie.
	rec := do(h, "GET", "/api/v1/users/1", "bogus")
	if c := rec.Result().Cookies(); len(c) != 1 || c[0].MaxAge >= 0 {
		t.Fatalf("expected the cookie to be cleared, got %v", c)
	}
}

func TestMiddlewareFailsClosedWhenBackendsAreDown(t *testing.T) {
	h, sessions, users, _ := setup()

	sessions.err = errors.New("redis down")
	if rec := do(h, "GET", "/api/v1/users/1", "good"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("redis down: status = %d, want 503", rec.Code)
	}

	sessions.err = nil
	users.err = errors.New("db down")
	if rec := do(h, "GET", "/api/v1/users/1", "good"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("user store down: status = %d, want 503", rec.Code)
	}
}

func TestMiddlewarePublicRoutesNeedNoSession(t *testing.T) {
	h, _, _, _ := setup()

	tests := []struct {
		method, path string
		want         int
	}{
		{"POST", "/api/v1/auth/login", 200},
		{"GET", "/swagger/index.html", 200},
		{"GET", "/api/v1/auth/login", 401},   // same path, other method
		{"POST", "/api/v1/auth/login/", 401}, // exact match only
		{"GET", "/api/v1/records", 401},
		{"DELETE", "/api/v1/groups/1", 401},
	}
	for _, tt := range tests {
		if rec := do(h, tt.method, tt.path, ""); rec.Code != tt.want {
			t.Errorf("%s %s = %d, want %d", tt.method, tt.path, rec.Code, tt.want)
		}
	}
}

func TestOriginGuard(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := OriginGuard([]string{"http://app.example.com"})(ok)

	tests := []struct {
		name, method, origin string
		want                 int
		wantCORS             bool
	}{
		{"POST without Origin (curl)", "POST", "", 200, false},
		{"POST from the same host", "POST", "http://example.com", 200, false},
		{"POST from an allowed origin", "POST", "http://app.example.com", 200, true},
		{"POST from a foreign origin", "POST", "http://evil.test", 403, false},
		{"DELETE from a foreign origin", "DELETE", "http://evil.test", 403, false},
		{"GET from a foreign origin", "GET", "http://evil.test", 200, false},
		{"preflight from an allowed origin", "OPTIONS", "http://app.example.com", 204, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "http://example.com/api/v1/records", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
			cors := rec.Header().Get("Access-Control-Allow-Origin")
			if tt.wantCORS && (cors != tt.origin || rec.Header().Get("Access-Control-Allow-Credentials") != "true") {
				t.Fatalf("missing CORS headers: %v", rec.Header())
			}
			if !tt.wantCORS && cors != "" {
				t.Fatalf("unexpected CORS header %q", cors)
			}
		})
	}
}

func TestSessionCookieFlags(t *testing.T) {
	rec := httptest.NewRecorder()
	SetSessionCookie(rec, CookieConfig{Secure: true, TTL: 3600e9}, "tok")

	c := rec.Result().Cookies()[0]
	if c.Name != CookieName || c.Value != "tok" || !c.HttpOnly || !c.Secure ||
		c.SameSite != http.SameSiteStrictMode || c.MaxAge != 3600 || c.Path != "/" {
		t.Fatalf("cookie = %+v", c)
	}
}
