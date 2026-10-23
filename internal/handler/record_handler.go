package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"crud/internal/domain"
	"crud/internal/service"
	"crud/internal/util"
)

type RecordHandler struct {
	service *service.TaskService
}

func NewRecordHandler(svc *service.TaskService) *RecordHandler {
	return &RecordHandler{service: svc}
}

// Register adds the record routes to mux.
func (h *RecordHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/records", h.Create)
	mux.HandleFunc("GET /api/v1/records", h.GetAll)
	mux.HandleFunc("GET /api/v1/records/{id}", h.GetByID)
	mux.HandleFunc("PUT /api/v1/records/{id}", h.Update)
	mux.HandleFunc("DELETE /api/v1/records/{id}", h.Delete)
}

type recordRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type createRecordRequest struct {
	UserID      int64  `json:"user_id" example:"1"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB limit

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return err
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("request must contain only one JSON value")
		}
		return err
	}

	return nil
}

// Create godoc
// @Summary     Create a record for a user
// @Description The record belongs to the user given by user_id (404 if the user doesn't exist).
// @Tags        records
// @Accept      json
// @Produce     json
// @Param       body body     createRecordRequest true "Record to create"
// @Success     201  {object} domain.Task
// @Failure     400  {object} util.ErrorResponse
// @Failure     404  {object} util.ErrorResponse
// @Failure     500  {object} util.ErrorResponse
// @Router      /records [post]
func (h *RecordHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRecordRequest

	if err := decodeJSON(w, r, &req); err != nil {
		util.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}

	record, err := h.service.Create(r.Context(), domain.Task{
		UserID:      req.UserID,
		Title:       req.Title,
		Description: req.Description,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusCreated, record)
}

// GetAll godoc
// @Summary     List the records of a user
// @Description Returns only the records that belong to the given user (an empty list if they have none).
// @Tags        records
// @Produce     json
// @Param       user_id query    int true "User ID"
// @Success     200     {array}  domain.Task
// @Failure     400     {object} util.ErrorResponse
// @Failure     404     {object} util.ErrorResponse
// @Failure     500     {object} util.ErrorResponse
// @Router      /records [get]
func (h *RecordHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.ParseInt(r.URL.Query().Get("user_id"), 10, 64)
	if err != nil || userID <= 0 {
		util.Error(w, http.StatusBadRequest, "user_id query parameter is required and must be a positive integer")
		return
	}

	records, err := h.service.ListByUser(r.Context(), userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, records)
}

// GetByID godoc
// @Summary  Get a record by ID
// @Tags     records
// @Produce  json
// @Param    id  path     int true "Record ID"
// @Success  200 {object} domain.Task
// @Failure  400 {object} util.ErrorResponse
// @Failure  404 {object} util.ErrorResponse
// @Failure  500 {object} util.ErrorResponse
// @Router   /records/{id} [get]
func (h *RecordHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	record, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, record)
}

// Update godoc
// @Summary  Update a record
// @Tags     records
// @Accept   json
// @Produce  json
// @Param    id   path     int           true "Record ID"
// @Param    body body     recordRequest true "New title and description"
// @Success  200  {object} domain.Task
// @Failure  400  {object} util.ErrorResponse
// @Failure  404  {object} util.ErrorResponse
// @Failure  500  {object} util.ErrorResponse
// @Router   /records/{id} [put]
func (h *RecordHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	var req recordRequest
	if err := decodeJSON(w, r, &req); err != nil {
		util.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}

	record, err := h.service.Update(r.Context(), domain.Task{
		ID:          id,
		Title:       req.Title,
		Description: req.Description,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, record)
}

// Delete godoc
// @Summary  Delete a record
// @Tags     records
// @Produce  json
// @Param    id  path     int true "Record ID"
// @Success  204
// @Failure  400 {object} util.ErrorResponse
// @Failure  404 {object} util.ErrorResponse
// @Failure  500 {object} util.ErrorResponse
// @Router   /records/{id} [delete]
func (h *RecordHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	return parsePathID(w, r, "invalid record ID")
}

func parsePathID(w http.ResponseWriter, r *http.Request, message string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		util.Error(w, http.StatusBadRequest, message)
		return 0, false
	}

	return id, true
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidTask),
		errors.Is(err, domain.ErrInvalidTaskID),
		errors.Is(err, domain.ErrInvalidUser),
		errors.Is(err, domain.ErrInvalidUserID),
		errors.Is(err, domain.ErrInvalidGroup),
		errors.Is(err, domain.ErrInvalidGroupID),
		errors.Is(err, domain.ErrInvalidOrganisation),
		errors.Is(err, domain.ErrInvalidOrganisationID),
		errors.Is(err, domain.ErrGroupNotInOrganisation):
		util.Error(w, http.StatusBadRequest, err.Error())

	case errors.Is(err, domain.ErrTaskNotFound):
		util.Error(w, http.StatusNotFound, "record not found")

	case errors.Is(err, domain.ErrUserNotFound),
		errors.Is(err, domain.ErrGroupNotFound),
		errors.Is(err, domain.ErrOrganisationNotFound):
		util.Error(w, http.StatusNotFound, err.Error())

	case errors.Is(err, domain.ErrEmailTaken),
		errors.Is(err, domain.ErrGroupExists),
		errors.Is(err, domain.ErrGroupInUse),
		errors.Is(err, domain.ErrUserInUse),
		errors.Is(err, domain.ErrOrganisationExists),
		errors.Is(err, domain.ErrOrganisationInUse):
		util.Error(w, http.StatusConflict, err.Error())

	default:
		// Keep internal database details out of public responses.
		util.Error(w, http.StatusInternalServerError, "internal server error")
	}
}

// Prevent accidental whitespace-only titles from being treated as valid
// by future handlers that might construct requests differently.
var _ = strings.TrimSpace
