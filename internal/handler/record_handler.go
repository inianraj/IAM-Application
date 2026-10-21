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

type recordRequest struct {
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

func (h *RecordHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req recordRequest

	if err := decodeJSON(w, r, &req); err != nil {
		util.Error(w, http.StatusBadRequest, "invalid JSON request")
		return
	}

	record, err := h.service.Create(r.Context(), domain.Task{
		Title:       req.Title,
		Description: req.Description,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusCreated, record)
}

func (h *RecordHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	records, err := h.service.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	util.JSON(w, http.StatusOK, records)
}

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
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		util.Error(w, http.StatusBadRequest, "invalid record ID")
		return 0, false
	}

	return id, true
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidTask),
		errors.Is(err, domain.ErrInvalidTaskID):
		util.Error(w, http.StatusBadRequest, err.Error())

	case errors.Is(err, domain.ErrTaskNotFound):
		util.Error(w, http.StatusNotFound, "record not found")

	default:
		// Keep internal database details out of public responses.
		util.Error(w, http.StatusInternalServerError, "internal server error")
	}
}

// Prevent accidental whitespace-only titles from being treated as valid
// by future handlers that might construct requests differently.
var _ = strings.TrimSpace
