package router

import (
	"net/http"

	httpSwagger "github.com/swaggo/http-swagger/v2"

	"crud/internal/handler"
)

// Handlers holds every handler that serves routes.
type Handlers struct {
	Records       *handler.RecordHandler
	Users         *handler.UserHandler
	Groups        *handler.GroupHandler
	Organisations *handler.OrganisationHandler
}

// New builds the HTTP handler for the whole API. Each handler registers its
// own routes; add shared middleware by wrapping the returned handler.
func New(h Handlers) http.Handler {
	mux := http.NewServeMux()

	h.Records.Register(mux)
	h.Users.Register(mux)
	h.Groups.Register(mux)
	h.Organisations.Register(mux)

	mux.Handle("GET /swagger/", httpSwagger.WrapHandler)

	return mux
}
