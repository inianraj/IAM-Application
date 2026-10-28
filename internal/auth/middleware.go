package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"crud/internal/domain"
	"crud/internal/session"
	"crud/internal/util"
)

// UserGetter loads the current state of a user (the cached store in main).
type UserGetter interface {
	GetByID(ctx context.Context, id int64) (domain.User, error)
}

// Middleware requires a valid session for every route that is not public.
type Middleware struct {
	sessions session.Store
	users    UserGetter
	cookie   CookieConfig
	public   []string
	logger   *slog.Logger
}

// NewMiddleware builds the authentication middleware. public lists the routes
// that need no session, as "METHOD /exact/path"; an entry ending in "/"
// matches every path under it. Everything else, including routes added later,
// is protected.
func NewMiddleware(sessions session.Store, users UserGetter, cookie CookieConfig, public []string, logger *slog.Logger) *Middleware {
	return &Middleware{sessions: sessions, users: users, cookie: cookie, public: public, logger: logger}
}

func (m *Middleware) isPublic(r *http.Request) bool {
	route := r.Method + " " + r.URL.Path
	for _, p := range m.public {
		if p == route || (strings.HasSuffix(p, "/") && strings.HasPrefix(route, p)) {
			return true
		}
	}
	return false
}

// Wrap authenticates requests to protected routes. It fails closed: when Redis
// or the user store can't be reached the request is refused with 503, never
// let through.
func (m *Middleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.isPublic(r) {
			next.ServeHTTP(w, r)
			return
		}

		token := TokenFromRequest(r)
		if token == "" {
			util.Error(w, http.StatusUnauthorized, domain.ErrUnauthenticated.Error())
			return
		}

		data, err := m.sessions.Get(r.Context(), token)
		if errors.Is(err, session.ErrNotFound) {
			ClearSessionCookie(w, m.cookie)
			util.Error(w, http.StatusUnauthorized, domain.ErrUnauthenticated.Error())
			return
		}
		if err != nil {
			m.logger.Error("session lookup failed", "error", err)
			util.Error(w, http.StatusServiceUnavailable, domain.ErrUnavailable.Error())
			return
		}

		user, err := m.users.GetByID(r.Context(), data.UserID)
		if errors.Is(err, domain.ErrUserNotFound) {
			// The user was deleted after logging in.
			ClearSessionCookie(w, m.cookie)
			util.Error(w, http.StatusUnauthorized, domain.ErrUnauthenticated.Error())
			return
		}
		if err != nil {
			m.logger.Error("loading session user failed", "user_id", data.UserID, "error", err)
			util.Error(w, http.StatusServiceUnavailable, domain.ErrUnavailable.Error())
			return
		}

		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), Principal{User: user})))
	})
}
