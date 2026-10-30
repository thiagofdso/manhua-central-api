package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
)

var (
	ErrCanonicalNotFound = errors.New("canonical manhua not found")
)

type CanonicalRepository interface {
	Create(ctx context.Context, m *domain.CanonicalManhua) error
	GetByID(ctx context.Context, id int64) (*domain.CanonicalManhua, error)
	GetBySlug(ctx context.Context, slug string) (*domain.CanonicalManhua, error)
	GetByIDs(ctx context.Context, ids []int64) ([]*domain.CanonicalManhua, error)
	Update(ctx context.Context, m *domain.CanonicalManhua) error
	List(ctx context.Context, filter domain.CatalogFilter) (*domain.CatalogSearchResult, error)
	UpsertBatch(ctx context.Context, items []*domain.CanonicalManhua) error
	ListDomains(ctx context.Context) ([]string, error)
}

type PostgresCanonicalRepository struct {
	db *sql.DB
}

func NewPostgresCanonicalRepository(db *sql.DB) *PostgresCanonicalRepository {
	return &PostgresCanonicalRepository{db: db}
}

func (r *PostgresCanonicalRepository) Create(ctx context.Context, m *domain.CanonicalManhua) error {
	if m.ID == 0 {
		m.ID = time.Now().UnixNano() / 1e6
	}

	query := `
		INSERT INTO canonical_manhuas (
			id, slug, title, alternative_titles, synopsis, cover_url,
			primary_domain, url_template, last_chapter, update_day,
			is_hiatus, bayesian_rating, total_votes, total_views, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)
		RETURNING created_at, updated_at;
	`

	var altTitles, synopsis, coverURL sql.NullString
	if m.AlternativeTitles != nil {
		altTitles = sql.NullString{String: *m.AlternativeTitles, Valid: true}
	}
	if m.Synopsis != nil {
		synopsis = sql.NullString{String: *m.Synopsis, Valid: true}
	}
	if m.CoverURL != nil {
		coverURL = sql.NullString{String: *m.CoverURL, Valid: true}
	}

	var lastCh, updateDay sql.NullInt64
	if m.LastChapter != nil {
		lastCh = sql.NullInt64{Int64: int64(*m.LastChapter), Valid: true}
	}
	if m.UpdateDay != nil {
		updateDay = sql.NullInt64{Int64: int64(*m.UpdateDay), Valid: true}
	}

	err := r.db.QueryRowContext(ctx, query,
		m.ID,
		m.Slug,
		m.Title,
		altTitles,
		synopsis,
		coverURL,
		m.PrimaryDomain,
		m.URLTemplate,
		lastCh,
		updateDay,
		m.IsHiatus,
		m.BayesianRating,
		m.TotalVotes,
		m.TotalViews,
	).Scan(&m.CreatedAt, &m.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create canonical manhua: %w", err)
	}

	return nil
}

func (r *PostgresCanonicalRepository) GetByID(ctx context.Context, id int64) (*domain.CanonicalManhua, error) {
	query := `
		SELECT id, slug, title, alternative_titles, synopsis, cover_url,
		       primary_domain, url_template, last_chapter, update_day,
		       is_hiatus, bayesian_rating, total_votes, total_views, created_at, updated_at
		FROM canonical_manhuas
		WHERE id = $1;
	`
	row := r.db.QueryRowContext(ctx, query, id)
	return scanCanonical(row)
}

func (r *PostgresCanonicalRepository) GetBySlug(ctx context.Context, slug string) (*domain.CanonicalManhua, error) {
	query := `
		SELECT id, slug, title, alternative_titles, synopsis, cover_url,
		       primary_domain, url_template, last_chapter, update_day,
		       is_hiatus, bayesian_rating, total_votes, total_views, created_at, updated_at
		FROM canonical_manhuas
		WHERE slug = $1;
	`
	row := r.db.QueryRowContext(ctx, query, slug)
	return scanCanonical(row)
}

func (r *PostgresCanonicalRepository) GetByIDs(ctx context.Context, ids []int64) ([]*domain.CanonicalManhua, error) {
	if len(ids) == 0 {
		return []*domain.CanonicalManhua{}, nil
	}

	query := `
		SELECT id, slug, title, alternative_titles, synopsis, cover_url,
		       primary_domain, url_template, last_chapter, update_day,
		       is_hiatus, bayesian_rating, total_votes, total_views, created_at, updated_at
		FROM canonical_manhuas
		WHERE id = ANY($1);
	`
	rows, err := r.db.QueryContext(ctx, query, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("failed to query canonical manhuas by ids: %w", err)
	}
	defer rows.Close()

	itemsMap := make(map[int64]*domain.CanonicalManhua)
	for rows.Next() {
		m, err := scanCanonicalRows(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan canonical row: %w", err)
		}
		itemsMap[m.ID] = m
	}

	// Preserve the exact order of the input `ids` slice
	ordered := make([]*domain.CanonicalManhua, 0, len(ids))
	for _, id := range ids {
		if item, found := itemsMap[id]; found {
			ordered = append(ordered, item)
		}
	}

	return ordered, nil
}

