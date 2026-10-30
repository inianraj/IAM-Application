package handler

import (
	"net/http"

	"crud/internal/domain"
	"crud/internal/service"
	"crud/internal/util"
)

type RoleHandler struct {
	service *service.RoleService
}

func NewRoleHandler(svc *service.RoleService) *RoleHandler {
	return &RoleHandler{service: svc}
}

// Register adds the role routes to mux.
func (h *RoleHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/roles", h.Create)
	mux.HandleFunc("GET /api/v1/roles", h.GetAll)
	mux.HandleFunc("GET /api/v1/roles/{id}", h.GetByID)
	mux.HandleFunc("PUT /api/v1/roles/{id}", h.Update)
	mux.HandleFunc("DELETE /api/v1/roles/{id}", h.Delete)
	mux.HandleFunc("GET /api/v1/roles/{id}/permissions", h.GetPermissions)
	mux.HandleFunc("PUT /api/v1/roles/{id}/permissions", h.SetPermissions)
	mux.HandleFunc("GET /api/v1/permissions", h.ListPermissions)
}

type createRoleRequest struct {
	Name string `json:"name" example:"developer"`
	// Permissions are keys from GET /permissions. Leave the field out for a
	// role with none.
	Permissions []string `json:"permissions,omitempty" example:"record:read:org"`
}

type updateRoleRequest struct {
	Name string `json:"name" example:"developer"`
}

type rolePermissionsRequest struct {
	Permissions []string `json:"permissions" example:"record:read:org,user:read"`
}

type rolePermissionsResponse struct {
	Permissions []string `json:"permissions" example:"record:read:org,user:read"`
}

const invalidRoleID = "invalid role ID"

// Create godoc
// @Summary     Create a role
// @Description Needs role:create. Creates a role in your own organisation. permissions (keys from GET /permissions) are what the role allows; you can only give permissions you hold yourself (403 otherwise, 400 for an unknown key). Names are lower-cased, must start with a letter, and are unique within the organisation (409).
// @Tags        roles
// @Accept      json
// @Produce     json
// @Param       body body     createRoleRequest true "Role to create"
// @Success     201  {object} domain.Role
// @Failure     400  {object} util.ErrorResponse
// @Failure     403  {object} util.ErrorResponse
// @Failure     404  {object} util.ErrorResponse
// @Failure     409  {object} util.ErrorResponse
// @Failure     500  {object} util.ErrorResponse
// @Router      /roles [post]
func (h *RoleHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRoleRequest
	if err := decodeJSON(w, r, &req); err != nil {
		util.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}

	role, err := h.service.Create(r.Context(), domain.Role{Name: req.Name}, req.Permissions)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusCreated, role)
}

// GetAll godoc
// @Summary     List the roles of your organisation
// @Description Needs role:read. Includes the four built-in roles (owner, admin, member, guest).
// @Tags        roles
// @Produce     json
// @Success     200 {array}  domain.Role
// @Failure     403 {object} util.ErrorResponse
// @Failure     404 {object} util.ErrorResponse
// @Failure     500 {object} util.ErrorResponse
// @Router      /roles [get]
func (h *RoleHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	roles, err := h.service.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, roles)
}

// GetByID godoc
// @Summary     Get a role by ID
// @Description Needs role:read.
// @Tags        roles
// @Produce     json
// @Param       id  path     int true "Role ID"
// @Success     200 {object} domain.Role
// @Failure     400 {object} util.ErrorResponse
// @Failure     403 {object} util.ErrorResponse
// @Failure     404 {object} util.ErrorResponse
// @Failure     500 {object} util.ErrorResponse
// @Router      /roles/{id} [get]
func (h *RoleHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidRoleID)
	if !ok {
		return
	}

	role, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, role)
}

