package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/handlers"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
)

type mockCanonicalRepo struct {
	items []*domain.CanonicalManhua
}

func (m *mockCanonicalRepo) Create(ctx context.Context, item *domain.CanonicalManhua) error {
	m.items = append(m.items, item)
	return nil
}

func (m *mockCanonicalRepo) GetByID(ctx context.Context, id int64) (*domain.CanonicalManhua, error) {
	for _, it := range m.items {
		if it.ID == id {
			return it, nil
		}
	}
	return nil, repository.ErrCanonicalNotFound
}

func (m *mockCanonicalRepo) GetBySlug(ctx context.Context, slug string) (*domain.CanonicalManhua, error) {
	for _, it := range m.items {
		if it.Slug == slug {
			return it, nil
		}
	}
	return nil, repository.ErrCanonicalNotFound
}

func (m *mockCanonicalRepo) GetByIDs(ctx context.Context, ids []int64) ([]*domain.CanonicalManhua, error) {
	var res []*domain.CanonicalManhua
	for _, id := range ids {
		for _, it := range m.items {
			if it.ID == id {
				res = append(res, it)
				break
			}
		}
	}
	return res, nil
}

func (m *mockCanonicalRepo) Update(ctx context.Context, item *domain.CanonicalManhua) error {
	for i, it := range m.items {
		if it.ID == item.ID {
			m.items[i] = item
			return nil
		}
	}
	return repository.ErrCanonicalNotFound
}

func (m *mockCanonicalRepo) List(ctx context.Context, filter domain.CatalogFilter) (*domain.CatalogSearchResult, error) {
	var result []*domain.CanonicalManhua
	for _, it := range m.items {
		if filter.Domain != "" && it.PrimaryDomain != filter.Domain {
			continue
		}
		result = append(result, it)
	}
	return &domain.CatalogSearchResult{
		Items:      result,
		Total:      len(result),
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: 1,
	}, nil
}

func (m *mockCanonicalRepo) UpsertBatch(ctx context.Context, items []*domain.CanonicalManhua) error {
	m.items = append(m.items, items...)
	return nil
}

func TestCatalogHandler_List(t *testing.T) {
	sampleManhua := &domain.CanonicalManhua{
		ID:            101,
		Slug:          "overgeared",
		Title:         "Overgeared",
		PrimaryDomain: "asurascans.com",
		URLTemplate:   "https://asurascans.com/comics/overgeared/chapter/{chapter}",
		TotalViews:    15000,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	repo := &mockCanonicalRepo{items: []*domain.CanonicalManhua{sampleManhua}}
	handler := handlers.NewCatalogHandler(repo)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 1. List all
	req := httptest.NewRequest(http.MethodGet, "/api/catalog?q=over&domain=asurascans.com&page=1&page_size=10", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp domain.CatalogSearchResult
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 1, resp.Total)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "Overgeared", resp.Items[0].Title)
}

func TestCatalogHandler_GetBySlug(t *testing.T) {
	sampleManhua := &domain.CanonicalManhua{
		ID:            101,
		Slug:          "overgeared",
		Title:         "Overgeared",
		PrimaryDomain: "asurascans.com",
		URLTemplate:   "https://asurascans.com/comics/overgeared/chapter/{chapter}",
		TotalViews:    15000,
	}

	repo := &mockCanonicalRepo{items: []*domain.CanonicalManhua{sampleManhua}}
	handler := handlers.NewCatalogHandler(repo)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 1. Existing slug
	req := httptest.NewRequest(http.MethodGet, "/api/catalog/overgeared", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var fetched domain.CanonicalManhua
	err := json.Unmarshal(rr.Body.Bytes(), &fetched)
	require.NoError(t, err)
	assert.Equal(t, "Overgeared", fetched.Title)
	assert.Equal(t, "overgeared", fetched.Slug)

	// 2. Non-existing slug
	req404 := httptest.NewRequest(http.MethodGet, "/api/catalog/non-existing", nil)
	rr404 := httptest.NewRecorder()
	mux.ServeHTTP(rr404, req404)

	assert.Equal(t, http.StatusNotFound, rr404.Code)
}
