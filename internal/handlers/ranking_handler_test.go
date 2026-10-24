package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/auth"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/handlers"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/middleware"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/service"
)

type mockRatingRepoForHandler struct {
	submitVoteFunc  func(ctx context.Context, userID string, manhuaID int64, score int) (*domain.CanonicalManhua, error)
	getUserVoteFunc func(ctx context.Context, userID string, manhuaID int64) (*int, error)
}

func (m *mockRatingRepoForHandler) SubmitVote(ctx context.Context, userID string, manhuaID int64, score int) (*domain.CanonicalManhua, error) {
	if m.submitVoteFunc != nil {
		return m.submitVoteFunc(ctx, userID, manhuaID, score)
	}
	return nil, nil
}

func (m *mockRatingRepoForHandler) GetUserVote(ctx context.Context, userID string, manhuaID int64) (*int, error) {
	if m.getUserVoteFunc != nil {
		return m.getUserVoteFunc(ctx, userID, manhuaID)
	}
	return nil, nil
}

func (m *mockRatingRepoForHandler) SyncViewsToPostgres(ctx context.Context, views map[int64]int64) error {
	return nil
}

type mockCanonicalRepoForHandler struct {
	listFunc func(ctx context.Context, filter domain.CatalogFilter) (*domain.CatalogSearchResult, error)
}

func (m *mockCanonicalRepoForHandler) Create(ctx context.Context, manhua *domain.CanonicalManhua) error { return nil }
func (m *mockCanonicalRepoForHandler) GetByID(ctx context.Context, id int64) (*domain.CanonicalManhua, error) {
	return &domain.CanonicalManhua{ID: id, Title: "Test Manhua"}, nil
}
func (m *mockCanonicalRepoForHandler) GetBySlug(ctx context.Context, slug string) (*domain.CanonicalManhua, error) { return nil, nil }
func (m *mockCanonicalRepoForHandler) GetByIDs(ctx context.Context, ids []int64) ([]*domain.CanonicalManhua, error) { return nil, nil }
func (m *mockCanonicalRepoForHandler) Update(ctx context.Context, manhua *domain.CanonicalManhua) error { return nil }
func (m *mockCanonicalRepoForHandler) List(ctx context.Context, filter domain.CatalogFilter) (*domain.CatalogSearchResult, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, filter)
	}
	return &domain.CatalogSearchResult{
		Items: []*domain.CanonicalManhua{
			{ID: 10, Title: "Top Ranked", BayesianRating: 4.88},
		},
		Total: 1,
	}, nil
}
func (m *mockCanonicalRepoForHandler) UpsertBatch(ctx context.Context, items []*domain.CanonicalManhua) error { return nil }

func setupRankingHandlerTest() (*handlers.RankingHandler, *auth.JWTService, *middleware.AuthMiddleware) {
	jwtService := auth.NewJWTService("super_secret_test_key_minimum_32_bytes_long", "super_secret_test_key_minimum_32_bytes_long", 15*time.Minute, 7*24*time.Hour)
	authMiddleware := middleware.NewAuthMiddleware(jwtService)

	ratingRepo := &mockRatingRepoForHandler{
		submitVoteFunc: func(ctx context.Context, userID string, manhuaID int64, score int) (*domain.CanonicalManhua, error) {
			return &domain.CanonicalManhua{
				ID:             manhuaID,
				Title:          "Solo Leveling",
				BayesianRating: 4.85,
				TotalVotes:     10,
			}, nil
		},
		getUserVoteFunc: func(ctx context.Context, userID string, manhuaID int64) (*int, error) {
			s := 5
			return &s, nil
		},
	}

	canonicalRepo := &mockCanonicalRepoForHandler{}
	rankingService := service.NewRankingService(ratingRepo, canonicalRepo, nil)
	rankingHandler := handlers.NewRankingHandler(rankingService)

	return rankingHandler, jwtService, authMiddleware
}

func TestRankingHandler(t *testing.T) {
	rankingHandler, jwtService, authMiddleware := setupRankingHandlerTest()

	mux := http.NewServeMux()
	rankingHandler.RegisterRoutes(mux, authMiddleware)

	user := &domain.User{
		ID:          "user-uuid-1234",
		Email:       "test@example.com",
		DisplayName: "Tester",
		Role:        domain.RoleReader,
	}
	validToken, _, _, err := jwtService.GenerateTokenPair(user)
	require.NoError(t, err)

	t.Run("POST /api/ratings requires authentication", func(t *testing.T) {
		body := []byte(`{"canonicalManhuaId": 100, "score": 5}`)
		req := httptest.NewRequest(http.MethodPost, "/api/ratings", bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("POST /api/ratings rejects score outside 1-5 scale", func(t *testing.T) {
		body := []byte(`{"canonicalManhuaId": 100, "score": 6}`)
		req := httptest.NewRequest(http.MethodPost, "/api/ratings", bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer "+validToken)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("POST /api/ratings accepts valid 5-star rating", func(t *testing.T) {
		body := []byte(`{"canonicalManhuaId": 100, "score": 5}`)
		req := httptest.NewRequest(http.MethodPost, "/api/ratings", bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer "+validToken)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp domain.CanonicalManhua
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, int64(100), resp.ID)
		assert.Equal(t, 4.85, resp.BayesianRating)
	})

	t.Run("GET /api/ratings/me/{id} returns user score", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/ratings/me/100", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, float64(5), resp["score"])
	})

	t.Run("GET /api/rankings returns leaderboard items", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/rankings?type=top_rated&limit=10", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var list []*domain.RankedManhua
		err := json.NewDecoder(w.Body).Decode(&list)
		require.NoError(t, err)
		require.Len(t, list, 1)
		assert.Equal(t, 1, list[0].Rank)
		assert.Equal(t, 4.88, list[0].Score)
		assert.Equal(t, "Top Ranked", list[0].Canonical.Title)
	})

	t.Run("POST /api/reader/view increments view count", func(t *testing.T) {
		body := []byte(`{"canonicalManhuaId": 100}`)
		req := httptest.NewRequest(http.MethodPost, "/api/reader/view", bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]string
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, "recorded", resp["status"])
	})
}
