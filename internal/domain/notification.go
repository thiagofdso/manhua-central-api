package domain

import (
	"context"
	"time"
)

type Notification struct {
	ID                string    `json:"id"`
	UserID            string    `json:"userId"`
	CanonicalManhuaID int64     `json:"canonicalManhuaId"`
	Chapter           int       `json:"chapter"`
	Title             string    `json:"title"`
	Body              string    `json:"body"`
	IsRead            bool      `json:"isRead"`
	CreatedAt         time.Time `json:"createdAt"`
	ManhuaSlug        string    `json:"manhuaSlug,omitempty"`
	ManhuaCoverURL    *string   `json:"manhuaCoverUrl,omitempty"`
}

type NotificationRepository interface {
	ListByUser(ctx context.Context, userID string) ([]*Notification, error)
	CreateBatch(ctx context.Context, notifications []*Notification) error
	MarkAsRead(ctx context.Context, id string, userID string) error
	Dismiss(ctx context.Context, id string, userID string) error
	DismissAll(ctx context.Context, userID string) error
	GetUnreadCount(ctx context.Context, userID string) (int, error)
}
