package handler

import (
	"net/http"
	"strconv"

	"crud/internal/domain"
	"crud/internal/service"
	"crud/internal/util"
)

type GroupHandler struct {
	service *service.GroupService
}

func NewGroupHandler(svc *service.GroupService) *GroupHandler {
	return &GroupHandler{service: svc}
}

// Register adds the group routes to mux.
func (h *GroupHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/groups", h.Create)
	mux.HandleFunc("GET /api/v1/groups", h.GetAll)
	mux.HandleFunc("GET /api/v1/groups/{id}", h.GetByID)
	mux.HandleFunc("PUT /api/v1/groups/{id}", h.Update)
	mux.HandleFunc("DELETE /api/v1/groups/{id}", h.Delete)
	mux.HandleFunc("GET /api/v1/groups/{id}/users", h.ListMembers)
	mux.HandleFunc("GET /api/v1/groups/{id}/permissions", h.GetPermissions)
	mux.HandleFunc("PUT /api/v1/groups/{id}/permissions", h.SetPermissions)
	mux.HandleFunc("GET /api/v1/permissions", h.ListPermissions)
}

type groupRequest struct {
	OrganisationID int64  `json:"organisation_id" example:"1"`
	Name           string `json:"name" example:"editors"`
}

type createGroupRequest struct {
	OrganisationID int64  `json:"organisation_id" example:"1"`
	Name           string `json:"name" example:"editors"`
	// Permissions are keys from GET /permissions. Leave the field out to give
	// the group the same permissions as the built-in member group.
	Permissions []string `json:"permissions,omitempty" example:"record:read:org"`
}

type groupPermissionsRequest struct {
	Permissions []string `json:"permissions" example:"record:read:org,user:read"`
}

type groupPermissionsResponse struct {
	Permissions []string `json:"permissions" example:"record:read:org,user:read"`
}

const invalidGroupID = "invalid group ID"

// Create godoc
// @Summary     Create a group in an organisation
// @Description Needs group:create. Returns the new group, including its ID. Names are lower-cased, must start with a letter, and are unique within the organisation. permissions (keys from GET /permissions) choose what the group may do and need group:manage_permissions; without it the group gets the member permissions.
// @Tags        groups
// @Accept      json
// @Produce     json
// @Param       body body     createGroupRequest true "Group to create"
// @Success     201  {object} domain.Group
// @Failure     400  {object} util.ErrorResponse
// @Failure     403  {object} util.ErrorResponse
// @Failure     404  {object} util.ErrorResponse
// @Failure     409  {object} util.ErrorResponse
// @Failure     500  {object} util.ErrorResponse
// @Router      /groups [post]
func (h *GroupHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createGroupRequest
	if err := decodeJSON(w, r, &req); err != nil {
		util.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}

	group, err := h.service.Create(r.Context(),
		domain.Group{OrganisationID: req.OrganisationID, Name: req.Name}, req.Permissions)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusCreated, group)
}

// GetAll godoc
// @Summary     List the groups of an organisation
// @Description Returns only the groups in the given organisation (an empty list if it has none).
// @Tags        groups
// @Produce     json
// @Param       organisation_id query    int true "Organisation ID"
// @Success     200             {array}  domain.Group
// @Failure     400             {object} util.ErrorResponse
// @Failure     404             {object} util.ErrorResponse
// @Failure     500             {object} util.ErrorResponse
// @Router      /groups [get]
func (h *GroupHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	organisationID, err := strconv.ParseInt(r.URL.Query().Get("organisation_id"), 10, 64)
	if err != nil || organisationID <= 0 {
		util.Error(w, http.StatusBadRequest, "organisation_id query parameter is required and must be a positive integer")
		return
	}

	groups, err := h.service.ListByOrganisation(r.Context(), organisationID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, groups)
}

// GetByID godoc
// @Summary Get a group by ID
// @Tags    groups
// @Produce json
// @Param   id  path     int true "Group ID"
// @Success 200 {object} domain.Group
// @Failure 400 {object} util.ErrorResponse
// @Failure 404 {object} util.ErrorResponse
// @Failure 500 {object} util.ErrorResponse
// @Router  /groups/{id} [get]
func (h *GroupHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidGroupID)
	if !ok {
		return
	}

	group, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, group)
}

