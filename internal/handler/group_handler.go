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
	mux.HandleFunc("GET /api/v1/groups/{id}/roles", h.GetRoles)
	mux.HandleFunc("PUT /api/v1/groups/{id}/roles", h.SetRoles)
}

type groupRequest struct {
	OrganisationID int64  `json:"organisation_id" example:"1"`
	Name           string `json:"name" example:"editors"`
}

type createGroupRequest struct {
	OrganisationID int64  `json:"organisation_id" example:"1"`
	Name           string `json:"name" example:"editors"`
	// RoleIDs are IDs from GET /roles. Leave the field out for a group with no
	// roles.
	RoleIDs []int64 `json:"role_ids,omitempty" example:"4"`
}

type setRolesRequest struct {
	RoleIDs []int64 `json:"role_ids" example:"4,5"`
}

const invalidGroupID = "invalid group ID"

// Create godoc
// @Summary     Create a group in an organisation
// @Description Needs group:create. Returns the new group, including its ID. Names are lower-cased, must start with a letter, and are unique within the organisation. role_ids (IDs from GET /roles) give the group its roles and need role:assign; without them the group has none, and its members only get what their own roles allow.
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
		domain.Group{OrganisationID: req.OrganisationID, Name: req.Name}, req.RoleIDs)
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

// GetRoles godoc
// @Summary     List the roles of a group
// @Description Needs group:read. Every member of the group gets what these roles allow, on top of their own roles.
// @Tags        groups
// @Produce     json
// @Param       id  path     int true "Group ID"
// @Success     200 {array}  domain.Role
// @Failure     400 {object} util.ErrorResponse
// @Failure     403 {object} util.ErrorResponse
// @Failure     404 {object} util.ErrorResponse
// @Failure     500 {object} util.ErrorResponse
// @Router      /groups/{id}/roles [get]
func (h *GroupHandler) GetRoles(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidGroupID)
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
// @Summary     Change the roles of a group
// @Description Needs role:assign. Replaces the group's roles with the given role IDs (404 for a role that does not exist in the organisation). You can only add or remove roles whose permissions you hold yourself (403 otherwise). Takes effect for the members on their next request.
// @Tags        groups
// @Accept      json
// @Produce     json
// @Param       id   path     int               true "Group ID"
// @Param       body body     setRolesRequest   true "New roles"
// @Success     200  {array}  domain.Role
// @Failure     400  {object} util.ErrorResponse
// @Failure     403  {object} util.ErrorResponse
// @Failure     404  {object} util.ErrorResponse
// @Failure     500  {object} util.ErrorResponse
// @Router      /groups/{id}/roles [put]
func (h *GroupHandler) SetRoles(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidGroupID)
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
