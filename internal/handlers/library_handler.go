package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/middleware"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/service"
)

type LibraryHandler struct {
	libraryService *service.LibraryService
}

func NewLibraryHandler(libraryService *service.LibraryService) *LibraryHandler {
	return &LibraryHandler{libraryService: libraryService}
}

func (h *LibraryHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *middleware.AuthMiddleware) {
	mux.Handle("GET /api/library", authMiddleware.RequireAuth(http.HandlerFunc(h.List)))
	mux.Handle("POST /api/library", authMiddleware.RequireAuth(http.HandlerFunc(h.Add)))
	mux.Handle("PATCH /api/library/{id}/progress", authMiddleware.RequireAuth(http.HandlerFunc(h.UpdateProgress)))
	mux.Handle("PATCH /api/library/{id}/favorite", authMiddleware.RequireAuth(http.HandlerFunc(h.ToggleFavorite)))
	mux.Handle("PUT /api/library/reorder", authMiddleware.RequireAuth(http.HandlerFunc(h.Reorder)))
	mux.Handle("DELETE /api/library/{id}", authMiddleware.RequireAuth(http.HandlerFunc(h.Remove)))
}

func (h *LibraryHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		sendJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	items, err := h.libraryService.GetUserLibrary(r.Context(), userID)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "failed to get library: "+err.Error())
		return
	}

	if items == nil {
		items = []*domain.UserLibrary{}
	}

	sendJSON(w, http.StatusOK, items)
}

func (h *LibraryHandler) Add(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		sendJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var dto domain.AddToLibraryDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		sendJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	item, err := h.libraryService.AddToLibrary(r.Context(), userID, dto)
	if err != nil {
		if errors.Is(err, service.ErrAlreadyInLibrary) {
			sendJSONError(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, service.ErrChapterExceedsLastChapter) {
			sendJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, repository.ErrCanonicalNotFound) {
			sendJSONError(w, http.StatusNotFound, "canonical manhua not found")
			return
		}
		sendJSONError(w, http.StatusInternalServerError, "failed to add to library: "+err.Error())
		return
	}

	sendJSON(w, http.StatusCreated, item)
}

func (h *LibraryHandler) UpdateProgress(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		sendJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		id = extractSubpathID(r.URL.Path, "/api/library/", "/progress")
	}

	var dto domain.UpdateProgressDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		sendJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	item, err := h.libraryService.UpdateProgress(r.Context(), userID, id, dto.CurrentChapter)
	if err != nil {
		if errors.Is(err, service.ErrChapterExceedsLastChapter) || errors.Is(err, service.ErrInvalidChapterProgress) {
			sendJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, repository.ErrLibraryItemNotFound) {
			sendJSONError(w, http.StatusNotFound, "library item not found")
			return
		}
		sendJSONError(w, http.StatusInternalServerError, "failed to update progress: "+err.Error())
		return
	}

	sendJSON(w, http.StatusOK, item)
}

func (h *LibraryHandler) ToggleFavorite(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		sendJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		id = extractSubpathID(r.URL.Path, "/api/library/", "/favorite")
	}

	var dto domain.ToggleFavoriteDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		sendJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	item, err := h.libraryService.ToggleFavorite(r.Context(), userID, id, dto.IsFavorite)
	if err != nil {
		if errors.Is(err, repository.ErrLibraryItemNotFound) {
			sendJSONError(w, http.StatusNotFound, "library item not found")
			return
		}
		sendJSONError(w, http.StatusInternalServerError, "failed to toggle favorite: "+err.Error())
		return
	}

	sendJSON(w, http.StatusOK, item)
}

func (h *LibraryHandler) Reorder(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		sendJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var dto domain.ReorderLibraryDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		sendJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.libraryService.ReorderLibrary(r.Context(), userID, dto); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "failed to reorder library: "+err.Error())
		return
	}

	sendJSON(w, http.StatusOK, map[string]string{"status": "reordered"})
}

func (h *LibraryHandler) Remove(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		sendJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		id = strings.TrimPrefix(r.URL.Path, "/api/library/")
	}

	if err := h.libraryService.RemoveFromLibrary(r.Context(), userID, id); err != nil {
		if errors.Is(err, repository.ErrLibraryItemNotFound) {
			sendJSONError(w, http.StatusNotFound, "library item not found")
			return
		}
		sendJSONError(w, http.StatusInternalServerError, "failed to remove from library: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func extractSubpathID(path, prefix, suffix string) string {
	s := strings.TrimPrefix(path, prefix)
	s = strings.TrimSuffix(s, suffix)
	return strings.Trim(s, "/")
}
