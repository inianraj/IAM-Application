// Package auth authenticates requests from the session cookie and exposes the
// caller to the rest of the code through the request context.
package auth

import (
	"context"

	"crud/internal/domain"
)

// Principal is the authenticated caller. User is read fresh on every request,
// so group changes and deletions take effect immediately.
type Principal struct {
	User domain.User
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// FromContext returns the caller, or false for a request that did not go
// through the authentication middleware (public routes).
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}
