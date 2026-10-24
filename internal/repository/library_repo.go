package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
)

var (
	ErrLibraryItemNotFound = errors.New("library item not found")
	ErrLibraryItemConflict = errors.New("manhua already in user library")
)

type LibraryRepository interface {
	Create(ctx context.Context, item *domain.UserLibrary) error
	GetByID(ctx context.Context, userID, id string) (*domain.UserLibrary, error)
	GetByUserAndManhuaID(ctx context.Context, userID string, manhuaID int64) (*domain.UserLibrary, error)
	ListByUser(ctx context.Context, userID string) ([]*domain.UserLibrary, error)
	Update(ctx context.Context, item *domain.UserLibrary) error
	UpdateOrderBatch(ctx context.Context, userID string, items []*domain.UserLibrary) error
	Delete(ctx context.Context, userID, id string) error
	GetMinCustomOrder(ctx context.Context, userID string, isFavorite bool) (int, error)
}

type PostgresLibraryRepository struct {
	db *sql.DB
}

func NewPostgresLibraryRepository(db *sql.DB) *PostgresLibraryRepository {
	return &PostgresLibraryRepository{db: db}
}

func (r *PostgresLibraryRepository) Create(ctx context.Context, item *domain.UserLibrary) error {
	if item.ID == "" {
		item.ID = uuid.New().String()
	}
	if item.UpdatedAt == 0 {
		item.UpdatedAt = time.Now().Unix()
	}

	query := `
		INSERT INTO user_libraries (
			id, user_id, canonical_manhua_id, current_chapter, is_favorite, custom_order, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		)
		RETURNING created_at;
	`

	err := r.db.QueryRowContext(ctx, query,
		item.ID,
		item.UserID,
		item.CanonicalManhuaID,
		item.CurrentChapter,
		item.IsFavorite,
		item.CustomOrder,
		item.UpdatedAt,
	).Scan(&item.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to add item to library: %w", err)
	}

	return nil
}

func (r *PostgresLibraryRepository) GetByID(ctx context.Context, userID, id string) (*domain.UserLibrary, error) {
	query := `
		SELECT ul.id, ul.user_id, ul.canonical_manhua_id, ul.current_chapter, ul.is_favorite, ul.custom_order, ul.created_at, ul.updated_at,
		       cm.id, cm.slug, cm.title, cm.alternative_titles, cm.synopsis, cm.cover_url,
		       cm.primary_domain, cm.url_template, cm.last_chapter, cm.update_day, cm.is_hiatus,
		       cm.bayesian_rating, cm.total_votes, cm.total_views, cm.created_at, cm.updated_at
		FROM user_libraries ul
		JOIN canonical_manhuas cm ON ul.canonical_manhua_id = cm.id
		WHERE ul.user_id = $1 AND ul.id = $2;
	`
	row := r.db.QueryRowContext(ctx, query, userID, id)
	return scanLibraryItem(row)
}

func (r *PostgresLibraryRepository) GetByUserAndManhuaID(ctx context.Context, userID string, manhuaID int64) (*domain.UserLibrary, error) {
	query := `
		SELECT ul.id, ul.user_id, ul.canonical_manhua_id, ul.current_chapter, ul.is_favorite, ul.custom_order, ul.created_at, ul.updated_at,
		       cm.id, cm.slug, cm.title, cm.alternative_titles, cm.synopsis, cm.cover_url,
		       cm.primary_domain, cm.url_template, cm.last_chapter, cm.update_day, cm.is_hiatus,
		       cm.bayesian_rating, cm.total_votes, cm.total_views, cm.created_at, cm.updated_at
		FROM user_libraries ul
		JOIN canonical_manhuas cm ON ul.canonical_manhua_id = cm.id
		WHERE ul.user_id = $1 AND ul.canonical_manhua_id = $2;
	`
	row := r.db.QueryRowContext(ctx, query, userID, manhuaID)
	return scanLibraryItem(row)
}

