package handler

import (
	"net"
	"net/http"

	"crud/internal/auth"
	"crud/internal/domain"
	"crud/internal/service"
	"crud/internal/util"
)

type AuthHandler struct {
	service       *service.AuthService
	users         *service.UserService
	organisations *service.OrganisationService
	cookie        auth.CookieConfig
}

func NewAuthHandler(svc *service.AuthService, users *service.UserService, organisations *service.OrganisationService, cookie auth.CookieConfig) *AuthHandler {
	return &AuthHandler{service: svc, users: users, organisations: organisations, cookie: cookie}
}

// Register adds the auth routes to mux. The two create routes are public:
// they are how an organisation and its first users come to exist.
func (h *AuthHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/organisations", h.CreateOrganisation)
	mux.HandleFunc("POST /api/v1/auth/users", h.CreateUser)
	mux.HandleFunc("POST /api/v1/auth/login", h.Login)
	mux.HandleFunc("POST /api/v1/auth/logout", h.Logout)
}

type createUserRequest struct {
	Name  string `json:"name" example:"Alice"`
	Email string `json:"email" example:"alice@company.com"`
}

type createOrganisationRequest struct {
	Name       string `json:"name" example:"Acme Corp"`
	AdminName  string `json:"admin_name" example:"Alice"`
	AdminEmail string `json:"admin_email" example:"alice@company.com"`
}

type organisationResponse struct {
	ID    int64       `json:"id" example:"1"`
	Name  string      `json:"name" example:"Acme Corp"`
	Admin domain.User `json:"admin"`
}

// CreateOrganisation godoc
// @Summary     Create an organisation
// @Description Needs no session. Creates the organisation (name unique, ignoring case) with an "admin" and a "member" group, and its first admin user from admin_name and admin_email, in one step: the email must not be registered yet (409), and nothing is created if any part fails. Personal email providers (gmail, yahoo, ...) are rejected. The admin signs in with POST /auth/login.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body     createOrganisationRequest true "Organisation and its admin"
// @Success     201  {object} organisationResponse
// @Failure     400  {object} util.ErrorResponse
// @Failure     409  {object} util.ErrorResponse
// @Failure     500  {object} util.ErrorResponse
// @Router      /auth/organisations [post]
func (h *AuthHandler) CreateOrganisation(w http.ResponseWriter, r *http.Request) {
	var req createOrganisationRequest
	if err := decodeJSON(w, r, &req); err != nil {
		util.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}

	org, admin, err := h.organisations.Create(r.Context(),
		domain.Organisation{Name: req.Name},
		domain.User{Name: req.AdminName, Email: req.AdminEmail})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusCreated, organisationResponse{ID: org.ID, Name: org.Name, Admin: admin})
}

// CreateUser godoc
// @Summary     Create a user
// @Description Requires a session and the user:assign_group permission (admins). The user is added to your own organisation in the basic "member" group; there is no way to join an organisation without an admin. Returns the created user. There is no password; the user signs in with POST /auth/login using the email. Personal email providers (gmail, yahoo, ...) are rejected.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body     createUserRequest true "User details"
// @Success     201  {object} domain.User
// @Failure     400  {object} util.ErrorResponse
// @Failure     401  {object} util.ErrorResponse
// @Failure     403  {object} util.ErrorResponse
// @Failure     404  {object} util.ErrorResponse
// @Failure     409  {object} util.ErrorResponse
// @Failure     500  {object} util.ErrorResponse
// @Router      /auth/users [post]
func (h *AuthHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		util.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}

	user, err := h.users.Create(r.Context(),
		domain.User{Name: req.Name, Email: req.Email})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusCreated, user)
}

type loginRequest struct {
	Email string `json:"email" example:"alice@company.com"`
}

// Login godoc
// @Summary     Sign in with an email
// @Description There is no password: knowing the email of a user created with POST /auth/users is enough (development only, no proof of ownership). On success, sets an HttpOnly "session" cookie (SameSite=Strict) that the browser sends on later requests, and returns the user. Accounts can only sign in with the method they were created with (403 for google/github accounts). An unknown email returns 401. After 5 failed attempts per email and IP, 429 for 15 minutes.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body     loginRequest true "Email"
// @Success     200  {object} domain.User
// @Failure     400  {object} util.ErrorResponse
// @Failure     401  {object} util.ErrorResponse
// @Failure     403  {object} util.ErrorResponse
// @Failure     429  {object} util.ErrorResponse
// @Failure     503  {object} util.ErrorResponse
// @Router      /auth/login [post]
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		util.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}

	token, user, err := h.service.Login(r.Context(), req.Email,
		service.LoginMeta{IP: ip, UserAgent: r.UserAgent()})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	auth.SetSessionCookie(w, h.cookie, token)
	util.JSON(w, http.StatusOK, user)
}

// Logout godoc
// @Summary     Sign out
// @Description Ends the current session and clears the cookie. Requires a session.
// @Tags        auth
// @Success     204
// @Failure     401 {object} util.ErrorResponse
// @Failure     503 {object} util.ErrorResponse
// @Router      /auth/logout [post]
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Logout(r.Context(), auth.TokenFromRequest(r)); err != nil {
		writeServiceError(w, err)
		return
	}

	auth.ClearSessionCookie(w, h.cookie)
	w.WriteHeader(http.StatusNoContent)
}
