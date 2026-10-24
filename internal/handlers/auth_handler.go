package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/auth"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/middleware"
)

type AuthHandler struct {
	authService *auth.AuthService
}

func NewAuthHandler(authService *auth.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var dto domain.RegisterDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		sendJSONError(w, http.StatusBadRequest, "invalid request payload")
		return
	}

	resp, err := h.authService.Register(r.Context(), dto)
	if err != nil {
		if errors.Is(err, auth.ErrEmailAlreadyExists) {
			sendJSONError(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, auth.ErrWeakPassword) || errors.Is(err, auth.ErrInvalidEmail) {
			sendJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		sendJSONError(w, http.StatusInternalServerError, "failed to register user")
		return
	}

	sendJSON(w, http.StatusCreated, resp)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var dto domain.LoginDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		sendJSONError(w, http.StatusBadRequest, "invalid request payload")
		return
	}

	resp, err := h.authService.Login(r.Context(), dto)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			sendJSONError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		sendJSONError(w, http.StatusInternalServerError, "login failed")
		return
	}

	sendJSON(w, http.StatusOK, resp)
}

func (h *AuthHandler) GoogleAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var dto domain.GoogleAuthDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		sendJSONError(w, http.StatusBadRequest, "invalid request payload")
		return
	}

	resp, err := h.authService.GoogleAuth(r.Context(), dto.IDToken)
	if err != nil {
		sendJSONError(w, http.StatusUnauthorized, "invalid google token")
		return
	}

	sendJSON(w, http.StatusOK, resp)
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var dto domain.RefreshDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		sendJSONError(w, http.StatusBadRequest, "invalid request payload")
		return
	}

	resp, err := h.authService.RefreshToken(r.Context(), dto.RefreshToken)
	if err != nil {
		sendJSONError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	sendJSON(w, http.StatusOK, resp)
}

func (h *AuthHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		sendJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := h.authService.GetProfile(r.Context(), userID)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, "user not found")
		return
	}

	sendJSON(w, http.StatusOK, user)
}

func (h *AuthHandler) DeleteCurrentUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		sendJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := h.authService.DeleteAccount(r.Context(), userID); err != nil {
		sendJSONError(w, http.StatusInternalServerError, "failed to delete account")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func sendJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}

func sendJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}