func (r *PostgresLibraryRepository) ListByUser(ctx context.Context, userID string) ([]*domain.UserLibrary, error) {
	query := `
		SELECT ul.id, ul.user_id, ul.canonical_manhua_id, ul.current_chapter, ul.is_favorite, ul.custom_order, ul.created_at, ul.updated_at,
		       cm.id, cm.slug, cm.title, cm.alternative_titles, cm.synopsis, cm.cover_url,
		       cm.primary_domain, cm.url_template, cm.last_chapter, cm.update_day, cm.is_hiatus,
		       cm.bayesian_rating, cm.total_votes, cm.total_views, cm.created_at, cm.updated_at
		FROM user_libraries ul
		JOIN canonical_manhuas cm ON ul.canonical_manhua_id = cm.id
		WHERE ul.user_id = $1
		ORDER BY ul.is_favorite DESC, ul.custom_order ASC, ul.updated_at DESC;
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list user library: %w", err)
	}
	defer rows.Close()

	var list []*domain.UserLibrary
	for rows.Next() {
		it, err := scanLibraryItem(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, it)
	}

	return list, nil
}

func (r *PostgresLibraryRepository) Update(ctx context.Context, item *domain.UserLibrary) error {
	query := `
		UPDATE user_libraries
		SET current_chapter = $3,
		    is_favorite = $4,
		    custom_order = $5,
		    updated_at = $6
		WHERE user_id = $1 AND id = $2;
	`

	res, err := r.db.ExecContext(ctx, query,
		item.UserID,
		item.ID,
		item.CurrentChapter,
		item.IsFavorite,
		item.CustomOrder,
		item.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update library item: %w", err)
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrLibraryItemNotFound
	}

	return nil
}

func (r *PostgresLibraryRepository) UpdateOrderBatch(ctx context.Context, userID string, items []*domain.UserLibrary) error {
	if len(items) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin tx: %w", err)
	}
	defer tx.Rollback()

	query := `
		UPDATE user_libraries
		SET custom_order = $3,
		    is_favorite = $4
		WHERE user_id = $1 AND id = $2;
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare order update: %w", err)
	}
	defer stmt.Close()

	for _, it := range items {
		_, err := stmt.ExecContext(ctx, userID, it.ID, it.CustomOrder, it.IsFavorite)
		if err != nil {
			return fmt.Errorf("failed to update item order %s: %w", it.ID, err)
		}
	}

	return tx.Commit()
}

func (r *PostgresLibraryRepository) Delete(ctx context.Context, userID, id string) error {
	query := `DELETE FROM user_libraries WHERE user_id = $1 AND id = $2;`
	res, err := r.db.ExecContext(ctx, query, userID, id)
	if err != nil {
		return fmt.Errorf("failed to delete library item: %w", err)
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrLibraryItemNotFound
	}
	return nil
}

func (r *PostgresLibraryRepository) GetMinCustomOrder(ctx context.Context, userID string, isFavorite bool) (int, error) {
	query := `SELECT COALESCE(MIN(custom_order), 0) FROM user_libraries WHERE user_id = $1 AND is_favorite = $2;`
	var minOrder int
	err := r.db.QueryRowContext(ctx, query, userID, isFavorite).Scan(&minOrder)
	if err != nil {
		return 0, fmt.Errorf("failed to get min custom order: %w", err)
	}
	return minOrder, nil
}

func scanLibraryItem(s scannable) (*domain.UserLibrary, error) {
	var ul domain.UserLibrary
	var cm domain.CanonicalManhua
	var altTitles, synopsis, coverURL sql.NullString
	var lastCh, updateDay sql.NullInt64

	err := s.Scan(
		&ul.ID,
		&ul.UserID,
		&ul.CanonicalManhuaID,
		&ul.CurrentChapter,
		&ul.IsFavorite,
		&ul.CustomOrder,
		&ul.CreatedAt,
		&ul.UpdatedAt,
		&cm.ID,
		&cm.Slug,
		&cm.Title,
		&altTitles,
		&synopsis,
		&coverURL,
		&cm.PrimaryDomain,
		&cm.URLTemplate,
		&lastCh,
		&updateDay,
		&cm.IsHiatus,
		&cm.BayesianRating,
		&cm.TotalVotes,
		&cm.TotalViews,
		&cm.CreatedAt,
		&cm.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrLibraryItemNotFound
		}
		return nil, fmt.Errorf("failed to scan library item: %w", err)
	}

	if altTitles.Valid {
		cm.AlternativeTitles = &altTitles.String
	}
	if synopsis.Valid {
		cm.Synopsis = &synopsis.String
	}
	if coverURL.Valid {
		cm.CoverURL = &coverURL.String
	}
	if lastCh.Valid {
		v := int(lastCh.Int64)
		cm.LastChapter = &v
	}
	if updateDay.Valid {
		v := int16(updateDay.Int64)
		cm.UpdateDay = &v
	}

	ul.CanonicalManhua = &cm
	return &ul, nil
}
