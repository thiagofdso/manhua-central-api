package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/service"
)

type mockCanonicalRepoForBatch struct {
	manhua *domain.CanonicalManhua
}

func (m *mockCanonicalRepoForBatch) Create(ctx context.Context, manhua *domain.CanonicalManhua) error {
	return nil
}

func (m *mockCanonicalRepoForBatch) GetByID(ctx context.Context, id int64) (*domain.CanonicalManhua, error) {
	if m.manhua != nil && m.manhua.ID == id {
		return m.manhua, nil
	}
	return nil, errors.New("not found")
}

func (m *mockCanonicalRepoForBatch) GetBySlug(ctx context.Context, slug string) (*domain.CanonicalManhua, error) {
	if m.manhua != nil && m.manhua.Slug == slug {
		return m.manhua, nil
	}
	return nil, errors.New("not found")
}

func (m *mockCanonicalRepoForBatch) GetByIDs(ctx context.Context, ids []int64) ([]*domain.CanonicalManhua, error) {
	if m.manhua != nil {
		return []*domain.CanonicalManhua{m.manhua}, nil
	}
	return []*domain.CanonicalManhua{}, nil
}

func (m *mockCanonicalRepoForBatch) Update(ctx context.Context, manhua *domain.CanonicalManhua) error {
	return nil
}

func (m *mockCanonicalRepoForBatch) List(ctx context.Context, filter domain.CatalogFilter) (*domain.CatalogSearchResult, error) {
	return &domain.CatalogSearchResult{}, nil
}

func (m *mockCanonicalRepoForBatch) UpsertBatch(ctx context.Context, items []*domain.CanonicalManhua) error {
	return nil
}

func (m *mockCanonicalRepoForBatch) ListDomains(ctx context.Context) ([]string, error) {
	if m.manhua != nil && m.manhua.PrimaryDomain != "" {
		return []string{m.manhua.PrimaryDomain}, nil
	}
	return nil, nil
}

func TestBatchService_Validation(t *testing.T) {
	lastCh := 40
	manhua := &domain.CanonicalManhua{
		ID:          1,
		Slug:        "test-manhua",
		Title:       "Test Manhua",
		LastChapter: &lastCh,
	}
	repo := &mockCanonicalRepoForBatch{manhua: manhua}
	mockExtractor := func(ctx context.Context, m *domain.CanonicalManhua, chapter int) ([]domain.BatchChapterImage, error) {
		return []domain.BatchChapterImage{
			{Index: 1, URL: "https://asurascans.com/p1.webp", ProxyURL: "/api/reader/proxy-image?url=p1"},
		}, nil
	}
	svc := service.NewBatchService(repo, 5, mockExtractor)
	ctx := context.Background()

	t.Run("Rejects start < 1", func(t *testing.T) {
		_, err := svc.ExtractBatchChapters(ctx, 1, 0, 10)
		assert.ErrorIs(t, err, service.ErrInvalidBatchRange)
	})

	t.Run("Rejects end < start", func(t *testing.T) {
		_, err := svc.ExtractBatchChapters(ctx, 1, 10, 5)
		assert.ErrorIs(t, err, service.ErrInvalidBatchRange)
	})

	t.Run("Rejects batch size exceeding 50 chapters (RN-05.1)", func(t *testing.T) {
		_, err := svc.ExtractBatchChapters(ctx, 1, 1, 52)
		assert.ErrorIs(t, err, service.ErrBatchSizeExceeded)
	})

	t.Run("Rejects range exceeding lastChapter ceiling (RN-05.1)", func(t *testing.T) {
		_, err := svc.ExtractBatchChapters(ctx, 1, 35, 45)
		assert.ErrorIs(t, err, service.ErrExceedsLastChapterCeiling)
	})

	t.Run("Rejects start beyond lastChapter", func(t *testing.T) {
		_, err := svc.ExtractBatchChapters(ctx, 1, 41, 42)
		assert.ErrorIs(t, err, service.ErrExceedsLastChapterCeiling)
	})
}

func TestBatchService_ConcurrentExtraction(t *testing.T) {
	lastCh := 100
	manhua := &domain.CanonicalManhua{
		ID:          10,
		Slug:        "overgeared",
		Title:       "Overgeared",
		LastChapter: &lastCh,
	}
	repo := &mockCanonicalRepoForBatch{manhua: manhua}
	mockExtractor := func(ctx context.Context, m *domain.CanonicalManhua, chapter int) ([]domain.BatchChapterImage, error) {
		return []domain.BatchChapterImage{
			{Index: 1, URL: "https://asurascans.com/p1.webp", ProxyURL: "/api/reader/proxy-image?url=p1"},
		}, nil
	}
	svc := service.NewBatchService(repo, 5, mockExtractor)
	ctx := context.Background()

	res, err := svc.ExtractBatchChapters(ctx, 10, 1, 5)
	require.NoError(t, err)

	assert.Equal(t, int64(10), res.CanonicalManhuaID)
	assert.Equal(t, "overgeared", res.Slug)
	assert.Equal(t, 1, res.Start)
	assert.Equal(t, 5, res.End)
	assert.Equal(t, 5, res.TotalChapters)
	assert.Len(t, res.Chapters, 5)

	// Ensure sequential order and success
	for i, ch := range res.Chapters {
		expectedCh := i + 1
		assert.Equal(t, expectedCh, ch.Chapter)
		assert.True(t, ch.Success)
		assert.NotEmpty(t, ch.Images)
	}
}

func TestBatchService_PartialFailureTolerance(t *testing.T) {
	lastCh := 20
	manhua := &domain.CanonicalManhua{
		ID:          10,
		Slug:        "overgeared",
		Title:       "Overgeared",
		LastChapter: &lastCh,
	}
	repo := &mockCanonicalRepoForBatch{manhua: manhua}

	// Custom extractor that fails on chapter 3
	customExtractor := func(ctx context.Context, m *domain.CanonicalManhua, ch int) ([]domain.BatchChapterImage, error) {
		if ch == 3 {
			return nil, errors.New("network timeout extracting chapter 3")
		}
		return []domain.BatchChapterImage{
			{Index: 1, URL: "https://example.com/panel1.jpg", ProxyURL: "/api/reader/proxy-image?url=..."},
		}, nil
	}

	svc := service.NewBatchService(repo, 5, customExtractor)
	ctx := context.Background()

	res, err := svc.ExtractBatchChapters(ctx, 10, 1, 4)
	require.NoError(t, err)

	require.Len(t, res.Chapters, 4)
	assert.True(t, res.Chapters[0].Success) // Ch 1
	assert.True(t, res.Chapters[1].Success) // Ch 2
	assert.False(t, res.Chapters[2].Success) // Ch 3 failed
	assert.Equal(t, "network timeout extracting chapter 3", res.Chapters[2].Error)
	assert.Empty(t, res.Chapters[2].Images)
	assert.True(t, res.Chapters[3].Success) // Ch 4
}
