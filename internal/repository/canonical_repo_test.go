package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/migrations"
)

func TestPostgresCanonicalRepository(t *testing.T) {
	connStr, cleanup := setupTestPostgres(t)
	defer cleanup()

	pgDB, err := repository.NewPostgresDB(connStr)
	require.NoError(t, err)
	defer pgDB.Close()

	err = pgDB.RunMigrationsFS(migrations.FS, ".")
	require.NoError(t, err)

	ctx := context.Background()
	repo := repository.NewPostgresCanonicalRepository(pgDB.DB)

	// Clean up database tables for clean test state
	_, _ = pgDB.DB.Exec("DELETE FROM user_libraries")
	_, _ = pgDB.DB.Exec("DELETE FROM canonical_manhuas")

	testSuffix := time.Now().UnixNano()
	slug1 := "test-solo-leveling-ragnarok"
	slug2 := "test-almighty-pig-slaughtering-knife"

	synopsis := "A legendary hunter returns."
	altTitles := "SL: Ragnarok, Solo Leveling 2"
	cover := "https://cdn.asurascans.com/covers/sl-rag.webp"
	lastCh := 50
	var updateDay int16 = 3

	m1 := &domain.CanonicalManhua{
		ID:                testSuffix / 1e6,
		Slug:              slug1,
		Title:             "Solo Leveling: Ragnarok",
		AlternativeTitles: &altTitles,
		Synopsis:          &synopsis,
		CoverURL:          &cover,
		PrimaryDomain:     "asurascans.com",
		URLTemplate:       "https://asurascans.com/comics/solo-leveling-ragnarok/chapter/{chapter}",
		LastChapter:       &lastCh,
		UpdateDay:         &updateDay,
		IsHiatus:          false,
		BayesianRating:    9.45,
		TotalVotes:        1200,
		TotalViews:        50000,
	}

	// 1. Create
	err = repo.Create(ctx, m1)
	require.NoError(t, err)
	assert.False(t, m1.CreatedAt.IsZero())

	// 2. GetByID
	fetched, err := repo.GetByID(ctx, m1.ID)
	require.NoError(t, err)
	assert.Equal(t, m1.Slug, fetched.Slug)
	assert.Equal(t, m1.Title, fetched.Title)
	assert.Equal(t, "asurascans.com", fetched.PrimaryDomain)
	assert.Equal(t, 9.45, fetched.BayesianRating)

	// 3. GetBySlug
	bySlug, err := repo.GetBySlug(ctx, slug1)
	require.NoError(t, err)
	assert.Equal(t, m1.ID, bySlug.ID)

	// 4. Update
	newCh := 51
	m1.LastChapter = &newCh
	m1.TotalViews = 51000
	err = repo.Update(ctx, m1)
	require.NoError(t, err)

	updated, err := repo.GetByID(ctx, m1.ID)
	require.NoError(t, err)
	assert.Equal(t, 51, *updated.LastChapter)
	assert.Equal(t, int64(51000), updated.TotalViews)

	// 5. Insert second manhua for list & search tests
	m2 := &domain.CanonicalManhua{
		ID:            (testSuffix / 1e6) + 1,
		Slug:          slug2,
		Title:         "Almighty Pig Slaughtering Knife",
		PrimaryDomain: "ravenscans.org",
		URLTemplate:   "https://ravenscans.org/knife-chapter-{chapter}/",
		IsHiatus:      false,
		TotalViews:    10000,
	}
	err = repo.Create(ctx, m2)
	require.NoError(t, err)

	// 6. List with Query filter
	res, err := repo.List(ctx, domain.CatalogFilter{
		Query:    "slaughtering",
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Total)
	require.Len(t, res.Items, 1)
	assert.Equal(t, "Almighty Pig Slaughtering Knife", res.Items[0].Title)

	// 7. List with Domain filter
	resDomain, err := repo.List(ctx, domain.CatalogFilter{
		Query:    "test-solo",
		Domain:   "asurascans.com",
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, resDomain.Total)
	assert.Equal(t, "Solo Leveling: Ragnarok", resDomain.Items[0].Title)

	// 8. UpsertBatch
	newBatchItem := &domain.CanonicalManhua{
		ID:            (testSuffix / 1e6) + 2,
		Slug:          slug1, // conflict on slug -> should update
		Title:         "Solo Leveling: Ragnarok (Updated)",
		PrimaryDomain: "asurascans.com",
		URLTemplate:   "https://asurascans.com/comics/solo-leveling-ragnarok/chapter/{chapter}",
	}
	err = repo.UpsertBatch(ctx, []*domain.CanonicalManhua{newBatchItem})
	require.NoError(t, err)

	afterBatch, err := repo.GetBySlug(ctx, slug1)
	require.NoError(t, err)
	assert.Equal(t, "Solo Leveling: Ragnarok (Updated)", afterBatch.Title)
}
