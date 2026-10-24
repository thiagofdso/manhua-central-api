package repository_test

import (
	"context"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/migrations"
)

func setupTestPostgres(t *testing.T) (string, func()) {
	t.Helper()

	// If DATABASE_URL is set explicitly in test environment, use it
	if envURL := os.Getenv("TEST_DATABASE_URL"); envURL != "" {
		return envURL, func() {}
	}

	ctx := context.Background()
	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("manhua_pro_test"),
		tcpostgres.WithUsername("test_user"),
		tcpostgres.WithPassword("test_pass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	require.NoError(t, err, "failed to start testcontainers postgres")

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "failed to get connection string")

	cleanup := func() {
		_ = pgContainer.Terminate(ctx)
	}

	return connStr, cleanup
}

func TestPostgresMigrationsAndSchema(t *testing.T) {
	connStr, cleanup := setupTestPostgres(t)
	defer cleanup()

	// Initialize repo
	pgDB, err := repository.NewPostgresDB(connStr)
	require.NoError(t, err)
	defer pgDB.Close()

	// Verify Ping
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	assert.NoError(t, pgDB.Ping(ctx))

	// Run migrations from embedded FS
	err = pgDB.RunMigrationsFS(migrations.FS, ".")
	require.NoError(t, err, "failed to run initial migrations")

	// Verify all 5 core tables exist
	expectedTables := []string{
		"users",
		"canonical_manhuas",
		"user_libraries",
		"user_ratings",
		"fcm_device_tokens",
	}

	for _, table := range expectedTables {
		var exists bool
		query := `SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' AND table_name = $1
		);`
		err := pgDB.DB.QueryRow(query, table).Scan(&exists)
		assert.NoError(t, err)
		assert.True(t, exists, "table %s should exist in database", table)
	}

	// Verify Golden Rule index exists on user_libraries
	var indexExists bool
	indexQuery := `SELECT EXISTS (
		SELECT FROM pg_indexes 
		WHERE tablename = 'user_libraries' AND indexname = 'idx_user_library_sort'
	);`
	err = pgDB.DB.QueryRow(indexQuery).Scan(&indexExists)
	assert.NoError(t, err)
	assert.True(t, indexExists, "golden rule index idx_user_library_sort should exist")

	// Test inserting a user and canonical manhua
	_, _ = pgDB.DB.Exec("DELETE FROM users WHERE email = 'test@example.com'")
	_, _ = pgDB.DB.Exec("DELETE FROM canonical_manhuas WHERE id = 1001 OR slug = 'solo-leveling'")

	var userID string
	err = pgDB.DB.QueryRow(`
		INSERT INTO users (email, display_name, auth_provider, role) 
		VALUES ('test@example.com', 'Test User', 'google', 'reader') 
		RETURNING id;
	`).Scan(&userID)
	require.NoError(t, err)
	assert.NotEmpty(t, userID)

	var manhuaID int64 = 1001
	_, err = pgDB.DB.Exec(`
		INSERT INTO canonical_manhuas (id, slug, title, primary_domain, url_template)
		VALUES ($1, 'solo-leveling', 'Solo Leveling', 'asurascans.com', 'https://asurascans.com/manga/solo-leveling/{chapter}')
	`, manhuaID)
	require.NoError(t, err)

	// Verify foreign key and cascade deletion on user_libraries
	_, err = pgDB.DB.Exec(`
		INSERT INTO user_libraries (user_id, canonical_manhua_id, current_chapter, is_favorite, custom_order, updated_at)
		VALUES ($1, $2, 10, true, 0, $3)
	`, userID, manhuaID, time.Now().Unix())
	require.NoError(t, err)

	// Test deleting user cascades to user_libraries
	_, err = pgDB.DB.Exec("DELETE FROM users WHERE id = $1", userID)
	require.NoError(t, err)

	var libCount int
	err = pgDB.DB.QueryRow("SELECT COUNT(*) FROM user_libraries WHERE user_id = $1", userID).Scan(&libCount)
	require.NoError(t, err)
	assert.Equal(t, 0, libCount, "user_libraries should be deleted on cascade when user is deleted")
}
