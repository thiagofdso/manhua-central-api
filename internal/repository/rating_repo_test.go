package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/migrations"
)

func TestPostgresRatingRepository(t *testing.T) {
	connStr, cleanup := setupTestPostgres(t)
	defer cleanup()

	pgDB, err := repository.NewPostgresDB(connStr)
	require.NoError(t, err)
	defer pgDB.Close()

	err = pgDB.RunMigrationsFS(migrations.FS, ".")
	require.NoError(t, err)

	ctx := context.Background()
	userRepo := repository.NewPostgresUserRepository(pgDB.DB)
	canonicalRepo := repository.NewPostgresCanonicalRepository(pgDB.DB)
	ratingRepo := repository.NewPostgresRatingRepository(pgDB.DB, nil)

	// Create 2 users
	user1 := &domain.User{
		Email:        fmt.Sprintf("rate_u1_%d@example.com", time.Now().UnixNano()),
		DisplayName:  "Rater 1",
		AuthProvider: domain.ProviderEmail,
		Role:         domain.RoleReader,
	}
	require.NoError(t, userRepo.CreateUser(ctx, user1))

	user2 := &domain.User{
		Email:        fmt.Sprintf("rate_u2_%d@example.com", time.Now().UnixNano()),
		DisplayName:  "Rater 2",
		AuthProvider: domain.ProviderEmail,
		Role:         domain.RoleReader,
	}
	require.NoError(t, userRepo.CreateUser(ctx, user2))

	// Create a canonical manhua
	mID := time.Now().UnixNano() / 1e6
	m := &domain.CanonicalManhua{
		ID:            mID,
		Slug:          fmt.Sprintf("rating-manhua-%d", mID),
		Title:         "Rating Manhua",
		PrimaryDomain: "asurascans.com",
		URLTemplate:   "https://asurascans.com/comics/1/{chapter}",
	}
	require.NoError(t, canonicalRepo.Create(ctx, m))

	t.Run("Reject invalid scores outside 1-5 scale", func(t *testing.T) {
		_, err := ratingRepo.SubmitVote(ctx, user1.ID, m.ID, 0)
		assert.ErrorIs(t, err, repository.ErrInvalidScore)

		_, err = ratingRepo.SubmitVote(ctx, user1.ID, m.ID, 6)
		assert.ErrorIs(t, err, repository.ErrInvalidScore)

		_, err = ratingRepo.SubmitVote(ctx, user1.ID, m.ID, -1)
		assert.ErrorIs(t, err, repository.ErrInvalidScore)
	})

	t.Run("Submit valid 5-star vote and calculate initial Bayesian rating", func(t *testing.T) {
		updated, err := ratingRepo.SubmitVote(ctx, user1.ID, m.ID, 5)
		require.NoError(t, err)
		assert.Equal(t, 1, updated.TotalVotes)
		// WR = (1/11)*5 + (10/11)*3.5 = 40/11 = 3.64
		assert.InDelta(t, 3.64, updated.BayesianRating, 0.01)

		// Check GetUserVote for user1
		vote, err := ratingRepo.GetUserVote(ctx, user1.ID, m.ID)
		require.NoError(t, err)
		require.NotNil(t, vote)
		assert.Equal(t, 5, *vote)

		// User 2 hasn't voted yet
		vote2, err := ratingRepo.GetUserVote(ctx, user2.ID, m.ID)
		require.NoError(t, err)
		assert.Nil(t, vote2)
	})

	t.Run("Second user submits 4-star vote", func(t *testing.T) {
		updated, err := ratingRepo.SubmitVote(ctx, user2.ID, m.ID, 4)
		require.NoError(t, err)
		assert.Equal(t, 2, updated.TotalVotes)
		// Average = 4.5. WR = (2/12)*4.5 + (10/12)*3.5 = 44/12 = 3.67
		assert.InDelta(t, 3.67, updated.BayesianRating, 0.01)
	})

	t.Run("User 1 changes vote to 1 star (upsert test)", func(t *testing.T) {
		updated, err := ratingRepo.SubmitVote(ctx, user1.ID, m.ID, 1)
		require.NoError(t, err)
		// Total votes should NOT increase
		assert.Equal(t, 2, updated.TotalVotes)
		// Average = (1 + 4)/2 = 2.5. WR = (2/12)*2.5 + (10/12)*3.5 = 40/12 = 3.33
		assert.InDelta(t, 3.33, updated.BayesianRating, 0.01)

		vote, err := ratingRepo.GetUserVote(ctx, user1.ID, m.ID)
		require.NoError(t, err)
		require.NotNil(t, vote)
		assert.Equal(t, 1, *vote)
	})

	t.Run("SyncViewsToPostgres updates total_views", func(t *testing.T) {
		viewsMap := map[int64]int64{
			m.ID: 1500,
		}
		err := ratingRepo.SyncViewsToPostgres(ctx, viewsMap)
		require.NoError(t, err)

		fetched, err := canonicalRepo.GetByID(ctx, m.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(1500), fetched.TotalViews)
	})

	t.Run("CanonicalRepo GetByIDs preserves input order", func(t *testing.T) {
		m2ID := mID + 10
		m2 := &domain.CanonicalManhua{
			ID:            m2ID,
			Slug:          fmt.Sprintf("manhua-order-%d", m2ID),
			Title:         "Manhua Order",
			PrimaryDomain: "ravenscans.org",
			URLTemplate:   "https://ravenscans.org/manga/{chapter}",
		}
		require.NoError(t, canonicalRepo.Create(ctx, m2))

		// Request in reverse order [m2.ID, m.ID]
		list, err := canonicalRepo.GetByIDs(ctx, []int64{m2.ID, m.ID})
		require.NoError(t, err)
		require.Len(t, list, 2)
		assert.Equal(t, m2.ID, list[0].ID)
		assert.Equal(t, m.ID, list[1].ID)
	})
}
