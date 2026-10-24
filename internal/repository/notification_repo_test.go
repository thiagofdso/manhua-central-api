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

func TestPostgresNotificationRepository(t *testing.T) {
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
	notifRepo := repository.NewPostgresNotificationRepository(pgDB.DB)

	// Create user
	user := &domain.User{
		Email:        fmt.Sprintf("notif_u_%d@example.com", time.Now().UnixNano()),
		DisplayName:  "Notification User",
		AuthProvider: domain.ProviderEmail,
		Role:         domain.RoleReader,
	}
	require.NoError(t, userRepo.CreateUser(ctx, user))

	// Create canonical manhua
	mID := time.Now().UnixNano() / 1e6
	cover := "https://example.com/cover.jpg"
	manhua := &domain.CanonicalManhua{
		ID:            mID,
		Slug:          fmt.Sprintf("solo-test-%d", mID),
		Title:         "Solo Leveling Test",
		CoverURL:      &cover,
		PrimaryDomain: "asurascans.com",
		URLTemplate:   "https://asurascans.com/series/solo-leveling/chapter-{chapter}",
	}
	require.NoError(t, canonicalRepo.Create(ctx, manhua))

	// Batch insert notifications
	notifs := []*domain.Notification{
		{
			ID:                uuid.New().String(),
			UserID:            user.ID,
			CanonicalManhuaID: manhua.ID,
			Chapter:           101,
			Title:             "Novo Capítulo Disponível!",
			Body:              "Solo Leveling Test Cap. 101 já pode ser lido.",
			IsRead:            false,
			CreatedAt:         time.Now(),
		},
		{
			ID:                uuid.New().String(),
			UserID:            user.ID,
			CanonicalManhuaID: manhua.ID,
			Chapter:           102,
			Title:             "Novo Capítulo Disponível!",
			Body:              "Solo Leveling Test Cap. 102 já pode ser lido.",
			IsRead:            false,
			CreatedAt:         time.Now().Add(time.Minute),
		},
	}

	err = notifRepo.CreateBatch(ctx, notifs)
	require.NoError(t, err)

	// List notifications
	items, err := notifRepo.ListByUser(ctx, user.ID)
	require.NoError(t, err)
	assert.Len(t, items, 2)
	assert.Equal(t, 102, items[0].Chapter) // Sorted DESC by created_at
	assert.Equal(t, manhua.Slug, items[0].ManhuaSlug)
	assert.Equal(t, &cover, items[0].ManhuaCoverURL)

	// Check unread count
	unread, err := notifRepo.GetUnreadCount(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, unread)

	// Mark first as read
	err = notifRepo.MarkAsRead(ctx, items[0].ID, user.ID)
	require.NoError(t, err)

	unread, err = notifRepo.GetUnreadCount(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, unread)

	// Dismiss second
	err = notifRepo.Dismiss(ctx, items[1].ID, user.ID)
	require.NoError(t, err)

	itemsAfter, err := notifRepo.ListByUser(ctx, user.ID)
	require.NoError(t, err)
	assert.Len(t, itemsAfter, 1)
}
