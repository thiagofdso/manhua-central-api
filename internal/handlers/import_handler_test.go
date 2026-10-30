package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/auth"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/handlers"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/providers"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/service"
)

type mockTestScraper struct {
	domain string
	name   string
}

func (m *mockTestScraper) Domain() string { return m.domain }
func (m *mockTestScraper) Name() string   { return m.name }
func (m *mockTestScraper) SearchOrBrowse(ctx context.Context, q string, page int) (*domain.ScrapeResult, error) {
	ch := 10
	return &domain.ScrapeResult{
		Items: []domain.ScrapedItem{
			{
				Title:         "Sample Manga",
				Slug:          "sample-manga",
				URLTemplate:   "https://sample.com/chapter/{chapter}",
				CoverURL:      "https://sample.com/c.jpg",
				LastChapter:   &ch,
				PrimaryDomain: "sample.com",
			},
		},
		Page:       page,
		TotalPages: 1,
	}, nil
}

func TestImportHandler_GetDomains(t *testing.T) {
	reg := providers.NewProviderRegistry()
	reg.Register(providers.NewAsuraScansScraper())

	svc := service.NewImportService(reg, nil, nil, nil)
	h := handlers.NewImportHandler(svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/import/domains", nil)
	w := httptest.NewRecorder()

	h.GetDomains(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestImportHandler_Browse(t *testing.T) {
	reg := providers.NewProviderRegistry()
	reg.Register(&mockTestScraper{domain: "sample.com", name: "Sample"})

	svc := service.NewImportService(reg, nil, nil, nil)
	h := handlers.NewImportHandler(svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/import/browse?domain=sample.com", nil)
	w := httptest.NewRecorder()

	h.Browse(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var res service.ImportBrowseResult
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(res.Items) != 1 || res.Items[0].Title != "Sample Manga" {
		t.Fatalf("unexpected browse items: %+v", res.Items)
	}
}

func TestImportHandler_ImportWorks_Authentication(t *testing.T) {
	reg := providers.NewProviderRegistry()
	jwtSvc := auth.NewJWTService("super_secret_test_jwt_key_32_chars!", "super_refresh_test_key_32_chars!", 15*time.Minute, 7*24*time.Hour)

	svc := service.NewImportService(reg, nil, nil, nil)
	h := handlers.NewImportHandler(svc, jwtSvc)

	// 1. Unauthenticated request should fail with 401
	payload := map[string]interface{}{
		"items": []map[string]interface{}{
			{
				"title":         "Sample Manga",
				"slug":          "sample-manga",
				"urlTemplate":   "https://sample.com/chapter/{chapter}",
				"coverUrl":      "https://sample.com/c.jpg",
				"primaryDomain": "sample.com",
			},
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/import", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.ImportWorks(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized, got %d", w.Code)
	}

	// 2. Authenticated request should pass
	user := &domain.User{
		ID:          "user-123",
		Email:       "reader@example.com",
		DisplayName: "Test Reader",
		Role:        domain.RoleReader,
	}
	token, _, _, err := jwtSvc.GenerateTokenPair(user)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	reqAuth := httptest.NewRequest(http.MethodPost, "/api/import", bytes.NewReader(body))
	reqAuth.Header.Set("Authorization", "Bearer "+token)
	wAuth := httptest.NewRecorder()

	h.ImportWorks(wAuth, reqAuth)
	if wAuth.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d: %s", wAuth.Code, wAuth.Body.String())
	}
}