func (r *PostgresCanonicalRepository) Update(ctx context.Context, m *domain.CanonicalManhua) error {
	query := `
		UPDATE canonical_manhuas
		SET title = $2,
		    alternative_titles = $3,
		    synopsis = $4,
		    cover_url = $5,
		    primary_domain = $6,
		    url_template = $7,
		    last_chapter = $8,
		    update_day = $9,
		    is_hiatus = $10,
		    bayesian_rating = $11,
		    total_votes = $12,
		    total_views = $13,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING updated_at;
	`

	var altTitles, synopsis, coverURL sql.NullString
	if m.AlternativeTitles != nil {
		altTitles = sql.NullString{String: *m.AlternativeTitles, Valid: true}
	}
	if m.Synopsis != nil {
		synopsis = sql.NullString{String: *m.Synopsis, Valid: true}
	}
	if m.CoverURL != nil {
		coverURL = sql.NullString{String: *m.CoverURL, Valid: true}
	}

	var lastCh, updateDay sql.NullInt64
	if m.LastChapter != nil {
		lastCh = sql.NullInt64{Int64: int64(*m.LastChapter), Valid: true}
	}
	if m.UpdateDay != nil {
		updateDay = sql.NullInt64{Int64: int64(*m.UpdateDay), Valid: true}
	}

	err := r.db.QueryRowContext(ctx, query,
		m.ID,
		m.Title,
		altTitles,
		synopsis,
		coverURL,
		m.PrimaryDomain,
		m.URLTemplate,
		lastCh,
		updateDay,
		m.IsHiatus,
		m.BayesianRating,
		m.TotalVotes,
		m.TotalViews,
	).Scan(&m.UpdatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCanonicalNotFound
		}
		return fmt.Errorf("failed to update canonical manhua: %w", err)
	}

	return nil
}

