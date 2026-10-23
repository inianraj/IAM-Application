package handler

import (
	"net/http"

	"crud/internal/domain"
	"crud/internal/service"
	"crud/internal/util"
)

type OrganisationHandler struct {
	service *service.OrganisationService
}

func NewOrganisationHandler(svc *service.OrganisationService) *OrganisationHandler {
	return &OrganisationHandler{service: svc}
}

// Register adds the organisation routes to mux.
func (h *OrganisationHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/organisations", h.Create)
	mux.HandleFunc("GET /api/v1/organisations", h.GetAll)
	mux.HandleFunc("GET /api/v1/organisations/{id}", h.GetByID)
	mux.HandleFunc("PUT /api/v1/organisations/{id}", h.Update)
	mux.HandleFunc("DELETE /api/v1/organisations/{id}", h.Delete)
}

type organisationRequest struct {
	Name string `json:"name" example:"Acme Corp"`
}

const invalidOrganisationID = "invalid organisation ID"

// Create godoc
// @Summary     Create an organisation
// @Description The name must be unique, ignoring case. An "admin" group is created for the organisation automatically.
// @Tags        organisations
// @Accept      json
// @Produce     json
// @Param       body body     organisationRequest true "Organisation to create"
// @Success     201  {object} domain.Organisation
// @Failure     400  {object} util.ErrorResponse
// @Failure     409  {object} util.ErrorResponse
// @Failure     500  {object} util.ErrorResponse
// @Router      /organisations [post]
func (h *OrganisationHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req organisationRequest
	if err := decodeJSON(w, r, &req); err != nil {
		util.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}

	org, err := h.service.Create(r.Context(), domain.Organisation{Name: req.Name})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusCreated, org)
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

// GetByID godoc
// @Summary Get an organisation by ID
// @Tags    organisations
// @Produce json
// @Param   id  path     int true "Organisation ID"
// @Success 200 {object} domain.Organisation
// @Failure 400 {object} util.ErrorResponse
// @Failure 404 {object} util.ErrorResponse
// @Failure 500 {object} util.ErrorResponse
// @Router  /organisations/{id} [get]
func (h *OrganisationHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidOrganisationID)
	if !ok {
		return
	}

	org, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, org)
}

// Update godoc
// @Summary     Update an organisation
// @Description The new name must be unique, ignoring case.
// @Tags        organisations
// @Accept      json
// @Produce     json
// @Param       id   path     int                 true "Organisation ID"
// @Param       body body     organisationRequest true "New organisation details"
// @Success     200  {object} domain.Organisation
// @Failure     400  {object} util.ErrorResponse
// @Failure     404  {object} util.ErrorResponse
// @Failure     409  {object} util.ErrorResponse
// @Failure     500  {object} util.ErrorResponse
// @Router      /organisations/{id} [put]
func (h *OrganisationHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidOrganisationID)
	if !ok {
		return
	}

	var req organisationRequest
	if err := decodeJSON(w, r, &req); err != nil {
		util.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}

	org, err := h.service.Update(r.Context(), domain.Organisation{ID: id, Name: req.Name})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, org)
}

// Delete godoc
// @Summary     Delete an organisation
// @Description Also deletes the organisation's groups. Fails with 409 while any of those groups still has users.
// @Tags        organisations
// @Param       id  path int true "Organisation ID"
// @Success     204
// @Failure     400 {object} util.ErrorResponse
// @Failure     404 {object} util.ErrorResponse
// @Failure     409 {object} util.ErrorResponse
// @Failure     500 {object} util.ErrorResponse
// @Router  /organisations/{id} [delete]
func (h *OrganisationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathID(w, r, invalidOrganisationID)
	if !ok {
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
