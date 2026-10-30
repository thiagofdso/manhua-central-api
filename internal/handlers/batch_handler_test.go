package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/handlers"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/service"
)

type mockCanonicalRepoForBatchHandler struct {
	manhua *domain.CanonicalManhua
}

func (m *mockCanonicalRepoForBatchHandler) Create(ctx context.Context, manhua *domain.CanonicalManhua) error {
	return nil
}
func (m *mockCanonicalRepoForBatchHandler) GetByID(ctx context.Context, id int64) (*domain.CanonicalManhua, error) {
	if m.manhua != nil && m.manhua.ID == id {
		return m.manhua, nil
	}
	return nil, service.ErrCanonicalManhuaNotFound
}
func (m *mockCanonicalRepoForBatchHandler) GetBySlug(ctx context.Context, slug string) (*domain.CanonicalManhua, error) {
	return nil, nil
}
func (m *mockCanonicalRepoForBatchHandler) GetByIDs(ctx context.Context, ids []int64) ([]*domain.CanonicalManhua, error) {
	return nil, nil
}
func (m *mockCanonicalRepoForBatchHandler) Update(ctx context.Context, manhua *domain.CanonicalManhua) error {
	return nil
}
func (m *mockCanonicalRepoForBatchHandler) List(ctx context.Context, filter domain.CatalogFilter) (*domain.CatalogSearchResult, error) {
	return nil, nil
}
func (m *mockCanonicalRepoForBatchHandler) UpsertBatch(ctx context.Context, items []*domain.CanonicalManhua) error {
	return nil
}
func (m *mockCanonicalRepoForBatchHandler) ListDomains(ctx context.Context) ([]string, error) {
	if m.manhua != nil && m.manhua.PrimaryDomain != "" {
		return []string{m.manhua.PrimaryDomain}, nil
	}
	return nil, nil
}

func TestBatchHandler(t *testing.T) {
	lastCh := 50
	manhua := &domain.CanonicalManhua{
		ID:          1,
		Slug:        "solo-leveling",
		Title:       "Solo Leveling",
		LastChapter: &lastCh,
	}
	repo := &mockCanonicalRepoForBatchHandler{manhua: manhua}
	batchService := service.NewBatchService(repo, 5, nil)
	batchHandler := handlers.NewBatchHandler(batchService)

	mux := http.NewServeMux()
	batchHandler.RegisterRoutes(mux)

	t.Run("GET /api/reader/batch returns 400 when missing parameters", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/reader/batch?start=1", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("GET /api/reader/batch returns 400 when batch size exceeds 50 (RN-05.1)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/reader/batch?canonicalId=1&start=1&end=52", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "limit of 50 chapters")
	})

	t.Run("GET /api/reader/batch returns 400 when end exceeds lastChapter ceiling (RN-05.1)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/reader/batch?canonicalId=1&start=45&end=55", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "ceiling")
	})

	t.Run("GET /api/reader/batch succeeds for valid range", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/reader/batch?canonicalId=1&start=1&end=3", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp domain.BatchChaptersResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, int64(1), resp.CanonicalManhuaID)
		assert.Equal(t, 3, resp.TotalChapters)
		assert.Len(t, resp.Chapters, 3)
	})

	t.Run("GET /api/manhuas/{id}/batch-chapter-images succeeds via alternate path", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/manhuas/1/batch-chapter-images?start=1&end=2", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp domain.BatchChaptersResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, int64(1), resp.CanonicalManhuaID)
		assert.Equal(t, 2, resp.TotalChapters)
	})
}
