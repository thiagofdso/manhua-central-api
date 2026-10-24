package handlers

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/middleware"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/service"
)

type RankingHandler struct {
	rankingService *service.RankingService
}

func NewRankingHandler(rankingService *service.RankingService) *RankingHandler {
	return &RankingHandler{rankingService: rankingService}
}

func (h *RankingHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *middleware.AuthMiddleware) {
	mux.Handle("POST /api/ratings", authMiddleware.RequireAuth(http.HandlerFunc(h.SubmitRating)))
	mux.Handle("GET /api/ratings/me/{id}", authMiddleware.RequireAuth(http.HandlerFunc(h.GetUserRating)))
	mux.Handle("GET /api/rankings", http.HandlerFunc(h.GetRankings))
	mux.Handle("POST /api/reader/view", http.HandlerFunc(h.RecordView))
}

// SubmitRating handles POST /api/ratings to record a user's 1-5 star review.
func (h *RankingHandler) SubmitRating(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		sendJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req domain.SubmitRatingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Score < 1 || req.Score > 5 {
		sendJSONError(w, http.StatusBadRequest, "score must be between 1 and 5")
		return
	}

	manhua, err := h.rankingService.SubmitRating(r.Context(), userID, req.CanonicalManhuaID, req.Score)
	if err != nil {
		if errors.Is(err, repository.ErrInvalidScore) {
			sendJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		sendJSONError(w, http.StatusInternalServerError, "failed to submit rating: "+err.Error())
		return
	}

	sendJSON(w, http.StatusOK, manhua)
}

// GetUserRating handles GET /api/ratings/me/{id} to retrieve the current user's rating for a specific manhua.
func (h *RankingHandler) GetUserRating(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		sendJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		idStr = strings.TrimPrefix(r.URL.Path, "/api/ratings/me/")
	}

	manhuaID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		sendJSONError(w, http.StatusBadRequest, "invalid canonical manhua id")
		return
	}

	score, err := h.rankingService.GetUserRating(r.Context(), userID, manhuaID)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "failed to get rating: "+err.Error())
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"canonicalManhuaId": manhuaID,
		"score":             score,
	})
}

// GetRankings handles GET /api/rankings?type={top_rated|daily|weekly|all_time}&limit={n}.
func (h *RankingHandler) GetRankings(w http.ResponseWriter, r *http.Request) {
	rankingType := r.URL.Query().Get("type")
	if rankingType == "" {
		rankingType = "top_rated"
	}

	limit := 20
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	rankings, err := h.rankingService.GetRankings(r.Context(), rankingType, limit)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "failed to get rankings: "+err.Error())
		return
	}

	if rankings == nil {
		rankings = []*domain.RankedManhua{}
	}

	sendJSON(w, http.StatusOK, rankings)
}

// RecordView handles POST /api/reader/view to record view activity with rate-limiting.
func (h *RankingHandler) RecordView(w http.ResponseWriter, r *http.Request) {
	var req domain.RecordViewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Identify client: use userID if authenticated, else IP address
	identifier := ""
	if userID, ok := middleware.GetUserIDFromContext(r.Context()); ok {
		identifier = userID
	} else {
		identifier = getClientIP(r)
	}

	if err := h.rankingService.RecordView(r.Context(), req.CanonicalManhuaID, identifier); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "failed to record view: "+err.Error())
		return
	}

	sendJSON(w, http.StatusOK, map[string]string{"status": "recorded"})
}

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