func (r *PostgresCanonicalRepository) List(ctx context.Context, filter domain.CatalogFilter) (*domain.CatalogSearchResult, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	} else if filter.PageSize > 100 {
		filter.PageSize = 100
	}

	var whereClauses []string
	var args []interface{}
	argIdx := 1

	if strings.TrimSpace(filter.Query) != "" {
		param := "%" + strings.TrimSpace(filter.Query) + "%"
		whereClauses = append(whereClauses, fmt.Sprintf("(title ILIKE $%d OR alternative_titles ILIKE $%d OR slug ILIKE $%d)", argIdx, argIdx, argIdx))
		args = append(args, param)
		argIdx++
	}

	if strings.TrimSpace(filter.Domain) != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("primary_domain = $%d", argIdx))
		args = append(args, strings.TrimSpace(filter.Domain))
		argIdx++
	}

	if filter.UpdateDay != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("update_day = $%d", argIdx))
		args = append(args, *filter.UpdateDay)
		argIdx++
	}

	if filter.IsHiatus != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("is_hiatus = $%d", argIdx))
		args = append(args, *filter.IsHiatus)
		argIdx++
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Count query
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM canonical_manhuas %s;", whereSQL)
	var total int
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to count canonical manhuas: %w", err)
	}

	// Order by
	orderBy := "updated_at DESC"
	switch filter.SortBy {
	case "views":
		orderBy = "total_views DESC, updated_at DESC"
	case "rating":
		orderBy = "bayesian_rating DESC, total_votes DESC"
	case "title":
		orderBy = "title ASC"
	case "updated":
		orderBy = "updated_at DESC"
	}

	offset := (filter.Page - 1) * filter.PageSize
	querySQL := fmt.Sprintf(`
		SELECT id, slug, title, alternative_titles, synopsis, cover_url,
		       primary_domain, url_template, last_chapter, update_day,
		       is_hiatus, bayesian_rating, total_votes, total_views, created_at, updated_at
		FROM canonical_manhuas
		%s
		ORDER BY %s
		LIMIT $%d OFFSET $%d;
	`, whereSQL, orderBy, argIdx, argIdx+1)

	queryArgs := append(args, filter.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query canonical manhuas: %w", err)
	}
	defer rows.Close()

	items := make([]*domain.CanonicalManhua, 0, filter.PageSize)
	for rows.Next() {
		m, err := scanCanonicalRows(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		items = append(items, m)
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + filter.PageSize - 1) / filter.PageSize
	}

	return &domain.CatalogSearchResult{
		Items:      items,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

func (r *PostgresCanonicalRepository) UpsertBatch(ctx context.Context, items []*domain.CanonicalManhua) error {
	if len(items) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT INTO canonical_manhuas (
			id, slug, title, alternative_titles, synopsis, cover_url,
			primary_domain, url_template, last_chapter, update_day,
			is_hiatus, bayesian_rating, total_votes, total_views, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)
		ON CONFLICT (slug) DO UPDATE SET
			title = EXCLUDED.title,
			alternative_titles = COALESCE(EXCLUDED.alternative_titles, canonical_manhuas.alternative_titles),
			synopsis = COALESCE(EXCLUDED.synopsis, canonical_manhuas.synopsis),
			cover_url = CASE WHEN canonical_manhuas.cover_url LIKE '/api/media/covers/%' THEN canonical_manhuas.cover_url ELSE COALESCE(NULLIF(EXCLUDED.cover_url, ''), canonical_manhuas.cover_url) END,
			url_template = EXCLUDED.url_template,
			last_chapter = COALESCE(EXCLUDED.last_chapter, canonical_manhuas.last_chapter),
			update_day = COALESCE(EXCLUDED.update_day, canonical_manhuas.update_day),
			updated_at = CURRENT_TIMESTAMP;
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare upsert: %w", err)
	}
	defer stmt.Close()

	baseID := (time.Now().UnixNano() / 1e6) * 1000
	for i, m := range items {
		if m.ID == 0 {
			m.ID = baseID + int64(i)
		}

		var altTitles, synopsis, coverURL sql.NullString
		if m.AlternativeTitles != nil {
			altTitles = sql.NullString{String: *m.AlternativeTitles, Valid: true}
		}
		if m.Synopsis != nil {
			synopsis = sql.NullString{String: *m.Synopsis, Valid: true}
		}
		if m.CoverURL != nil {
			coverURL = sql.NullString{String: *m.CoverURL, Valid: true}
		}

		var lastCh, updateDay sql.NullInt64
		if m.LastChapter != nil {
			lastCh = sql.NullInt64{Int64: int64(*m.LastChapter), Valid: true}
		}
		if m.UpdateDay != nil {
			updateDay = sql.NullInt64{Int64: int64(*m.UpdateDay), Valid: true}
		}

		_, err := stmt.ExecContext(ctx,
			m.ID,
			m.Slug,
			m.Title,
			altTitles,
			synopsis,
			coverURL,
			m.PrimaryDomain,
			m.URLTemplate,
			lastCh,
			updateDay,
			m.IsHiatus,
			m.BayesianRating,
			m.TotalVotes,
			m.TotalViews,
		)
		if err != nil {
			return fmt.Errorf("failed to upsert item %s: %w", m.Slug, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

type scannable interface {
	Scan(dest ...interface{}) error
}

func scanCanonical(s scannable) (*domain.CanonicalManhua, error) {
	var m domain.CanonicalManhua
	var altTitles, synopsis, coverURL sql.NullString
	var lastCh, updateDay sql.NullInt64

	err := s.Scan(
		&m.ID,
		&m.Slug,
		&m.Title,
		&altTitles,
		&synopsis,
		&coverURL,
		&m.PrimaryDomain,
		&m.URLTemplate,
		&lastCh,
		&updateDay,
		&m.IsHiatus,
		&m.BayesianRating,
		&m.TotalVotes,
		&m.TotalViews,
		&m.CreatedAt,
		&m.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCanonicalNotFound
		}
		return nil, fmt.Errorf("failed to scan canonical manhua: %w", err)
	}

	if altTitles.Valid {
		m.AlternativeTitles = &altTitles.String
	}
	if synopsis.Valid {
		m.Synopsis = &synopsis.String
	}
	if coverURL.Valid {
		m.CoverURL = &coverURL.String
	}
	if lastCh.Valid {
		v := int(lastCh.Int64)
		m.LastChapter = &v
	}
	if updateDay.Valid {
		v := int16(updateDay.Int64)
		m.UpdateDay = &v
	}

	return &m, nil
}

func scanCanonicalRows(rows *sql.Rows) (*domain.CanonicalManhua, error) {
	return scanCanonical(rows)
}

func (r *PostgresCanonicalRepository) ListDomains(ctx context.Context) ([]string, error) {
	query := `SELECT DISTINCT primary_domain FROM canonical_manhuas WHERE primary_domain != '' ORDER BY primary_domain ASC;`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var domains []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}