// Update godoc
// @Summary     Rename a role
// @Description Needs role:update. The built-in roles cannot be renamed (400).
// @Tags        roles
// @Accept      json
// @Produce     json
// @Param       id   path     int               true "Role ID"
// @Param       body body     updateRoleRequest true "New name"
// @Success     200  {object} domain.Role
// @Failure     400  {object} util.ErrorResponse
// @Failure     403  {object} util.ErrorResponse
// @Failure     404  {object} util.ErrorResponse
// @Failure     409  {object} util.ErrorResponse
// @Failure     500  {object} util.ErrorResponse
// @Router      /roles/{id} [put]
func (h *RoleHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidRoleID)
	if !ok {
		return
	}

	var req updateRoleRequest
	if err := decodeJSON(w, r, &req); err != nil {
		util.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}

	role, err := h.service.Update(r.Context(), domain.Role{ID: id, Name: req.Name})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, role)
}

// Delete godoc
// @Summary     Delete a role
// @Description Needs role:delete. Fails with 409 while a user or group still has the role, and the built-in roles cannot be deleted (400).
// @Tags        roles
// @Param       id  path int true "Role ID"
// @Success     204
// @Failure     400 {object} util.ErrorResponse
// @Failure     403 {object} util.ErrorResponse
// @Failure     404 {object} util.ErrorResponse
// @Failure     409 {object} util.ErrorResponse
// @Failure     500 {object} util.ErrorResponse
// @Router      /roles/{id} [delete]
func (h *RoleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidRoleID)
	if !ok {
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetPermissions godoc
// @Summary     List what a role allows
// @Description Needs role:read. Returns the permission keys of the role.
// @Tags        roles
// @Produce     json
// @Param       id  path     int true "Role ID"
// @Success     200 {object} rolePermissionsResponse
// @Failure     400 {object} util.ErrorResponse
// @Failure     403 {object} util.ErrorResponse
// @Failure     404 {object} util.ErrorResponse
// @Failure     500 {object} util.ErrorResponse
// @Router      /roles/{id}/permissions [get]
func (h *RoleHandler) GetPermissions(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidRoleID)
	if !ok {
		return
	}

	permissions, err := h.service.GetPermissions(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, rolePermissionsResponse{Permissions: permissions})
}

// SetPermissions godoc
// @Summary     Change what a role allows
// @Description Needs role:update. Replaces the role's permissions with the given keys from GET /permissions (400 for an unknown key). You can only add or remove permissions you hold yourself (403 otherwise), and the built-in roles cannot be changed (400). Takes effect for everyone who has the role on their next request.
// @Tags        roles
// @Accept      json
// @Produce     json
// @Param       id   path     int                     true "Role ID"
// @Param       body body     rolePermissionsRequest true "New permissions"
// @Success     200  {object} rolePermissionsResponse
// @Failure     400  {object} util.ErrorResponse
// @Failure     403  {object} util.ErrorResponse
// @Failure     404  {object} util.ErrorResponse
// @Failure     500  {object} util.ErrorResponse
// @Router      /roles/{id}/permissions [put]
func (h *RoleHandler) SetPermissions(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidRoleID)
	if !ok {
		return
	}

	var req rolePermissionsRequest
	if err := decodeJSON(w, r, &req); err != nil {
		util.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}
	if req.Permissions == nil {
		util.Error(w, http.StatusBadRequest, "permissions is required (use [] for none)")
		return
	}

	permissions, err := h.service.SetPermissions(r.Context(), id, req.Permissions)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, rolePermissionsResponse{Permissions: permissions})
}

// ListPermissions godoc
// @Summary     List every permission
// @Description Needs role:read. Returns the permissions a role can be given, with a description and the resource they belong to.
// @Tags        roles
// @Produce     json
// @Success     200 {array}  domain.Permission
// @Failure     403 {object} util.ErrorResponse
// @Failure     404 {object} util.ErrorResponse
// @Failure     500 {object} util.ErrorResponse
// @Router      /permissions [get]
func (h *RoleHandler) ListPermissions(w http.ResponseWriter, r *http.Request) {
	permissions, err := h.service.ListPermissions(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, permissions)
}
