package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/ranking"
)

var (
	ErrInvalidScore = errors.New("score must be between 1 and 5")
)

type RatingRepository interface {
	SubmitVote(ctx context.Context, userID string, manhuaID int64, score int) (*domain.CanonicalManhua, error)
	GetUserVote(ctx context.Context, userID string, manhuaID int64) (*int, error)
	SyncViewsToPostgres(ctx context.Context, views map[int64]int64) error
}

type PostgresRatingRepository struct {
	db          *sql.DB
	redisClient *RedisClient
}

func NewPostgresRatingRepository(db *sql.DB, redisClient *RedisClient) *PostgresRatingRepository {
	return &PostgresRatingRepository{
		db:          db,
		redisClient: redisClient,
	}
}

// SubmitVote records a user's 1-5 star vote, recalculates the Bayesian rating, and updates the canonical manhua.
func (r *PostgresRatingRepository) SubmitVote(ctx context.Context, userID string, manhuaID int64, score int) (*domain.CanonicalManhua, error) {
	if score < 1 || score > 5 {
		return nil, ErrInvalidScore
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. Upsert rating into user_ratings
	upsertQuery := `
		INSERT INTO user_ratings (id, user_id, canonical_manhua_id, score, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id, canonical_manhua_id)
		DO UPDATE SET score = EXCLUDED.score, updated_at = CURRENT_TIMESTAMP;
	`
	_, err = tx.ExecContext(ctx, upsertQuery, userID, manhuaID, score)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert user rating: %w", err)
	}

	// 2. Compute aggregate stats: v (total votes) and R (mean arithmetic score)
	var totalVotes int
	var avgScore float64
	statsQuery := `
		SELECT COUNT(*), COALESCE(AVG(score), 0.0)
		FROM user_ratings
		WHERE canonical_manhua_id = $1;
	`
	err = tx.QueryRowContext(ctx, statsQuery, manhuaID).Scan(&totalVotes, &avgScore)
	if err != nil {
		return nil, fmt.Errorf("failed to compute rating aggregates: %w", err)
	}

	// 3. Compute Bayesian rating WR
	wr := ranking.CalculateBayesianRating(totalVotes, avgScore, ranking.DefaultMinimumVotes, ranking.DefaultPriorMean)

	// 4. Update canonical_manhuas
	updateQuery := `
		UPDATE canonical_manhuas
		SET bayesian_rating = $1, total_votes = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3
		RETURNING id, slug, title, alternative_titles, synopsis, cover_url,
		          primary_domain, url_template, last_chapter, update_day,
		          is_hiatus, bayesian_rating, total_votes, total_views, created_at, updated_at;
	`
	row := tx.QueryRowContext(ctx, updateQuery, wr, totalVotes, manhuaID)
	manhua, err := scanCanonical(row)
	if err != nil {
		return nil, fmt.Errorf("failed to update canonical manhua rating: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit rating transaction: %w", err)
	}

	// 5. Update Redis Sorted Set for fast leaderboards
	if r.redisClient != nil {
		_ = r.redisClient.SetBayesianRating(ctx, manhuaID, wr)
	}

	return manhua, nil
}

// GetUserVote retrieves a user's vote for a given canonical manhua, returning nil if no vote exists.
func (r *PostgresRatingRepository) GetUserVote(ctx context.Context, userID string, manhuaID int64) (*int, error) {
	query := `SELECT score FROM user_ratings WHERE user_id = $1 AND canonical_manhua_id = $2;`
	var score int
	err := r.db.QueryRowContext(ctx, query, userID, manhuaID).Scan(&score)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user vote: %w", err)
	}
	return &score, nil
}

// SyncViewsToPostgres persists accumulated view counts from Redis into PostgreSQL.
func (r *PostgresRatingRepository) SyncViewsToPostgres(ctx context.Context, views map[int64]int64) error {
	if len(views) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		UPDATE canonical_manhuas
		SET total_views = GREATEST(total_views, $1), updated_at = CURRENT_TIMESTAMP
		WHERE id = $2;
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare sync view statement: %w", err)
	}
	defer stmt.Close()

	for id, viewCount := range views {
		_, err := stmt.ExecContext(ctx, viewCount, id)
		if err != nil {
			return fmt.Errorf("failed to sync views for manhua %d: %w", id, err)
		}
	}

	return tx.Commit()
}
