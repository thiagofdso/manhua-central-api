package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
)

type ChapterChecker func(ctx context.Context, manhua *domain.CanonicalManhua) (int, error)

type CheckResult struct {
	LockAcquired      bool `json:"lockAcquired"`
	UpdatedManhuas    int  `json:"updatedManhuas"`
	NotificationsSent int  `json:"notificationsSent"`
}

type Scheduler struct {
	db            *sql.DB
	canonicalRepo repository.CanonicalRepository
	notifRepo     domain.NotificationRepository
	checker       ChapterChecker
}

func NewScheduler(
	db *sql.DB,
	canonicalRepo repository.CanonicalRepository,
	notifRepo domain.NotificationRepository,
	checker ChapterChecker,
) *Scheduler {
	return &Scheduler{
		db:            db,
		canonicalRepo: canonicalRepo,
		notifRepo:     notifRepo,
		checker:       checker,
	}
}

func (s *Scheduler) RunRoutineCheck(ctx context.Context) (*CheckResult, error) {
	// 1. Begin transaction for advisory lock
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 2. Try transactional advisory lock: hashtext('daily_releases_check_2350')
	var acquired bool
	err = tx.QueryRowContext(ctx, "SELECT pg_try_advisory_xact_lock(hashtext('daily_releases_check_2350'))").Scan(&acquired)
	if err != nil {
		return nil, fmt.Errorf("failed to check advisory lock: %w", err)
	}

	if !acquired {
		return &CheckResult{LockAcquired: false}, nil
	}

	// 3. Find canonical manhuas to check
	rows, err := tx.QueryContext(ctx, `
		SELECT id, slug, title, primary_domain, url_template, COALESCE(last_chapter, 0)
		FROM canonical_manhuas
		WHERE is_hiatus = FALSE
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query canonical manhuas: %w", err)
	}
	defer rows.Close()

	type manhuaItem struct {
		id          int64
		slug        string
		title       string
		domain      string
		urlTemplate string
		lastChapter int
	}

	var items []manhuaItem
	for rows.Next() {
		var m manhuaItem
		if err := rows.Scan(&m.id, &m.slug, &m.title, &m.domain, &m.urlTemplate, &m.lastChapter); err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	rows.Close()

	result := &CheckResult{LockAcquired: true}

	for _, m := range items {
		canonical := &domain.CanonicalManhua{
			ID:            m.id,
			Slug:          m.slug,
			Title:         m.title,
			PrimaryDomain: m.domain,
			URLTemplate:   m.urlTemplate,
			LastChapter:   &m.lastChapter,
		}

		latestCh := m.lastChapter
		if s.checker != nil {
			detected, chErr := s.checker(ctx, canonical)
			if chErr == nil && detected > latestCh {
				latestCh = detected
			}
		}

		if latestCh > m.lastChapter {
			// Update last_chapter in DB
			_, err = tx.ExecContext(ctx, "UPDATE canonical_manhuas SET last_chapter = $1, updated_at = NOW() WHERE id = $2", latestCh, m.id)
			if err != nil {
				log.Printf("[Scheduler] Error updating last_chapter for %d: %v", m.id, err)
				continue
			}
			result.UpdatedManhuas++

			// Find all users who have this manhua in their library
			userRows, err := tx.QueryContext(ctx, "SELECT user_id FROM user_libraries WHERE canonical_manhua_id = $1", m.id)
			if err != nil {
				log.Printf("[Scheduler] Error querying users for manhua %d: %v", m.id, err)
				continue
			}

			var userIDs []string
			for userRows.Next() {
				var uID string
				if err := userRows.Scan(&uID); err == nil {
					userIDs = append(userIDs, uID)
				}
			}
			userRows.Close()

			// Prepare notification batch
			var batch []*domain.Notification
			now := time.Now()
			for _, uID := range userIDs {
				batch = append(batch, &domain.Notification{
					ID:                uuid.New().String(),
					UserID:            uID,
					CanonicalManhuaID: m.id,
					Chapter:           latestCh,
					Title:             fmt.Sprintf("Novo Capítulo de %s!", m.title),
					Body:              fmt.Sprintf("O capítulo %d de %s já está disponível para leitura.", latestCh, m.title),
					IsRead:            false,
					CreatedAt:         now,
				})
			}

			if len(batch) > 0 {
				stmt, err := tx.PrepareContext(ctx, `
					INSERT INTO notifications (id, user_id, canonical_manhua_id, chapter, title, body, is_read, created_at)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				`)
				if err == nil {
					for _, notif := range batch {
						_, execErr := stmt.ExecContext(ctx, notif.ID, notif.UserID, notif.CanonicalManhuaID, notif.Chapter, notif.Title, notif.Body, notif.IsRead, notif.CreatedAt)
						if execErr == nil {
							result.NotificationsSent++
						}
					}
					stmt.Close()
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return result, nil
}
