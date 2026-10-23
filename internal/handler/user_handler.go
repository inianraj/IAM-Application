package handler

import (
	"net/http"

	"crud/internal/domain"
	"crud/internal/service"
	"crud/internal/util"
)

type UserHandler struct {
	service *service.UserService
}

func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{service: svc}
}

// Register adds the user routes to mux.
func (h *UserHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/signup", h.Signup)

	mux.HandleFunc("GET /api/v1/users/{id}", h.GetByID)
	mux.HandleFunc("PUT /api/v1/users/{id}", h.Update)
	mux.HandleFunc("DELETE /api/v1/users/{id}", h.Delete)
	mux.HandleFunc("PUT /api/v1/users/{id}/group", h.AssignGroup)
}

type signupRequest struct {
	Name           string `json:"name" example:"Alice"`
	Email          string `json:"email" example:"alice@company.com"`
	OrganisationID int64  `json:"organisation_id" example:"1"`
}

type signupResponse struct {
	Message string `json:"message" example:"signup successful"`
	UserID  int64  `json:"user_id" example:"1"`
}

type updateUserRequest struct {
	Name  string `json:"name" example:"Alice"`
	Email string `json:"email" example:"alice@company.com"`
}

type assignGroupRequest struct {
	GroupID int64 `json:"group_id" example:"2"`
}

type assignGroupResponse struct {
	UserID  int64 `json:"user_id" example:"1"`
	GroupID int64 `json:"group_id" example:"2"`
}

const invalidUserID = "invalid user ID"

// Signup godoc
// @Summary     Sign up with a work email
// @Description Creates the user in the "admin" group of the given organisation (404 if the organisation doesn't exist). Personal email providers (gmail, yahoo, ...) are rejected.
// @Tags        users
// @Accept      json
// @Produce     json
// @Param       body body     signupRequest true "Signup details"
// @Success     201  {object} signupResponse
// @Failure     400  {object} util.ErrorResponse
// @Failure     404  {object} util.ErrorResponse
// @Failure     409  {object} util.ErrorResponse
// @Failure     500  {object} util.ErrorResponse
// @Router      /auth/signup [post]
func (h *UserHandler) Signup(w http.ResponseWriter, r *http.Request) {
	var req signupRequest
	if err := decodeJSON(w, r, &req); err != nil {
		util.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}

	user, err := h.service.Signup(r.Context(),
		domain.User{Name: req.Name, Email: req.Email, OrganisationID: req.OrganisationID})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusCreated, signupResponse{Message: "signup successful", UserID: user.ID})
}

// GetByID godoc
// @Summary Get a user by ID
// @Tags    users
// @Produce json
// @Param   id  path     int true "User ID"
// @Success 200 {object} domain.User
// @Failure 400 {object} util.ErrorResponse
// @Failure 404 {object} util.ErrorResponse
// @Failure 500 {object} util.ErrorResponse
// @Router  /users/{id} [get]
func (h *UserHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidUserID)
	if !ok {
		return
	}

	user, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, user)
}

// Update godoc
// @Summary     Update a user
// @Description Changes the name and email (work email only). The group is changed with the assign-group endpoint.
// @Tags        users
// @Accept      json
// @Produce     json
// @Param       id   path     int               true "User ID"
// @Param       body body     updateUserRequest true "New user details"
// @Success     200  {object} domain.User
// @Failure     400  {object} util.ErrorResponse
// @Failure     404  {object} util.ErrorResponse
// @Failure     409  {object} util.ErrorResponse
// @Failure     500  {object} util.ErrorResponse
// @Router      /users/{id} [put]
func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidUserID)
	if !ok {
		return
	}

	var req updateUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		util.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}

	user, err := h.service.Update(r.Context(),
		domain.User{ID: id, Name: req.Name, Email: req.Email})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, user)
}

// Delete godoc
// @Summary     Delete a user
// @Description Fails with 409 while the user still has records; delete those first.
// @Tags        users
// @Param       id  path int true "User ID"
// @Success     204
// @Failure     400 {object} util.ErrorResponse
// @Failure     404 {object} util.ErrorResponse
// @Failure     409 {object} util.ErrorResponse
// @Failure     500 {object} util.ErrorResponse
// @Router  /users/{id} [delete]
func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidUserID)
	if !ok {
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AssignGroup godoc
// @Summary     Assign a group to a user
// @Description The group must exist (404) and belong to the organisation stored on the user (400 otherwise).
// @Tags        users
// @Accept      json
// @Produce     json
// @Param       id   path     int                true "User ID"
// @Param       body body     assignGroupRequest true "Group to assign"
// @Success     200  {object} assignGroupResponse
// @Failure     400  {object} util.ErrorResponse
// @Failure     404  {object} util.ErrorResponse
// @Failure     500  {object} util.ErrorResponse
// @Router      /users/{id}/group [put]
func (h *UserHandler) AssignGroup(w http.ResponseWriter, r *http.Request) {
	userID, ok := parsePathID(w, r, invalidUserID)
	if !ok {
		return
	}

	var req assignGroupRequest
	if err := decodeJSON(w, r, &req); err != nil {
		util.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}

	if err := h.service.AssignGroup(r.Context(), userID, req.GroupID); err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, assignGroupResponse{UserID: userID, GroupID: req.GroupID})
}
