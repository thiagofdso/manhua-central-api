package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
)

type PostgresNotificationRepository struct {
	db *sql.DB
}

func NewPostgresNotificationRepository(db *sql.DB) *PostgresNotificationRepository {
	return &PostgresNotificationRepository{db: db}
}

func (r *PostgresNotificationRepository) ListByUser(ctx context.Context, userID string) ([]*domain.Notification, error) {
	query := `
		SELECT n.id, n.user_id, n.canonical_manhua_id, n.chapter, n.title, n.body, n.is_read, n.created_at,
		       c.slug, c.cover_url
		FROM notifications n
		JOIN canonical_manhuas c ON c.id = n.canonical_manhua_id
		WHERE n.user_id = $1
		ORDER BY n.created_at DESC
		LIMIT 50
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifs []*domain.Notification
	for rows.Next() {
		var n domain.Notification
		var coverURL sql.NullString

		if err := rows.Scan(
			&n.ID,
			&n.UserID,
			&n.CanonicalManhuaID,
			&n.Chapter,
			&n.Title,
			&n.Body,
			&n.IsRead,
			&n.CreatedAt,
			&n.ManhuaSlug,
			&coverURL,
		); err != nil {
			return nil, err
		}

		if coverURL.Valid {
			n.ManhuaCoverURL = &coverURL.String
		}

		notifs = append(notifs, &n)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return notifs, nil
}

func (r *PostgresNotificationRepository) CreateBatch(ctx context.Context, notifications []*domain.Notification) error {
	if len(notifications) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO notifications (id, user_id, canonical_manhua_id, chapter, title, body, is_read, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, n := range notifications {
		if n.ID == "" {
			n.ID = uuid.New().String()
		}
		if n.CreatedAt.IsZero() {
			n.CreatedAt = time.Now()
		}

		_, err := stmt.ExecContext(ctx, n.ID, n.UserID, n.CanonicalManhuaID, n.Chapter, n.Title, n.Body, n.IsRead, n.CreatedAt)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *PostgresNotificationRepository) MarkAsRead(ctx context.Context, id string, userID string) error {
	query := `UPDATE notifications SET is_read = TRUE WHERE id = $1 AND user_id = $2`
	_, err := r.db.ExecContext(ctx, query, id, userID)
	return err
}

func (r *PostgresNotificationRepository) Dismiss(ctx context.Context, id string, userID string) error {
	query := `DELETE FROM notifications WHERE id = $1 AND user_id = $2`
	_, err := r.db.ExecContext(ctx, query, id, userID)
	return err
}

func (r *PostgresNotificationRepository) DismissAll(ctx context.Context, userID string) error {
	query := `DELETE FROM notifications WHERE user_id = $1`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

func (r *PostgresNotificationRepository) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	query := `SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = FALSE`
	var count int
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&count)
	return count, err
}
