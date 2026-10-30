package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/service"
)

type mockRatingRepo struct {
	submitVoteFunc func(ctx context.Context, userID string, manhuaID int64, score int) (*domain.CanonicalManhua, error)
	getUserVoteFunc func(ctx context.Context, userID string, manhuaID int64) (*int, error)
	syncViewsFunc   func(ctx context.Context, views map[int64]int64) error
}

func (m *mockRatingRepo) SubmitVote(ctx context.Context, userID string, manhuaID int64, score int) (*domain.CanonicalManhua, error) {
	if m.submitVoteFunc != nil {
		return m.submitVoteFunc(ctx, userID, manhuaID, score)
	}
	return nil, nil
}

func (m *mockRatingRepo) GetUserVote(ctx context.Context, userID string, manhuaID int64) (*int, error) {
	if m.getUserVoteFunc != nil {
		return m.getUserVoteFunc(ctx, userID, manhuaID)
	}
	return nil, nil
}

func (m *mockRatingRepo) SyncViewsToPostgres(ctx context.Context, views map[int64]int64) error {
	if m.syncViewsFunc != nil {
		return m.syncViewsFunc(ctx, views)
	}
	return nil
}

type mockCanonicalRepoForRanking struct {
	getByIDsFunc func(ctx context.Context, ids []int64) ([]*domain.CanonicalManhua, error)
	listFunc     func(ctx context.Context, filter domain.CatalogFilter) (*domain.CatalogSearchResult, error)
	getByIDFunc  func(ctx context.Context, id int64) (*domain.CanonicalManhua, error)
	updateFunc   func(ctx context.Context, m *domain.CanonicalManhua) error
}

func (m *mockCanonicalRepoForRanking) Create(ctx context.Context, manhua *domain.CanonicalManhua) error { return nil }
func (m *mockCanonicalRepoForRanking) GetByID(ctx context.Context, id int64) (*domain.CanonicalManhua, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, nil
}
func (m *mockCanonicalRepoForRanking) GetBySlug(ctx context.Context, slug string) (*domain.CanonicalManhua, error) { return nil, nil }
func (m *mockCanonicalRepoForRanking) GetByIDs(ctx context.Context, ids []int64) ([]*domain.CanonicalManhua, error) {
	if m.getByIDsFunc != nil {
		return m.getByIDsFunc(ctx, ids)
	}
	return nil, nil
}
func (m *mockCanonicalRepoForRanking) Update(ctx context.Context, manhua *domain.CanonicalManhua) error {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, manhua)
	}
	return nil
}
func (m *mockCanonicalRepoForRanking) List(ctx context.Context, filter domain.CatalogFilter) (*domain.CatalogSearchResult, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, filter)
	}
	return &domain.CatalogSearchResult{}, nil
}
func (m *mockCanonicalRepoForRanking) UpsertBatch(ctx context.Context, items []*domain.CanonicalManhua) error { return nil }
func (m *mockCanonicalRepoForRanking) ListDomains(ctx context.Context) ([]string, error) { return nil, nil }

func TestRankingService(t *testing.T) {
	ctx := context.Background()

	t.Run("SubmitRating delegates to repository and returns updated manhua", func(t *testing.T) {
		expectedManhua := &domain.CanonicalManhua{
			ID:             100,
			Title:          "Solo Leveling",
			BayesianRating: 4.85,
			TotalVotes:     50,
		}

		mockRepo := &mockRatingRepo{
			submitVoteFunc: func(ctx context.Context, userID string, manhuaID int64, score int) (*domain.CanonicalManhua, error) {
				assert.Equal(t, "user-123", userID)
				assert.Equal(t, int64(100), manhuaID)
				assert.Equal(t, 5, score)
				return expectedManhua, nil
			},
		}

		svc := service.NewRankingService(mockRepo, &mockCanonicalRepoForRanking{}, nil)
		res, err := svc.SubmitRating(ctx, "user-123", 100, 5)
		require.NoError(t, err)
		assert.Equal(t, expectedManhua, res)
	})

	t.Run("GetUserRating returns existing score", func(t *testing.T) {
		score := 4
		mockRepo := &mockRatingRepo{
			getUserVoteFunc: func(ctx context.Context, userID string, manhuaID int64) (*int, error) {
				return &score, nil
			},
		}

		svc := service.NewRankingService(mockRepo, &mockCanonicalRepoForRanking{}, nil)
		res, err := svc.GetUserRating(ctx, "user-123", 100)
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.Equal(t, 4, *res)
	})

	t.Run("GetRankings falls back to Postgres when Redis is nil", func(t *testing.T) {
		mockCanonical := &mockCanonicalRepoForRanking{
			listFunc: func(ctx context.Context, filter domain.CatalogFilter) (*domain.CatalogSearchResult, error) {
				assert.Equal(t, "rating", filter.SortBy)
				return &domain.CatalogSearchResult{
					Items: []*domain.CanonicalManhua{
						{ID: 1, Title: "Top 1", BayesianRating: 4.90, TotalVotes: 100},
						{ID: 2, Title: "Top 2", BayesianRating: 4.80, TotalVotes: 80},
					},
					Total: 2,
				}, nil
			},
		}

		svc := service.NewRankingService(&mockRatingRepo{}, mockCanonical, nil)
		rankings, err := svc.GetRankings(ctx, "top_rated", 10)
		require.NoError(t, err)
		require.Len(t, rankings, 2)
		assert.Equal(t, 1, rankings[0].Rank)
		assert.Equal(t, 4.90, rankings[0].Score)
		assert.Equal(t, "Top 1", rankings[0].Canonical.Title)
		assert.Equal(t, 2, rankings[1].Rank)
		assert.Equal(t, 4.80, rankings[1].Score)
		assert.Equal(t, "Top 2", rankings[1].Canonical.Title)
	})
}
