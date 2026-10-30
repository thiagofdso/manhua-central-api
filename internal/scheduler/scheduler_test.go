package scheduler_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/scheduler"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/migrations"
)

func setupTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	ctx := context.Background()
	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("manhua_scheduler_test"),
		tcpostgres.WithUsername("test_user"),
		tcpostgres.WithPassword("test_pass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	require.NoError(t, err)

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pgDB, err := repository.NewPostgresDB(connStr)
	require.NoError(t, err)

	err = pgDB.RunMigrationsFS(migrations.FS, ".")
	require.NoError(t, err)

	return pgDB.DB, func() {
		pgDB.Close()
		pgContainer.Terminate(ctx)
	}
}

func TestScheduler_RunRoutineCheck_WithAdvisoryLock(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	userRepo := repository.NewPostgresUserRepository(db)
	canonicalRepo := repository.NewPostgresCanonicalRepository(db)
	libraryRepo := repository.NewPostgresLibraryRepository(db)
	notifRepo := repository.NewPostgresNotificationRepository(db)

	// Create user
	user := &domain.User{
		Email:        fmt.Sprintf("sched_u_%d@example.com", time.Now().UnixNano()),
		DisplayName:  "Scheduler Reader",
		AuthProvider: domain.ProviderEmail,
		Role:         domain.RoleReader,
	}
	require.NoError(t, userRepo.CreateUser(ctx, user))

	// Create canonical manhua with last_chapter = 50
	lastCh := 50
	mID := time.Now().UnixNano() / 1e6
	manhua := &domain.CanonicalManhua{
		ID:            mID,
		Slug:          fmt.Sprintf("sched-slug-%d", mID),
		Title:         "Omniscient Reader Test",
		LastChapter:   &lastCh,
		PrimaryDomain: "asurascans.com",
		URLTemplate:   "https://asurascans.com/series/orv/chapter-{chapter}",
	}
	require.NoError(t, canonicalRepo.Create(ctx, manhua))

	// User adds manhua to library
	err := libraryRepo.Create(ctx, &domain.UserLibrary{
		UserID:            user.ID,
		CanonicalManhuaID: manhua.ID,
		CurrentChapter:    49,
		IsFavorite:        true,
	})
	require.NoError(t, err)

	// Create scheduler with chapter checker returning 51 (new release)
	mockChecker := func(ctx context.Context, m *domain.CanonicalManhua) (int, error) {
		return 51, nil
	}

	sched := scheduler.NewScheduler(db, canonicalRepo, notifRepo, mockChecker)

	// Run routine check
	res, err := sched.RunRoutineCheck(ctx)
	require.NoError(t, err)
	assert.True(t, res.LockAcquired)
	assert.Equal(t, 1, res.UpdatedManhuas)
	assert.Equal(t, 1, res.NotificationsSent)

	// Verify manhua last_chapter updated to 51
	updatedManhua, err := canonicalRepo.GetByID(ctx, manhua.ID)
	require.NoError(t, err)
	assert.Equal(t, 51, *updatedManhua.LastChapter)

	// Verify user received notification
	notifs, err := notifRepo.ListByUser(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, notifs, 1)
	assert.Equal(t, 51, notifs[0].Chapter)
	assert.Equal(t, manhua.ID, notifs[0].CanonicalManhuaID)
	assert.Contains(t, notifs[0].Title, "Omniscient Reader Test")
}
