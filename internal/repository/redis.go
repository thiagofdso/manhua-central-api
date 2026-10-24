package repository

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	Client *redis.Client
}

// NewRedisClient creates and validates a connection to Redis.
func NewRedisClient(redisURL string) (*RedisClient, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis URL: %w", err)
	}

	client := redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	return &RedisClient{Client: client}, nil
}

// Ping checks if the Redis connection is active.
func (r *RedisClient) Ping(ctx context.Context) error {
	return r.Client.Ping(ctx).Err()
}

// IncrementViews increments the view counter in the Redis Sorted Set for a specific period.
func (r *RedisClient) IncrementViews(ctx context.Context, period string, manhuaID int64, delta float64) error {
	key := fmt.Sprintf("rankings:views:%s", period)
	member := strconv.FormatInt(manhuaID, 10)
	return r.Client.ZIncrBy(ctx, key, delta, member).Err()
}

// GetTopViewed returns the top N canonical manhua IDs sorted by view count descending.
func (r *RedisClient) GetTopViewed(ctx context.Context, period string, limit int64) ([]int64, error) {
	if limit <= 0 {
		limit = 10
	}
	key := fmt.Sprintf("rankings:views:%s", period)
	// ZRevRange returns highest scores first (start 0, stop limit-1)
	results, err := r.Client.ZRevRange(ctx, key, 0, limit-1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get top viewed from redis: %w", err)
	}

	ids := make([]int64, 0, len(results))
	for _, strID := range results {
		id, err := strconv.ParseInt(strID, 10, 64)
		if err == nil {
			ids = append(ids, id)
		}
	}

	return ids, nil
}

// Close terminates the Redis client connection.
func (r *RedisClient) Close() error {
	return r.Client.Close()
}
