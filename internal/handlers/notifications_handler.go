package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/middleware"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/scheduler"
)

type RoutineRunner func(ctx context.Context) (*scheduler.CheckResult, error)

type NotificationsHandler struct {
	notifRepo domain.NotificationRepository
	runner    RoutineRunner
}

func NewNotificationsHandler(notifRepo domain.NotificationRepository, runner RoutineRunner) *NotificationsHandler {
	return &NotificationsHandler{
		notifRepo: notifRepo,
		runner:    runner,
	}
}

func (h *NotificationsHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *middleware.AuthMiddleware) {
	mux.Handle("GET /api/notifications", authMiddleware.RequireAuth(http.HandlerFunc(h.ListNotifications)))
	mux.Handle("GET /api/notifications/unread-count", authMiddleware.RequireAuth(http.HandlerFunc(h.GetUnreadCount)))
	mux.Handle("PATCH /api/notifications/{id}/read", authMiddleware.RequireAuth(http.HandlerFunc(h.MarkAsRead)))
	mux.Handle("DELETE /api/notifications/{id}", authMiddleware.RequireAuth(http.HandlerFunc(h.Dismiss)))
	mux.Handle("DELETE /api/notifications", authMiddleware.RequireAuth(http.HandlerFunc(h.DismissAll)))
	mux.Handle("POST /api/routine/run-check", http.HandlerFunc(h.RunCheck))
}

func (h *NotificationsHandler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		sendJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	items, err := h.notifRepo.ListByUser(r.Context(), userID)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "failed to list notifications")
		return
	}

	if items == nil {
		items = []*domain.Notification{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func (h *NotificationsHandler) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		sendJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	count, err := h.notifRepo.GetUnreadCount(r.Context(), userID)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "failed to get unread count")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"unreadCount": count})
}

func (h *NotificationsHandler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		sendJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "notification id is required")
		return
	}

	if err := h.notifRepo.MarkAsRead(r.Context(), id, userID); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "failed to mark notification as read")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *NotificationsHandler) Dismiss(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		sendJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		sendJSONError(w, http.StatusBadRequest, "notification id is required")
		return
	}

	if err := h.notifRepo.Dismiss(r.Context(), id, userID); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "failed to dismiss notification")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *NotificationsHandler) DismissAll(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		sendJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := h.notifRepo.DismissAll(r.Context(), userID); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "failed to dismiss all notifications")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *NotificationsHandler) RunCheck(w http.ResponseWriter, r *http.Request) {
	if h.runner == nil {
		sendJSONError(w, http.StatusNotImplemented, "scheduler routine runner not configured")
		return
	}

	res, err := h.runner(r.Context())
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
