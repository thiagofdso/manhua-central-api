package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/service"
)

type BatchHandler struct {
	batchService *service.BatchService
}

func NewBatchHandler(batchService *service.BatchService) *BatchHandler {
	return &BatchHandler{batchService: batchService}
}

func (h *BatchHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/reader/batch", h.GetBatchChapters)
	mux.HandleFunc("GET /api/manhuas/{id}/batch-chapter-images", h.GetBatchChapters)
}

// GetBatchChapters handles batch chapter extraction requests with strict range and ceiling validation.
func (h *BatchHandler) GetBatchChapters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 1. Resolve Canonical Manhua ID (from path or query)
	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = r.URL.Query().Get("canonicalId")
	}
	if idStr == "" {
		// Fallback path prefix parsing
		if strings.HasPrefix(r.URL.Path, "/api/manhuas/") {
			idStr = extractSubpathID(r.URL.Path, "/api/manhuas/", "/batch-chapter-images")
		}
	}

	if idStr == "" {
		sendJSONError(w, http.StatusBadRequest, "canonicalId or manhua id is required")
		return
	}

	canonicalID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		sendJSONError(w, http.StatusBadRequest, "invalid canonical manhua id")
		return
	}

	// 2. Parse start and end parameters
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	if startStr == "" || endStr == "" {
		sendJSONError(w, http.StatusBadRequest, "both 'start' and 'end' query parameters are required")
		return
	}

	start, err := strconv.Atoi(startStr)
	if err != nil {
		sendJSONError(w, http.StatusBadRequest, "invalid 'start' parameter: must be an integer")
		return
	}

	end, err := strconv.Atoi(endStr)
	if err != nil {
		sendJSONError(w, http.StatusBadRequest, "invalid 'end' parameter: must be an integer")
		return
	}

	// 3. Delegate to BatchService
	resp, err := h.batchService.ExtractBatchChapters(r.Context(), canonicalID, start, end)
	if err != nil {
		if errors.Is(err, service.ErrInvalidBatchRange) ||
			errors.Is(err, service.ErrBatchSizeExceeded) ||
			errors.Is(err, service.ErrExceedsLastChapterCeiling) {
			sendJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, service.ErrCanonicalManhuaNotFound) {
			sendJSONError(w, http.StatusNotFound, "canonical manhua not found")
			return
		}
		sendJSONError(w, http.StatusInternalServerError, "failed to extract batch chapters: "+err.Error())
		return
	}

	sendJSON(w, http.StatusOK, resp)
}
