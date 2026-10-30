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
	mux.HandleFunc("GET /api/v1/users", h.List)
	mux.HandleFunc("GET /api/v1/users/{id}", h.GetByID)
	mux.HandleFunc("PUT /api/v1/users/{id}", h.Update)
	mux.HandleFunc("DELETE /api/v1/users/{id}", h.Delete)
	mux.HandleFunc("PUT /api/v1/users/{id}/group", h.AssignGroup)
	mux.HandleFunc("GET /api/v1/users/{id}/roles", h.GetRoles)
	mux.HandleFunc("PUT /api/v1/users/{id}/roles", h.SetRoles)
	mux.HandleFunc("GET /api/v1/users/{id}/permissions", h.GetPermissions)
}

type updateUserRequest struct {
	Name string `json:"name" example:"Alice"`
}

type assignGroupRequest struct {
	GroupID int64 `json:"group_id" example:"2"`
}

type assignGroupResponse struct {
	UserID  int64 `json:"user_id" example:"1"`
	GroupID int64 `json:"group_id" example:"2"`
}

const invalidUserID = "invalid user ID"

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
// @Description Changes the name only: the email is the login identity and cannot be changed (an email field is rejected). Needs user:update:own for yourself, user:update:org for others. The group is changed with the assign-group endpoint and the roles with the roles endpoint.
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
		domain.User{ID: id, Name: req.Name})
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
// @Description Needs user:assign_group. The group must exist (404) and belong to the organisation stored on the user (400 otherwise). A group_id of 0 takes the user out of their group. The user gets the roles of the new group and loses those of the old one, so you must hold the permissions of every role that changes (403 otherwise).
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

type userPermissionsResponse struct {
	Permissions []string `json:"permissions" example:"record:read:own,user:read"`
}

// GetRoles godoc
// @Summary     List the roles a user holds directly
// @Description Needs role:read, except for yourself. These are the roles given to the user themselves; the roles their group gives are listed under GET /groups/{id}/roles.
// @Tags        users
// @Produce     json
// @Param       id  path     int true "User ID"
// @Success     200 {array}  domain.Role
// @Failure     400 {object} util.ErrorResponse
// @Failure     403 {object} util.ErrorResponse
// @Failure     404 {object} util.ErrorResponse
// @Failure     500 {object} util.ErrorResponse
// @Router      /users/{id}/roles [get]
func (h *UserHandler) GetRoles(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidUserID)
	if !ok {
		return
	}

	roles, err := h.service.GetRoles(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, roles)
}

// SetRoles godoc
// @Summary     Change the roles of a user
// @Description Needs role:assign. Replaces the roles the user holds directly with the given role IDs (404 for a role that does not exist in the organisation). You can only add or remove roles whose permissions you hold yourself (403 otherwise), so only an owner can give or take the owner role, and an organisation always keeps at least one owner (409). Takes effect on the user's next request.
// @Tags        users
// @Accept      json
// @Produce     json
// @Param       id   path     int             true "User ID"
// @Param       body body     setRolesRequest true "New roles"
// @Success     200  {array}  domain.Role
// @Failure     400  {object} util.ErrorResponse
// @Failure     403  {object} util.ErrorResponse
// @Failure     404  {object} util.ErrorResponse
// @Failure     409  {object} util.ErrorResponse
// @Failure     500  {object} util.ErrorResponse
// @Router      /users/{id}/roles [put]
func (h *UserHandler) SetRoles(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidUserID)
	if !ok {
		return
	}

	var req setRolesRequest
	if err := decodeJSON(w, r, &req); err != nil {
		util.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}
	if req.RoleIDs == nil {
		util.Error(w, http.StatusBadRequest, "role_ids is required (use [] for none)")
		return
	}

	roles, err := h.service.SetRoles(r.Context(), id, req.RoleIDs)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, roles)
}

// GetPermissions godoc
// @Summary     List what a user may do
// @Description Needs role:read, except for yourself. Returns every permission the user has: the union of their own roles and the roles of their group.
// @Tags        users
// @Produce     json
// @Param       id  path     int true "User ID"
// @Success     200 {object} userPermissionsResponse
// @Failure     400 {object} util.ErrorResponse
// @Failure     403 {object} util.ErrorResponse
// @Failure     404 {object} util.ErrorResponse
// @Failure     500 {object} util.ErrorResponse
// @Router      /users/{id}/permissions [get]
func (h *UserHandler) GetPermissions(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidUserID)
	if !ok {
		return
	}

	permissions, err := h.service.GetPermissions(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, userPermissionsResponse{Permissions: permissions})
}

// List godoc
// @Summary     List the users of your organisation
// @Description Needs user:read. Returns every user of your own organisation, oldest first, including those without a group.
// @Tags        users
// @Produce     json
// @Success     200 {array}  domain.User
// @Failure     403 {object} util.ErrorResponse
// @Failure     404 {object} util.ErrorResponse
// @Failure     500 {object} util.ErrorResponse
// @Router      /users [get]
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	users, err := h.service.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, users)
}
