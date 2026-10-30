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

// RecordViewRateLimited checks the 15-minute cooldown per identifier (IP or user ID) and increments daily, weekly, all-time ZSETs if allowed.
// Returns (true, nil) if the view was counted, or (false, nil) if rate-limited.
func (r *RedisClient) RecordViewRateLimited(ctx context.Context, manhuaID int64, identifier string) (bool, error) {
	dedupKey := fmt.Sprintf("view:%d:%s", manhuaID, identifier)
	ok, err := r.Client.SetNX(ctx, dedupKey, 1, 15*time.Minute).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check view cooldown in redis: %w", err)
	}
	if !ok {
		return false, nil
	}

	now := time.Now().UTC()
	member := strconv.FormatInt(manhuaID, 10)

	// Daily key
	dailyKey := fmt.Sprintf("rankings:views:daily:%s", now.Format("2006-01-02"))
	_ = r.Client.ZIncrBy(ctx, dailyKey, 1, member).Err()
	_ = r.Client.Expire(ctx, dailyKey, 48*time.Hour).Err()

	// Weekly key
	year, week := now.ISOWeek()
	weeklyKey := fmt.Sprintf("rankings:views:weekly:%d-%02d", year, week)
	_ = r.Client.ZIncrBy(ctx, weeklyKey, 1, member).Err()
	_ = r.Client.Expire(ctx, weeklyKey, 14*24*time.Hour).Err()

	// All-time key
	allTimeKey := "rankings:views:all_time"
	_ = r.Client.ZIncrBy(ctx, allTimeKey, 1, member).Err()

	return true, nil
}

// SetBayesianRating sets or updates the Bayesian rating in the top_rated ZSET.
func (r *RedisClient) SetBayesianRating(ctx context.Context, manhuaID int64, rating float64) error {
	member := strconv.FormatInt(manhuaID, 10)
	return r.Client.ZAdd(ctx, "rankings:top_rated", redis.Z{
		Score:  rating,
		Member: member,
	}).Err()
}

// GetTopRanked returns the top N canonical manhua IDs and scores sorted descending from a given Redis key.
func (r *RedisClient) GetTopRanked(ctx context.Context, key string, limit int64) ([]int64, []float64, error) {
	if limit <= 0 {
		limit = 20
	}
	results, err := r.Client.ZRevRangeWithScores(ctx, key, 0, limit-1).Result()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get top ranked from redis key %s: %w", key, err)
	}

	ids := make([]int64, 0, len(results))
	scores := make([]float64, 0, len(results))
	for _, item := range results {
		strMember, ok := item.Member.(string)
		if !ok {
			continue
		}
		id, err := strconv.ParseInt(strMember, 10, 64)
		if err == nil {
			ids = append(ids, id)
			scores = append(scores, item.Score)
		}
	}

	return ids, scores, nil
}

// GetAllViews returns all manhua IDs and accumulated views from the all-time ranking.
func (r *RedisClient) GetAllViews(ctx context.Context) (map[int64]int64, error) {
	results, err := r.Client.ZRangeWithScores(ctx, "rankings:views:all_time", 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get all views from redis: %w", err)
	}

	views := make(map[int64]int64, len(results))
	for _, item := range results {
		strMember, ok := item.Member.(string)
		if !ok {
			continue
		}
		id, err := strconv.ParseInt(strMember, 10, 64)
		if err == nil {
			views[id] = int64(item.Score)
		}
	}

	return views, nil
}

// GetTopViewed returns the top N canonical manhua IDs sorted by view count descending.
func (r *RedisClient) GetTopViewed(ctx context.Context, period string, limit int64) ([]int64, error) {
	if limit <= 0 {
		limit = 10
	}
	key := fmt.Sprintf("rankings:views:%s", period)
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
