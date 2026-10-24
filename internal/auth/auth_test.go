package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/auth"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/migrations"
)

type mockGoogleVerifier struct {
	payload *auth.GoogleTokenPayload
	err     error
}

func (m *mockGoogleVerifier) Verify(ctx context.Context, tokenString string) (*auth.GoogleTokenPayload, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.payload, nil
}

func setupTestDB(t *testing.T) (*repository.PostgresDB, func()) {
	t.Helper()

	ctx := context.Background()
	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("manhua_auth_test"),
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

	cleanup := func() {
		pgDB.Close()
		_ = pgContainer.Terminate(ctx)
	}

	return pgDB, cleanup
}

func TestPasswordHashingArgon2id(t *testing.T) {
	password := "SecurePassword123!"

	hash, err := auth.HashPassword(password)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.Contains(t, hash, "$argon2id$v=19$m=65536,t=3,p=2$")

	// Verify correct password
	match, err := auth.VerifyPassword(password, hash)
	require.NoError(t, err)
	assert.True(t, match)

	// Verify wrong password
	match, err = auth.VerifyPassword("WrongPassword123!", hash)
	require.NoError(t, err)
	assert.False(t, match)

	// Verify malformed hash
	_, err = auth.VerifyPassword(password, "invalid-hash-string")
	assert.ErrorIs(t, err, auth.ErrInvalidHash)
}

func TestJWTTokenService(t *testing.T) {
	jwtService := auth.NewJWTService("secret-key-32-chars-minimum-long!", "refresh-key-32-chars-minimum-long", 1*time.Second, 2*time.Second)

	user := &domain.User{
		ID:          "11111111-2222-3333-4444-555555555555",
		Email:       "reader@example.com",
		DisplayName: "Solo Reader",
		Role:        domain.RoleReader,
	}

	accessToken, refreshToken, exp, err := jwtService.GenerateTokenPair(user)
	require.NoError(t, err)
	assert.NotEmpty(t, accessToken)
	assert.NotEmpty(t, refreshToken)
	assert.Equal(t, int64(1), exp)

	// Validate Access Token
	claims, err := jwtService.ValidateAccessToken(accessToken)
	require.NoError(t, err)
	assert.Equal(t, user.ID, claims.UserID)
	assert.Equal(t, user.Email, claims.Email)
	assert.Equal(t, user.Role, claims.Role)
	assert.Equal(t, user.DisplayName, claims.DisplayName)

	// Validate Refresh Token
	userID, err := jwtService.ValidateRefreshToken(refreshToken)
	require.NoError(t, err)
	assert.Equal(t, user.ID, userID)

	// Wait for expiration
	time.Sleep(1100 * time.Millisecond)

	_, err = jwtService.ValidateAccessToken(accessToken)
	assert.ErrorIs(t, err, auth.ErrExpiredToken)
}

func TestAuthServiceIntegration(t *testing.T) {
	pgDB, cleanup := setupTestDB(t)
	defer cleanup()

	userRepo := repository.NewPostgresUserRepository(pgDB.DB)
	jwtService := auth.NewJWTService("secret-jwt-key-minimum-32-bytes!!", "refresh-jwt-key-minimum-32-bytes", 15*time.Minute, 7*24*time.Hour)
	googleMock := &mockGoogleVerifier{
		payload: &auth.GoogleTokenPayload{
			Sub:     "google-sub-99999",
			Email:   "google.user@example.com",
			Name:    "Google User",
			Picture: "https://lh3.googleusercontent.com/avatar.jpg",
		},
	}

	authService := auth.NewAuthService(userRepo, jwtService, googleMock)
	ctx := context.Background()

	// 1. Test Register
	regDTO := domain.RegisterDTO{
		Email:       "NEW.USER@EXAMPLE.COM", // test lowercase normalization
		Password:    "password123",
		DisplayName: "Newbie",
	}
	authResp, err := authService.Register(ctx, regDTO)
	require.NoError(t, err)
	require.NotNil(t, authResp)
	assert.Equal(t, "new.user@example.com", authResp.User.Email)
	assert.Equal(t, "Newbie", authResp.User.DisplayName)
	assert.NotEmpty(t, authResp.AccessToken)

	// Test Duplicate Email Register
	_, err = authService.Register(ctx, regDTO)
	assert.ErrorIs(t, err, auth.ErrEmailAlreadyExists)

	// 2. Test Login
	loginDTO := domain.LoginDTO{
		Email:    "new.user@example.com",
		Password: "password123",
	}
	loginResp, err := authService.Login(ctx, loginDTO)
	require.NoError(t, err)
	assert.Equal(t, authResp.User.ID, loginResp.User.ID)

	// Test Wrong Password
	_, err = authService.Login(ctx, domain.LoginDTO{Email: "new.user@example.com", Password: "wrong-password"})
	assert.ErrorIs(t, err, auth.ErrInvalidCredentials)

	// 3. Test Google Auth (First login creates user)
	gResp, err := authService.GoogleAuth(ctx, "valid-mock-google-token")
	require.NoError(t, err)
	assert.Equal(t, "google.user@example.com", gResp.User.Email)
	assert.Equal(t, domain.ProviderGoogle, gResp.User.AuthProvider)
	assert.NotNil(t, gResp.User.GoogleID)
	assert.Equal(t, "google-sub-99999", *gResp.User.GoogleID)

	// Test Google Auth (Second login updates/upserts user)
	gResp2, err := authService.GoogleAuth(ctx, "valid-mock-google-token")
	require.NoError(t, err)
	assert.Equal(t, gResp.User.ID, gResp2.User.ID)

	// 4. Test Refresh Token
	refResp, err := authService.RefreshToken(ctx, loginResp.RefreshToken)
	require.NoError(t, err)
	assert.NotEmpty(t, refResp.AccessToken)
	assert.Equal(t, authResp.User.ID, refResp.User.ID)

	// 5. Test Delete Account (Mandatory Google Play)
	err = authService.DeleteAccount(ctx, authResp.User.ID)
	require.NoError(t, err)

	// Verify user is gone
	_, err = authService.GetProfile(ctx, authResp.User.ID)
	assert.ErrorIs(t, err, repository.ErrUserNotFound)
}
