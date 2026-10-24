package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
)

type RankingService struct {
	ratingRepo    repository.RatingRepository
	canonicalRepo repository.CanonicalRepository
	redisClient   *repository.RedisClient
}

func NewRankingService(
	ratingRepo repository.RatingRepository,
	canonicalRepo repository.CanonicalRepository,
	redisClient *repository.RedisClient,
) *RankingService {
	return &RankingService{
		ratingRepo:    ratingRepo,
		canonicalRepo: canonicalRepo,
		redisClient:   redisClient,
	}
}

// SubmitRating records a user's 1-5 star review and recalculates Bayesian rating.
func (s *RankingService) SubmitRating(ctx context.Context, userID string, manhuaID int64, score int) (*domain.CanonicalManhua, error) {
	return s.ratingRepo.SubmitVote(ctx, userID, manhuaID, score)
}

// GetUserRating returns the score previously submitted by the user, or nil if none.
func (s *RankingService) GetUserRating(ctx context.Context, userID string, manhuaID int64) (*int, error) {
	return s.ratingRepo.GetUserVote(ctx, userID, manhuaID)
}

// RecordView tracks an access with 15-minute rate-limiting per client identifier (IP or User ID).
func (s *RankingService) RecordView(ctx context.Context, manhuaID int64, identifier string) error {
	if s.redisClient != nil {
		counted, err := s.redisClient.RecordViewRateLimited(ctx, manhuaID, identifier)
		if err != nil {
			log.Printf("[WARN] Failed to record view in redis for manhua %d: %v", manhuaID, err)
		}
		if counted {
			// Also increment directly in Postgres for immediate persistence
			m, err := s.canonicalRepo.GetByID(ctx, manhuaID)
			if err == nil && m != nil {
				m.TotalViews++
				_ = s.canonicalRepo.Update(ctx, m)
			}
		}
		return nil
	}

	// Fallback without Redis: increment Postgres views directly
	m, err := s.canonicalRepo.GetByID(ctx, manhuaID)
	if err != nil {
		return err
	}
	m.TotalViews++
	return s.canonicalRepo.Update(ctx, m)
}

// GetRankings executes the two-stage hybrid lookup (Redis ZSET -> Postgres hydration) with automatic Postgres fallback.
func (s *RankingService) GetRankings(ctx context.Context, rankingType string, limit int) ([]*domain.RankedManhua, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	if rankingType == "" {
		rankingType = "top_rated"
	}

	var redisKey string
	now := time.Now().UTC()

	switch rankingType {
	case "daily":
		redisKey = fmt.Sprintf("rankings:views:daily:%s", now.Format("2006-01-02"))
	case "weekly":
		year, week := now.ISOWeek()
		redisKey = fmt.Sprintf("rankings:views:weekly:%d-%02d", year, week)
	case "all_time":
		redisKey = "rankings:views:all_time"
	case "top_rated":
		redisKey = "rankings:top_rated"
	default:
		redisKey = "rankings:top_rated"
	}

	// Stage 1: Attempt Redis ZSET lookup
	if s.redisClient != nil {
		ids, scores, err := s.redisClient.GetTopRanked(ctx, redisKey, int64(limit))
		if err == nil && len(ids) > 0 {
			// Stage 2: Hydrate full records from PostgreSQL preserving Redis ordering
			manhuas, err := s.canonicalRepo.GetByIDs(ctx, ids)
			if err == nil && len(manhuas) > 0 {
				manhuaMap := make(map[int64]*domain.CanonicalManhua, len(manhuas))
				for _, m := range manhuas {
					manhuaMap[m.ID] = m
				}

				rankedList := make([]*domain.RankedManhua, 0, len(ids))
				for i, id := range ids {
					if m, found := manhuaMap[id]; found {
						score := 0.0
						if i < len(scores) {
							score = scores[i]
						}
						rankedList = append(rankedList, &domain.RankedManhua{
							Rank:      i + 1,
							Score:     score,
							Canonical: m,
						})
					}
				}
				if len(rankedList) > 0 {
					return rankedList, nil
				}
			}
		}
	}

	// Fallback to PostgreSQL catalog query
	sortBy := "rating"
	if rankingType == "daily" || rankingType == "weekly" || rankingType == "all_time" {
		sortBy = "views"
	}

	searchRes, err := s.canonicalRepo.List(ctx, domain.CatalogFilter{
		SortBy:   sortBy,
		Page:     1,
		PageSize: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ranking fallback from database: %w", err)
	}

	rankedList := make([]*domain.RankedManhua, 0, len(searchRes.Items))
	for i, item := range searchRes.Items {
		score := item.BayesianRating
		if sortBy == "views" {
			score = float64(item.TotalViews)
		}
		rankedList = append(rankedList, &domain.RankedManhua{
			Rank:      i + 1,
			Score:     score,
			Canonical: item,
		})
	}

	return rankedList, nil
}

// WarmUp populates Redis sorted sets from Postgres on application startup.
func (s *RankingService) WarmUp(ctx context.Context) error {
	if s.redisClient == nil {
		return nil
	}

	// 1. Warm up top_rated
	ratedRes, err := s.canonicalRepo.List(ctx, domain.CatalogFilter{
		SortBy:   "rating",
		Page:     1,
		PageSize: 100,
	})
	if err == nil {
		for _, m := range ratedRes.Items {
			_ = s.redisClient.SetBayesianRating(ctx, m.ID, m.BayesianRating)
		}
	}

	// 2. Warm up all_time views
	viewsRes, err := s.canonicalRepo.List(ctx, domain.CatalogFilter{
		SortBy:   "views",
		Page:     1,
		PageSize: 100,
	})
	if err == nil {
		for _, m := range viewsRes.Items {
			_ = s.redisClient.IncrementViews(ctx, "all_time", m.ID, float64(m.TotalViews))
		}
	}

	log.Println("[INFO] Ranking Redis cache warmed up successfully.")
	return nil
}

// StartBackgroundSync periodically reconciles accumulated Redis views back into PostgreSQL.
func (s *RankingService) StartBackgroundSync(ctx context.Context, interval time.Duration) {
	if s.redisClient == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				views, err := s.redisClient.GetAllViews(ctx)
				if err != nil {
					log.Printf("[WARN] Failed to fetch views for background sync: %v", err)
					continue
				}
				if len(views) > 0 {
					if err := s.ratingRepo.SyncViewsToPostgres(ctx, views); err != nil {
						log.Printf("[WARN] Failed to sync views to postgres: %v", err)
					}
				}
			}
		}
	}()
}
