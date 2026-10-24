package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/migrations"
)

func TestPostgresLibraryRepository(t *testing.T) {
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
	libraryRepo := repository.NewPostgresLibraryRepository(pgDB.DB)

	// Create test user
	user := &domain.User{
		Email:        fmt.Sprintf("lib_test_%d@example.com", time.Now().UnixNano()),
		DisplayName:  "Library Tester",
		AuthProvider: domain.ProviderEmail,
		Role:         domain.RoleReader,
	}
	err = userRepo.CreateUser(ctx, user)
	require.NoError(t, err)

	// Create 2 canonical manhuas
	mID1 := time.Now().UnixNano() / 1e6
	m1 := &domain.CanonicalManhua{
		ID:            mID1,
		Slug:          fmt.Sprintf("manhua-1-%d", mID1),
		Title:         "Manhua 1",
		PrimaryDomain: "asurascans.com",
		URLTemplate:   "https://asurascans.com/comics/1/{chapter}",
	}
	err = canonicalRepo.Create(ctx, m1)
	require.NoError(t, err)

	mID2 := mID1 + 1
	m2 := &domain.CanonicalManhua{
		ID:            mID2,
		Slug:          fmt.Sprintf("manhua-2-%d", mID2),
		Title:         "Manhua 2",
		PrimaryDomain: "ravenscans.org",
		URLTemplate:   "https://ravenscans.org/2-chapter-{chapter}/",
	}
	err = canonicalRepo.Create(ctx, m2)
	require.NoError(t, err)

	// 1. Create UserLibrary items
	item1 := &domain.UserLibrary{
		ID:                uuid.New().String(),
		UserID:            user.ID,
		CanonicalManhuaID: m1.ID,
		CurrentChapter:    5,
		IsFavorite:        false,
		CustomOrder:       2,
		UpdatedAt:         time.Now().Unix(),
	}
	err = libraryRepo.Create(ctx, item1)
	require.NoError(t, err)

	item2 := &domain.UserLibrary{
		ID:                uuid.New().String(),
		UserID:            user.ID,
		CanonicalManhuaID: m2.ID,
		CurrentChapter:    12,
		IsFavorite:        true,
		CustomOrder:       1,
		UpdatedAt:         time.Now().Unix(),
	}
	err = libraryRepo.Create(ctx, item2)
	require.NoError(t, err)

	// 2. GetByID (with joined CanonicalManhua)
	fetched, err := libraryRepo.GetByID(ctx, user.ID, item1.ID)
	require.NoError(t, err)
	assert.Equal(t, item1.ID, fetched.ID)
	assert.Equal(t, 5, fetched.CurrentChapter)
	require.NotNil(t, fetched.CanonicalManhua)
	assert.Equal(t, "Manhua 1", fetched.CanonicalManhua.Title)

	// 3. GetByUserAndManhuaID
	byManhua, err := libraryRepo.GetByUserAndManhuaID(ctx, user.ID, m2.ID)
	require.NoError(t, err)
	assert.Equal(t, item2.ID, byManhua.ID)
	assert.True(t, byManhua.IsFavorite)

	// 4. ListByUser (Golden Rule: item2 favorite must come first!)
	list, err := libraryRepo.ListByUser(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.True(t, list[0].IsFavorite)
	assert.Equal(t, item2.ID, list[0].ID)
	assert.False(t, list[1].IsFavorite)
	assert.Equal(t, item1.ID, list[1].ID)

	// 5. Update
	item1.CurrentChapter = 6
	item1.CustomOrder = -1
	err = libraryRepo.Update(ctx, item1)
	require.NoError(t, err)

	updated, err := libraryRepo.GetByID(ctx, user.ID, item1.ID)
	require.NoError(t, err)
	assert.Equal(t, 6, updated.CurrentChapter)
	assert.Equal(t, -1, updated.CustomOrder)

	// 6. UpdateOrderBatch
	item1.CustomOrder = 1
	item2.CustomOrder = 0
	err = libraryRepo.UpdateOrderBatch(ctx, user.ID, []*domain.UserLibrary{item2, item1})
	require.NoError(t, err)

	// 7. GetMinCustomOrder
	minFav, err := libraryRepo.GetMinCustomOrder(ctx, user.ID, true)
	require.NoError(t, err)
	assert.Equal(t, 0, minFav)

	minNonFav, err := libraryRepo.GetMinCustomOrder(ctx, user.ID, false)
	require.NoError(t, err)
	assert.Equal(t, 1, minNonFav)

	// 8. Delete
	err = libraryRepo.Delete(ctx, user.ID, item1.ID)
	require.NoError(t, err)

	_, err = libraryRepo.GetByID(ctx, user.ID, item1.ID)
	assert.ErrorIs(t, err, repository.ErrLibraryItemNotFound)
}
