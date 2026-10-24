package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/proxy"
)

type mockCanonicalReaderRepo struct {
	manhua *domain.CanonicalManhua
}

func (m *mockCanonicalReaderRepo) Create(ctx context.Context, item *domain.CanonicalManhua) error {
	return nil
}

func (m *mockCanonicalReaderRepo) GetByID(ctx context.Context, id int64) (*domain.CanonicalManhua, error) {
	if m.manhua != nil && m.manhua.ID == id {
		return m.manhua, nil
	}
	return nil, nil
}

func (m *mockCanonicalReaderRepo) GetBySlug(ctx context.Context, slug string) (*domain.CanonicalManhua, error) {
	if m.manhua != nil && m.manhua.Slug == slug {
		return m.manhua, nil
	}
	return nil, nil
}

func (m *mockCanonicalReaderRepo) GetByIDs(ctx context.Context, ids []int64) ([]*domain.CanonicalManhua, error) {
	if m.manhua != nil {
		return []*domain.CanonicalManhua{m.manhua}, nil
	}
	return []*domain.CanonicalManhua{}, nil
}

func (m *mockCanonicalReaderRepo) Update(ctx context.Context, item *domain.CanonicalManhua) error {
	return nil
}

func (m *mockCanonicalReaderRepo) List(ctx context.Context, filter domain.CatalogFilter) (*domain.CatalogSearchResult, error) {
	return nil, nil
}

func (m *mockCanonicalReaderRepo) UpsertBatch(ctx context.Context, items []*domain.CanonicalManhua) error {
	return nil
}

func TestReaderHandler_ChapterImages_Success(t *testing.T) {
	lastCh := 50
	repo := &mockCanonicalReaderRepo{
		manhua: &domain.CanonicalManhua{
			ID:            1,
			Slug:          "solo-leveling",
			Title:         "Solo Leveling",
			LastChapter:   &lastCh,
			URLTemplate:   "https://asurascans.com/comics/solo-leveling/chapter/{chapter}",
			PrimaryDomain: "asurascans.com",
		},
	}

	guard := proxy.NewSSRFGuard([]string{"asurascans.com", "images.unsplash.com"})
	streamer := proxy.NewStreamer(guard)
	handler := NewReaderHandler(streamer, repo)

	req := httptest.NewRequest(http.MethodGet, "/api/reader/chapter-images?slug=solo-leveling&chapter=10", nil)
	rec := httptest.NewRecorder()

	handler.GetChapterImages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp ChapterImagesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode json response: %v", err)
	}

	if resp.Slug != "solo-leveling" || resp.Chapter != 10 {
		t.Errorf("expected slug 'solo-leveling' and chapter 10, got %s / %d", resp.Slug, resp.Chapter)
	}

	if len(resp.Images) == 0 {
		t.Errorf("expected at least 1 image panel, got %d", len(resp.Images))
	}

	if resp.Images[0].ProxyURL == "" {
		t.Errorf("expected proxyUrl to be populated")
	}
}

func TestReaderHandler_ChapterImages_ExceedsLastChapter(t *testing.T) {
	lastCh := 50
	repo := &mockCanonicalReaderRepo{
		manhua: &domain.CanonicalManhua{
			ID:          1,
			Slug:        "solo-leveling",
			Title:       "Solo Leveling",
			LastChapter: &lastCh,
		},
	}

	guard := proxy.NewSSRFGuard([]string{"asurascans.com"})
	streamer := proxy.NewStreamer(guard)
	handler := NewReaderHandler(streamer, repo)

	req := httptest.NewRequest(http.MethodGet, "/api/reader/chapter-images?slug=solo-leveling&chapter=51", nil)
	rec := httptest.NewRecorder()

	handler.GetChapterImages(rec, req)

	// RN-03.2: Ceiling at lastChapter returns 400
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for chapter exceeding lastChapter, got %d", rec.Code)
	}
}

func TestReaderHandler_ProxyImage_BlocksSSRF(t *testing.T) {
	repo := &mockCanonicalReaderRepo{}
	guard := proxy.NewSSRFGuard([]string{"asurascans.com"})
	streamer := proxy.NewStreamer(guard)
	handler := NewReaderHandler(streamer, repo)

	req := httptest.NewRequest(http.MethodGet, "/api/reader/proxy-image?url="+url.QueryEscape("http://127.0.0.1:8080/metrics"), nil)
	rec := httptest.NewRecorder()

	handler.ProxyImage(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 Forbidden for SSRF attempt, got %d", rec.Code)
	}
}
