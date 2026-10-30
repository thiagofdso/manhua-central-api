package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/auth"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/handlers"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/middleware"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/scheduler"
)

type mockNotifRepo struct {
	items []*domain.Notification
}

func (m *mockNotifRepo) ListByUser(ctx context.Context, userID string) ([]*domain.Notification, error) {
	var res []*domain.Notification
	for _, n := range m.items {
		if n.UserID == userID {
			res = append(res, n)
		}
	}
	return res, nil
}

func (m *mockNotifRepo) CreateBatch(ctx context.Context, notifications []*domain.Notification) error {
	m.items = append(m.items, notifications...)
	return nil
}

func (m *mockNotifRepo) MarkAsRead(ctx context.Context, id string, userID string) error {
	for _, n := range m.items {
		if n.ID == id && n.UserID == userID {
			n.IsRead = true
			return nil
		}
	}
	return nil
}

func (m *mockNotifRepo) Dismiss(ctx context.Context, id string, userID string) error {
	var filtered []*domain.Notification
	for _, n := range m.items {
		if !(n.ID == id && n.UserID == userID) {
			filtered = append(filtered, n)
		}
	}
	m.items = filtered
	return nil
}

func (m *mockNotifRepo) DismissAll(ctx context.Context, userID string) error {
	var filtered []*domain.Notification
	for _, n := range m.items {
		if n.UserID != userID {
			filtered = append(filtered, n)
		}
	}
	m.items = filtered
	return nil
}

func (m *mockNotifRepo) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	count := 0
	for _, n := range m.items {
		if n.UserID == userID && !n.IsRead {
			count++
		}
	}
	return count, nil
}

func TestNotificationsHandler(t *testing.T) {
	jwtService := auth.NewJWTService("super_secret_test_key_minimum_32_bytes_long", "super_secret_test_key_minimum_32_bytes_long", 15*time.Minute, 7*24*time.Hour)
	authMiddleware := middleware.NewAuthMiddleware(jwtService)

	user := &domain.User{
		ID:          "test-user-uuid",
		Email:       "test@example.com",
		DisplayName: "Notification User",
		Role:        domain.RoleReader,
	}
	validToken, _, _, err := jwtService.GenerateTokenPair(user)
	require.NoError(t, err)

	notifRepo := &mockNotifRepo{
		items: []*domain.Notification{
			{
				ID:                "notif-1",
				UserID:            user.ID,
				CanonicalManhuaID: 10,
				Chapter:           25,
				Title:             "Novo Capítulo",
				Body:              "Capítulo 25 lançado",
				IsRead:            false,
				CreatedAt:         time.Now(),
			},
		},
	}

	mockSchedRunner := func(ctx context.Context) (*scheduler.CheckResult, error) {
		return &scheduler.CheckResult{
			LockAcquired:      true,
			UpdatedManhuas:    2,
			NotificationsSent: 3,
		}, nil
	}

	handler := handlers.NewNotificationsHandler(notifRepo, mockSchedRunner)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux, authMiddleware)

	t.Run("GET /api/notifications returns list", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/notifications", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var res []*domain.Notification
		err := json.Unmarshal(w.Body.Bytes(), &res)
		require.NoError(t, err)
		assert.Len(t, res, 1)
		assert.Equal(t, "notif-1", res[0].ID)
	})

	t.Run("GET /api/notifications/unread-count returns counter", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/notifications/unread-count", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var res map[string]int
		err := json.Unmarshal(w.Body.Bytes(), &res)
		require.NoError(t, err)
		assert.Equal(t, 1, res["unreadCount"])
	})

	t.Run("PATCH /api/notifications/{id}/read marks as read", func(t *testing.T) {
		req := httptest.NewRequest("PATCH", "/api/notifications/notif-1/read", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, notifRepo.items[0].IsRead)
	})

	t.Run("POST /api/routine/run-check executes check", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/routine/run-check", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var res scheduler.CheckResult
		err := json.Unmarshal(w.Body.Bytes(), &res)
		require.NoError(t, err)
		assert.True(t, res.LockAcquired)
		assert.Equal(t, 2, res.UpdatedManhuas)
		assert.Equal(t, 3, res.NotificationsSent)
	})
}
