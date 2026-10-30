package router

import (
	"net/http"

	httpSwagger "github.com/swaggo/http-swagger/v2"

	"crud/internal/handler"
)

// Handlers holds every handler that serves routes.
type Handlers struct {
	Auth          *handler.AuthHandler
	Records       *handler.RecordHandler
	Users         *handler.UserHandler
	Groups        *handler.GroupHandler
	Roles         *handler.RoleHandler
	Organisations *handler.OrganisationHandler

	// Middleware wraps the whole API (origin checks, authentication). It is
	// applied outermost-last, so the first entry runs first.
	Middleware []func(http.Handler) http.Handler
}

// PublicRoutes are the routes that need no session; the authentication
// middleware protects everything else, including routes added later. Entries
// ending in "/" match every path under them. Creating an organisation (with its
// first admin) and logging in are public; users are added to an organisation
// by its admins (POST /auth/users needs a session). Listing organisations is
// public for now.
var PublicRoutes = []string{
	"POST /api/v1/auth/login",
	"POST /api/v1/auth/organisations",
	"GET /api/v1/organisations",
	"GET /swagger/",
}

// New builds the HTTP handler for the whole API. Each handler registers its
// own routes; shared middleware comes from Handlers.Middleware.
func New(h Handlers) http.Handler {
	mux := http.NewServeMux()

	h.Auth.Register(mux)
	h.Records.Register(mux)
	h.Users.Register(mux)
	h.Groups.Register(mux)
	h.Roles.Register(mux)
	h.Organisations.Register(mux)

	mux.Handle("GET /swagger/", httpSwagger.WrapHandler)

	var root http.Handler = mux
	for i := len(h.Middleware) - 1; i >= 0; i-- {
		root = h.Middleware[i](root)
	}
	return root
}
