package service

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
)

const (
	MaxBatchSize       = 50
	DefaultConcurrency = 5
)

var (
	ErrInvalidBatchRange          = errors.New("invalid batch range: start must be >= 1 and end >= start")
	ErrBatchSizeExceeded          = errors.New("batch range exceeds maximum allowed limit of 50 chapters (RN-05.1)")
	ErrExceedsLastChapterCeiling  = errors.New("requested chapter range exceeds the last available chapter ceiling (RN-05.1)")
	ErrCanonicalManhuaNotFound    = errors.New("canonical manhua not found")
)

type ChapterExtractorFunc func(ctx context.Context, m *domain.CanonicalManhua, chapter int) ([]domain.BatchChapterImage, error)

type BatchService struct {
	canonicalRepo  repository.CanonicalRepository
	maxConcurrency int
	extractor      ChapterExtractorFunc
}

func NewBatchService(
	canonicalRepo repository.CanonicalRepository,
	maxConcurrency int,
	extractor ChapterExtractorFunc,
) *BatchService {
	if maxConcurrency <= 0 {
		maxConcurrency = DefaultConcurrency
	}
	if extractor == nil {
		extractor = defaultChapterExtractor
	}
	return &BatchService{
		canonicalRepo:  canonicalRepo,
		maxConcurrency: maxConcurrency,
		extractor:      extractor,
	}
}

// ExtractBatchChapters validates range and ceiling, then concurrently extracts chapter images with fault tolerance.
func (s *BatchService) ExtractBatchChapters(
	ctx context.Context,
	manhuaID int64,
	start int,
	end int,
) (*domain.BatchChaptersResponse, error) {
	// 1. Validation of range
	if start < 1 || end < start {
		return nil, ErrInvalidBatchRange
	}

	totalChapters := end - start + 1
	if totalChapters > MaxBatchSize {
		return nil, ErrBatchSizeExceeded
	}

	// 2. Fetch canonical manhua
	manhua, err := s.canonicalRepo.GetByID(ctx, manhuaID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCanonicalManhuaNotFound, err)
	}

	// 3. Ceiling enforcement by lastChapter (RN-05.1)
	if manhua.LastChapter != nil {
		if start > *manhua.LastChapter || end > *manhua.LastChapter {
			return nil, fmt.Errorf("%w: requested end %d exceeds last chapter %d", ErrExceedsLastChapterCeiling, end, *manhua.LastChapter)
		}
	}

	// 4. Concurrently extract chapters with bounded semaphore
	sem := make(chan struct{}, s.maxConcurrency)
	results := make([]domain.BatchChapterResult, totalChapters)
	var wg sync.WaitGroup

	for i := 0; i < totalChapters; i++ {
		chapterNum := start + i
		idx := i

		wg.Add(1)
		go func(ch int, resIdx int) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			images, extractErr := s.extractor(ctx, manhua, ch)
			if extractErr != nil {
				// RN-05.2: Partial failure tolerance - record error without aborting batch
				results[resIdx] = domain.BatchChapterResult{
					Chapter: ch,
					Success: false,
					Images:  []domain.BatchChapterImage{},
					Error:   extractErr.Error(),
				}
				return
			}

			results[resIdx] = domain.BatchChapterResult{
				Chapter: ch,
				Success: true,
				Images:  images,
			}
		}(chapterNum, idx)
	}

	wg.Wait()

	return &domain.BatchChaptersResponse{
		CanonicalManhuaID: manhua.ID,
		Slug:              manhua.Slug,
		Title:             manhua.Title,
		Start:             start,
		End:               end,
		TotalChapters:     totalChapters,
		Chapters:          results,
	}, nil
}

// defaultChapterExtractor returns an error when no custom extractor is configured.
func defaultChapterExtractor(ctx context.Context, m *domain.CanonicalManhua, chapter int) ([]domain.BatchChapterImage, error) {
	return nil, fmt.Errorf("no chapter extractor configured for %s", m.Slug)
}
