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
	Name     string `json:"name" example:"Alice"`
	Email    string `json:"email" example:"alice@company.com"`
	Password string `json:"password" example:"s3cret-pass"`
	// RoleIDs are IDs from GET /roles. Leave the field out to give the user the
	// member role.
	RoleIDs []int64 `json:"role_ids,omitempty" example:"4"`
}

type createOrganisationRequest struct {
	Name       string `json:"name" example:"Acme Corp"`
	AdminName  string `json:"admin_name" example:"Alice"`
	AdminEmail string `json:"admin_email" example:"alice@company.com"`
	// AdminPassword is the owner's password (8 to 72 bytes).
	AdminPassword string `json:"admin_password" example:"s3cret-pass"`
}

type organisationResponse struct {
	ID    int64       `json:"id" example:"1"`
	Name  string      `json:"name" example:"Acme Corp"`
	Admin domain.User `json:"admin"`
}

// CreateOrganisation godoc
// @Summary     Create an organisation
// @Description Needs no session. Creates the organisation (name unique, ignoring case) with its four built-in roles (owner, admin, member, guest) and its first user from admin_name, admin_email and admin_password (8 to 72 bytes), who gets the owner role, in one step: the email must not be registered yet (409), and nothing is created if any part fails. Personal email providers (gmail, yahoo, ...) are rejected. The owner signs in with POST /auth/login using the email and password.
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
		domain.User{Name: req.AdminName, Email: req.AdminEmail, Password: req.AdminPassword})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusCreated, organisationResponse{ID: org.ID, Name: org.Name, Admin: admin})
}

// CreateUser godoc
// @Summary     Create a user
// @Description Requires a session and the user:assign_group permission (admins). The user is added to your own organisation with the member role (or the role_ids you give, which needs role:assign and permissions you hold yourself) and no group; there is no way to join an organisation without an admin. Returns the created user. A password is required (8 to 72 bytes); the user signs in with POST /auth/login using the email and password. Personal email providers (gmail, yahoo, ...) are rejected.
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
		domain.User{Name: req.Name, Email: req.Email, Password: req.Password}, req.RoleIDs)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusCreated, user)
}

type loginRequest struct {
	Email    string `json:"email" example:"alice@company.com"`
	Password string `json:"password" example:"s3cret-pass"`
}

// Login godoc
// @Summary     Sign in with an email and password
// @Description Checks the email and password; a wrong password and an unknown email both return 401 "invalid email or password". On success, sets an HttpOnly "session" cookie (SameSite=Strict) that the browser sends on later requests, and returns the user. Accounts can only sign in with the method they were created with (403 for google/github accounts). After 5 failed attempts per email and IP, 429 for 15 minutes.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body     loginRequest true "Email and password"
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

	token, user, err := h.service.Login(r.Context(), req.Email, req.Password,
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
