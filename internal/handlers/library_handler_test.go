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

type mockLibRepoForHandler struct {
	items map[string]*domain.UserLibrary
}

func (m *mockLibRepoForHandler) Create(ctx context.Context, item *domain.UserLibrary) error {
	m.items[item.ID] = item
	return nil
}

func (m *mockLibRepoForHandler) GetByID(ctx context.Context, userID, id string) (*domain.UserLibrary, error) {
	if it, ok := m.items[id]; ok && it.UserID == userID {
		return it, nil
	}
	return nil, service.ErrAlreadyInLibrary
}

func (m *mockLibRepoForHandler) GetByUserAndManhuaID(ctx context.Context, userID string, manhuaID int64) (*domain.UserLibrary, error) {
	for _, it := range m.items {
		if it.UserID == userID && it.CanonicalManhuaID == manhuaID {
			return it, nil
		}
	}
	return nil, service.ErrAlreadyInLibrary
}

func (m *mockLibRepoForHandler) ListByUser(ctx context.Context, userID string) ([]*domain.UserLibrary, error) {
	var list []*domain.UserLibrary
	for _, it := range m.items {
		if it.UserID == userID {
			list = append(list, it)
		}
	}
	return list, nil
}

func (m *mockLibRepoForHandler) Update(ctx context.Context, item *domain.UserLibrary) error {
	m.items[item.ID] = item
	return nil
}

func (m *mockLibRepoForHandler) UpdateOrderBatch(ctx context.Context, userID string, items []*domain.UserLibrary) error {
	for _, it := range items {
		m.items[it.ID] = it
	}
	return nil
}

func (m *mockLibRepoForHandler) Delete(ctx context.Context, userID, id string) error {
	delete(m.items, id)
	return nil
}

func (m *mockLibRepoForHandler) GetMinCustomOrder(ctx context.Context, userID string, isFavorite bool) (int, error) {
	return 0, nil
}

type mockCanonicalForHandler struct{}

func (m *mockCanonicalForHandler) GetByID(ctx context.Context, id int64) (*domain.CanonicalManhua, error) {
	lastCh := 100
	return &domain.CanonicalManhua{
		ID:          id,
		Title:       "Test Manhua",
		LastChapter: &lastCh,
	}, nil
}

func TestLibraryHandler_Endpoints(t *testing.T) {
	repo := &mockLibRepoForHandler{items: make(map[string]*domain.UserLibrary)}
	canonical := &mockCanonicalForHandler{}
	libService := service.NewLibraryService(repo, canonical)

	jwtService := auth.NewJWTService("secret-key-32-chars-minimum-length-req", "refresh-secret-32-chars-minimum", 15*time.Minute, 7*24*time.Hour)
	authMiddleware := middleware.NewAuthMiddleware(jwtService)
	handler := handlers.NewLibraryHandler(libService)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux, authMiddleware)

	userID := "user-handler-test"
	user := &domain.User{
		ID:          userID,
		Email:       "test@example.com",
		DisplayName: "Tester",
		Role:        domain.RoleReader,
	}
	token, _, _, err := jwtService.GenerateTokenPair(user)
	require.NoError(t, err)

	// 1. POST /api/library (Add item)
	addPayload := domain.AddToLibraryDTO{CanonicalManhuaID: 10, CurrentChapter: 1, IsFavorite: true}
	body, _ := json.Marshal(addPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/library", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	var created domain.UserLibrary
	err = json.Unmarshal(rr.Body.Bytes(), &created)
	require.NoError(t, err)
	assert.Equal(t, int64(10), created.CanonicalManhuaID)
	assert.True(t, created.IsFavorite)

	// 2. GET /api/library (List items)
	reqGet := httptest.NewRequest(http.MethodGet, "/api/library", nil)
	reqGet.Header.Set("Authorization", "Bearer "+token)
	rrGet := httptest.NewRecorder()
	mux.ServeHTTP(rrGet, reqGet)

	assert.Equal(t, http.StatusOK, rrGet.Code)
	var list []*domain.UserLibrary
	err = json.Unmarshal(rrGet.Body.Bytes(), &list)
	require.NoError(t, err)
	require.Len(t, list, 1)

	// 3. PATCH /api/library/{id}/progress
	progressPayload := domain.UpdateProgressDTO{CurrentChapter: 15}
	pBody, _ := json.Marshal(progressPayload)
	reqPatch := httptest.NewRequest(http.MethodPatch, "/api/library/"+created.ID+"/progress", bytes.NewReader(pBody))
	reqPatch.Header.Set("Authorization", "Bearer "+token)
	rrPatch := httptest.NewRecorder()
	mux.ServeHTTP(rrPatch, reqPatch)

	assert.Equal(t, http.StatusOK, rrPatch.Code)
	var updated domain.UserLibrary
	err = json.Unmarshal(rrPatch.Body.Bytes(), &updated)
	require.NoError(t, err)
	assert.Equal(t, 15, updated.CurrentChapter)

	// 4. PATCH /api/library/{id}/favorite
	favPayload := domain.ToggleFavoriteDTO{IsFavorite: false}
	fBody, _ := json.Marshal(favPayload)
	reqFav := httptest.NewRequest(http.MethodPatch, "/api/library/"+created.ID+"/favorite", bytes.NewReader(fBody))
	reqFav.Header.Set("Authorization", "Bearer "+token)
	rrFav := httptest.NewRecorder()
	mux.ServeHTTP(rrFav, reqFav)

	assert.Equal(t, http.StatusOK, rrFav.Code)
	var updatedFav domain.UserLibrary
	err = json.Unmarshal(rrFav.Body.Bytes(), &updatedFav)
	require.NoError(t, err)
	assert.False(t, updatedFav.IsFavorite)

	// 5. PUT /api/library/reorder
	reorderPayload := domain.ReorderLibraryDTO{ItemIDs: []string{created.ID}}
	rBody, _ := json.Marshal(reorderPayload)
	reqReorder := httptest.NewRequest(http.MethodPut, "/api/library/reorder", bytes.NewReader(rBody))
	reqReorder.Header.Set("Authorization", "Bearer "+token)
	rrReorder := httptest.NewRecorder()
	mux.ServeHTTP(rrReorder, reqReorder)

	assert.Equal(t, http.StatusOK, rrReorder.Code)

	// 6. DELETE /api/library/{id}
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/library/"+created.ID, nil)
	reqDel.Header.Set("Authorization", "Bearer "+token)
	rrDel := httptest.NewRecorder()
	mux.ServeHTTP(rrDel, reqDel)

	assert.Equal(t, http.StatusNoContent, rrDel.Code)
}
