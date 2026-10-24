package repository_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
)

func setupTestRedis(t *testing.T) (string, func()) {
	t.Helper()

	if envURL := os.Getenv("TEST_REDIS_URL"); envURL != "" {
		return envURL, func() {}
	}

	ctx := context.Background()
	redisContainer, err := tcredis.Run(ctx,
		"redis:7-alpine",
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").
				WithOccurrence(1).
				WithStartupTimeout(20*time.Second)),
	)
	require.NoError(t, err, "failed to start testcontainers redis")

	connStr, err := redisContainer.ConnectionString(ctx)
	require.NoError(t, err, "failed to get redis connection string")

	cleanup := func() {
		_ = redisContainer.Terminate(ctx)
	}

	return connStr, cleanup
}

func TestRedisRankingOperations(t *testing.T) {
	connStr, cleanup := setupTestRedis(t)
	defer cleanup()

	redisClient, err := repository.NewRedisClient(connStr)
	require.NoError(t, err)
	defer redisClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Verify Ping
	err = redisClient.Ping(ctx)
	assert.NoError(t, err)

	// Increment views for manhuas 101, 102, 103
	period := "daily"
	err = redisClient.IncrementViews(ctx, period, 101, 15)
	require.NoError(t, err)

	err = redisClient.IncrementViews(ctx, period, 102, 42)
	require.NoError(t, err)

	err = redisClient.IncrementViews(ctx, period, 103, 7)
	require.NoError(t, err)

	// Increment 101 again by 30 => 15 + 30 = 45 (now highest)
	err = redisClient.IncrementViews(ctx, period, 101, 30)
	require.NoError(t, err)

	// Get Top 3
	topManhuas, err := redisClient.GetTopViewed(ctx, period, 3)
	require.NoError(t, err)
	require.Len(t, topManhuas, 3)

	// 101 (45) should be #1, 102 (42) #2, 103 (7) #3
	assert.Equal(t, int64(101), topManhuas[0])
	assert.Equal(t, int64(102), topManhuas[1])
	assert.Equal(t, int64(103), topManhuas[2])
}
