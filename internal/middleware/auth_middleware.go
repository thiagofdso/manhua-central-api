package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/auth"
)

type contextKey string

const (
	UserClaimsContextKey contextKey = "user_claims"
)

type AuthMiddleware struct {
	jwtService *auth.JWTService
}

func NewAuthMiddleware(jwtService *auth.JWTService) *AuthMiddleware {
	return &AuthMiddleware{jwtService: jwtService}
}

func (m *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeUnauthorized(w, "missing authorization header")
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeUnauthorized(w, "invalid authorization header format")
			return
		}

		tokenStr := parts[1]
		claims, err := m.jwtService.ValidateAccessToken(tokenStr)
		if err != nil {
			writeUnauthorized(w, err.Error())
			return
		}

		ctx := context.WithValue(r.Context(), UserClaimsContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetClaimsFromContext(ctx context.Context) (*auth.UserClaims, bool) {
	claims, ok := ctx.Value(UserClaimsContextKey).(*auth.UserClaims)
	return claims, ok
}

func GetUserIDFromContext(ctx context.Context) (string, bool) {
	claims, ok := GetClaimsFromContext(ctx)
	if !ok || claims == nil {
		return "", false
	}
	return claims.UserID, true
}

func writeUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}