// Update godoc
// @Summary     Update a group
// @Description Sets the group's organisation and name (both required). The name must be unique within the organisation. A group that has users cannot be moved to another organisation (409).
// @Tags        groups
// @Accept      json
// @Produce     json
// @Param       id   path     int          true "Group ID"
// @Param       body body     groupRequest true "New group details"
// @Success     200  {object} domain.Group
// @Failure     400  {object} util.ErrorResponse
// @Failure     404  {object} util.ErrorResponse
// @Failure     409  {object} util.ErrorResponse
// @Failure     500  {object} util.ErrorResponse
// @Router      /groups/{id} [put]
func (h *GroupHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidGroupID)
	if !ok {
		return
	}

	var req groupRequest
	if err := decodeJSON(w, r, &req); err != nil {
		util.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}

	group, err := h.service.Update(r.Context(),
		domain.Group{ID: id, OrganisationID: req.OrganisationID, Name: req.Name})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, group)
}

// Delete godoc
// @Summary     Delete a group
// @Description Fails with 409 while the group still has users; move them with the assign-group endpoint first.
// @Tags        groups
// @Param       id  path int true "Group ID"
// @Success     204
// @Failure     400 {object} util.ErrorResponse
// @Failure     404 {object} util.ErrorResponse
// @Failure     409 {object} util.ErrorResponse
// @Failure     500 {object} util.ErrorResponse
// @Router      /groups/{id} [delete]
func (h *GroupHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidGroupID)
	if !ok {
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListMembers godoc
// @Summary     List the users in a group
// @Description Returns the name and email of every user in the group (an empty list if the group has no users).
// @Tags        groups
// @Produce     json
// @Param       id  path     int true "Group ID"
// @Success     200 {array}  domain.GroupMember
// @Failure     400 {object} util.ErrorResponse
// @Failure     404 {object} util.ErrorResponse
// @Failure     500 {object} util.ErrorResponse
// @Router      /groups/{id}/users [get]
func (h *GroupHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidGroupID)
	if !ok {
		return
	}

	members, err := h.service.ListMembers(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, members)
}

// GetPermissions godoc
// @Summary     List what a group may do
// @Description Needs group:read. Returns the permission keys of the group.
// @Tags        groups
// @Produce     json
// @Param       id  path     int true "Group ID"
// @Success     200 {object} groupPermissionsResponse
// @Failure     400 {object} util.ErrorResponse
// @Failure     403 {object} util.ErrorResponse
// @Failure     404 {object} util.ErrorResponse
// @Failure     500 {object} util.ErrorResponse
// @Router      /groups/{id}/permissions [get]
func (h *GroupHandler) GetPermissions(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidGroupID)
	if !ok {
		return
	}

	permissions, err := h.service.GetPermissions(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, groupPermissionsResponse{Permissions: permissions})
}

// SetPermissions godoc
// @Summary     Change what a group may do
// @Description Needs group:manage_permissions. Replaces the group's permissions with the given keys from GET /permissions (400 for an unknown key). The built-in admin and member groups cannot be changed (400). Takes effect on the group's members' next request.
// @Tags        groups
// @Accept      json
// @Produce     json
// @Param       id   path     int                     true "Group ID"
// @Param       body body     groupPermissionsRequest true "New permissions"
// @Success     200  {object} groupPermissionsResponse
// @Failure     400  {object} util.ErrorResponse
// @Failure     403  {object} util.ErrorResponse
// @Failure     404  {object} util.ErrorResponse
// @Failure     500  {object} util.ErrorResponse
// @Router      /groups/{id}/permissions [put]
func (h *GroupHandler) SetPermissions(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidGroupID)
	if !ok {
		return
	}

	var req groupPermissionsRequest
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

	util.JSON(w, http.StatusOK, groupPermissionsResponse{Permissions: permissions})
}

// ListPermissions godoc
// @Summary     List every permission
// @Description Needs group:manage_permissions, so only admins see it. Returns the permissions a group can be given, with a description and the resource they belong to.
// @Tags        groups
// @Produce     json
// @Success     200 {array}  domain.Permission
// @Failure     403 {object} util.ErrorResponse
// @Failure     404 {object} util.ErrorResponse
// @Failure     500 {object} util.ErrorResponse
// @Router      /permissions [get]
func (h *GroupHandler) ListPermissions(w http.ResponseWriter, r *http.Request) {
	permissions, err := h.service.ListPermissions(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, permissions)
}
