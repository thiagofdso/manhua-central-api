package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/auth"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/handlers"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/middleware"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/migrations"
)

type mockGoogleVerifier struct{}

func (m *mockGoogleVerifier) Verify(ctx context.Context, tokenString string) (*auth.GoogleTokenPayload, error) {
	if tokenString == "valid-google-token" {
		return &auth.GoogleTokenPayload{
			Sub:     "google-sub-handler-test",
			Email:   "google.handler@example.com",
			Name:    "Google Handler",
			Picture: "https://example.com/avatar.png",
		}, nil
	}
	return nil, auth.ErrInvalidGoogleToken
}

func setupHandlerTest(t *testing.T) (*handlers.AuthHandler, *middleware.AuthMiddleware, func()) {
	t.Helper()

	ctx := context.Background()
	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("manhua_handler_test"),
		tcpostgres.WithUsername("test_user"),
		tcpostgres.WithPassword("test_pass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	require.NoError(t, err)

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pgDB, err := repository.NewPostgresDB(connStr)
	require.NoError(t, err)

	err = pgDB.RunMigrationsFS(migrations.FS, ".")
	require.NoError(t, err)

	userRepo := repository.NewPostgresUserRepository(pgDB.DB)
	jwtService := auth.NewJWTService("secret-jwt-key-minimum-32-bytes!!", "refresh-jwt-key-minimum-32-bytes", 15*time.Minute, 7*24*time.Hour)
	googleVerifier := &mockGoogleVerifier{}
	authService := auth.NewAuthService(userRepo, jwtService, googleVerifier)
	authHandler := handlers.NewAuthHandler(authService)
	authMiddleware := middleware.NewAuthMiddleware(jwtService)

	cleanup := func() {
		pgDB.Close()
		_ = pgContainer.Terminate(ctx)
	}

	return authHandler, authMiddleware, cleanup
}

func TestAuthHandlerEndpoints(t *testing.T) {
	handler, authMid, cleanup := setupHandlerTest(t)
	defer cleanup()

	// 1. Test POST /api/auth/register
	regBody, _ := json.Marshal(domain.RegisterDTO{
		Email:       "test@example.com",
		Password:    "password123",
		DisplayName: "Test User",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(regBody))
	rec := httptest.NewRecorder()
	handler.Register(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	var regResp domain.AuthResponse
	err := json.Unmarshal(rec.Body.Bytes(), &regResp)
	require.NoError(t, err)
	assert.NotEmpty(t, regResp.AccessToken)
	assert.Equal(t, "test@example.com", regResp.User.Email)

	// 2. Test POST /api/auth/register Duplicate Email -> 409 Conflict
	reqDup := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(regBody))
	recDup := httptest.NewRecorder()
	handler.Register(recDup, reqDup)
	assert.Equal(t, http.StatusConflict, recDup.Code)

	// 3. Test POST /api/auth/login
	loginBody, _ := json.Marshal(domain.LoginDTO{
		Email:    "test@example.com",
		Password: "password123",
	})
	reqLogin := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
	recLogin := httptest.NewRecorder()
	handler.Login(recLogin, reqLogin)

	assert.Equal(t, http.StatusOK, recLogin.Code)
	var loginResp domain.AuthResponse
	err = json.Unmarshal(recLogin.Body.Bytes(), &loginResp)
	require.NoError(t, err)
	assert.NotEmpty(t, loginResp.AccessToken)

	// 4. Test POST /api/auth/google
	googleBody, _ := json.Marshal(domain.GoogleAuthDTO{IDToken: "valid-google-token"})
	reqGoogle := httptest.NewRequest(http.MethodPost, "/api/auth/google", bytes.NewReader(googleBody))
	recGoogle := httptest.NewRecorder()
	handler.GoogleAuth(recGoogle, reqGoogle)

	assert.Equal(t, http.StatusOK, recGoogle.Code)

	// 5. Test Authenticated Route: GET /api/users/me
	reqMe := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	reqMe.Header.Set("Authorization", "Bearer "+loginResp.AccessToken)
	recMe := httptest.NewRecorder()
	authMid.RequireAuth(http.HandlerFunc(handler.GetCurrentUser)).ServeHTTP(recMe, reqMe)

	assert.Equal(t, http.StatusOK, recMe.Code)
	var meUser domain.User
	err = json.Unmarshal(recMe.Body.Bytes(), &meUser)
	require.NoError(t, err)
	assert.Equal(t, "test@example.com", meUser.Email)

	// 6. Test Authenticated Route: DELETE /api/users/me (Mandatory Google Play)
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/users/me", nil)
	reqDel.Header.Set("Authorization", "Bearer "+loginResp.AccessToken)
	recDel := httptest.NewRecorder()
	authMid.RequireAuth(http.HandlerFunc(handler.DeleteCurrentUser)).ServeHTTP(recDel, reqDel)

	assert.Equal(t, http.StatusNoContent, recDel.Code)

	// Verify token is now invalid / user deleted
	reqMeAfter := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	reqMeAfter.Header.Set("Authorization", "Bearer "+loginResp.AccessToken)
	recMeAfter := httptest.NewRecorder()
	authMid.RequireAuth(http.HandlerFunc(handler.GetCurrentUser)).ServeHTTP(recMeAfter, reqMeAfter)
	assert.Equal(t, http.StatusNotFound, recMeAfter.Code)
}
