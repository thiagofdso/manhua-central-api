package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
)

type CatalogHandler struct {
	repo repository.CanonicalRepository
}

func NewCatalogHandler(repo repository.CanonicalRepository) *CatalogHandler {
	return &CatalogHandler{repo: repo}
}

func (h *CatalogHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/catalog", h.List)
	mux.HandleFunc("GET /api/catalog/{slug}", h.GetBySlug)
}

func (h *CatalogHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize < 1 {
		pageSize = 20
	}

	var updateDay *int16
	if dayStr := q.Get("update_day"); dayStr != "" {
		if day, err := strconv.Atoi(dayStr); err == nil && day >= 0 && day <= 6 {
			d := int16(day)
			updateDay = &d
		}
	}

	var isHiatus *bool
	if hiatusStr := q.Get("is_hiatus"); hiatusStr != "" {
		b := hiatusStr == "true" || hiatusStr == "1"
		isHiatus = &b
	}

	filter := domain.CatalogFilter{
		Query:     strings.TrimSpace(q.Get("q")),
		Domain:    strings.TrimSpace(q.Get("domain")),
		UpdateDay: updateDay,
		IsHiatus:  isHiatus,
		SortBy:    strings.TrimSpace(q.Get("sort")),
		Page:      page,
		PageSize:  pageSize,
	}

	res, err := h.repo.List(r.Context(), filter)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "failed to query catalog: "+err.Error())
		return
	}

	sendJSON(w, http.StatusOK, res)
}

func (h *CatalogHandler) GetBySlug(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slug := r.PathValue("slug")
	if slug == "" {
		slug = strings.TrimPrefix(r.URL.Path, "/api/catalog/")
	}
	slug = strings.Trim(slug, "/")

	if slug == "" {
		sendJSONError(w, http.StatusBadRequest, "slug is required")
		return
	}

	manhua, err := h.repo.GetBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, repository.ErrCanonicalNotFound) {
			sendJSONError(w, http.StatusNotFound, "manhua not found")
			return
		}
		sendJSONError(w, http.StatusInternalServerError, "failed to get manhua: "+err.Error())
		return
	}

	sendJSON(w, http.StatusOK, manhua)
}
