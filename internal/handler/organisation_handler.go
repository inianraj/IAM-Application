package handler

import (
	"net/http"

	"crud/internal/service"
	"crud/internal/util"
)

type OrganisationHandler struct {
	service *service.OrganisationService
}

func NewOrganisationHandler(svc *service.OrganisationService) *OrganisationHandler {
	return &OrganisationHandler{service: svc}
}

// Register adds the organisation routes to mux. Get, update and delete act on
// the caller's own organisation, so they take no id; creating an organisation
// is part of the auth routes.
func (h *OrganisationHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/organisations", h.GetAll)
	mux.HandleFunc("GET /api/v1/organisation", h.GetCurrent)
	mux.HandleFunc("PUT /api/v1/organisation", h.UpdateCurrent)
	mux.HandleFunc("DELETE /api/v1/organisation", h.DeleteCurrent)
}

type organisationRequest struct {
	Name string `json:"name" example:"Acme Corp"`
}

type deleteOrganisationRequest struct {
	ConfirmName string `json:"confirm_name" example:"Acme Corp"`
}

// GetAll godoc
// @Summary List all organisations
// @Tags    organisations
// @Produce json
// @Success 200 {array}  domain.Organisation
// @Failure 500 {object} util.ErrorResponse
// @Router  /organisations [get]
func (h *OrganisationHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	orgs, err := h.service.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, orgs)
}

// GetCurrent godoc
// @Summary     Get my organisation
// @Description The organisation of the signed-in user; no id is needed. 404 for a user without an organisation. Requires a session.
// @Tags        organisations
// @Produce     json
// @Success     200 {object} domain.Organisation
// @Failure     401 {object} util.ErrorResponse
// @Failure     404 {object} util.ErrorResponse
// @Failure     500 {object} util.ErrorResponse
// @Router      /organisation [get]
func (h *OrganisationHandler) GetCurrent(w http.ResponseWriter, r *http.Request) {
	org, err := h.service.GetCurrent(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, org)
}

// UpdateCurrent godoc
// @Summary     Rename my organisation
// @Description Admins only (403 for members). The new name must be unique, ignoring case. Requires a session.
// @Tags        organisations
// @Accept      json
// @Produce     json
// @Param       body body     organisationRequest true "New organisation details"
// @Success     200  {object} domain.Organisation
// @Failure     400  {object} util.ErrorResponse
// @Failure     401  {object} util.ErrorResponse
// @Failure     403  {object} util.ErrorResponse
// @Failure     404  {object} util.ErrorResponse
// @Failure     409  {object} util.ErrorResponse
// @Failure     500  {object} util.ErrorResponse
// @Router      /organisation [put]
func (h *OrganisationHandler) UpdateCurrent(w http.ResponseWriter, r *http.Request) {
	var req organisationRequest
	if err := decodeJSON(w, r, &req); err != nil {
		util.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}

	org, err := h.service.UpdateCurrent(r.Context(), req.Name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, org)
}

// DeleteCurrent godoc
// @Summary     Delete my organisation
// @Description Admins only (403 for members). PERMANENTLY deletes the organisation, its groups, all its users (the caller included) and all their records, in one transaction. confirm_name must be the organisation's exact name (400 otherwise). The users' sessions stop working immediately. Requires a session.
// @Tags        organisations
// @Accept      json
// @Param       body body deleteOrganisationRequest true "Confirmation"
// @Success     204
// @Failure     400 {object} util.ErrorResponse
// @Failure     401 {object} util.ErrorResponse
// @Failure     403 {object} util.ErrorResponse
// @Failure     404 {object} util.ErrorResponse
// @Failure     409 {object} util.ErrorResponse
// @Failure     500 {object} util.ErrorResponse
// @Router      /organisation [delete]
func (h *OrganisationHandler) DeleteCurrent(w http.ResponseWriter, r *http.Request) {
	var req deleteOrganisationRequest
	if err := decodeJSON(w, r, &req); err != nil {
		util.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}

	if _, err := h.service.DeleteCurrent(r.Context(), req.ConfirmName); err != nil {
		writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
