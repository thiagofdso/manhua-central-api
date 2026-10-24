package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrEmailAlreadyExists = errors.New("email is already registered")
	ErrWeakPassword       = errors.New("password must be at least 8 characters long")
	ErrInvalidEmail       = errors.New("invalid email address")
)

type AuthService struct {
	userRepo       repository.UserRepository
	jwtService     *JWTService
	googleVerifier GoogleTokenVerifier
}

func NewAuthService(
	userRepo repository.UserRepository,
	jwtService *JWTService,
	googleVerifier GoogleTokenVerifier,
) *AuthService {
	return &AuthService{
		userRepo:       userRepo,
		jwtService:     jwtService,
		googleVerifier: googleVerifier,
	}
}

func (s *AuthService) Register(ctx context.Context, dto domain.RegisterDTO) (*domain.AuthResponse, error) {
	email := strings.TrimSpace(strings.ToLower(dto.Email))
	if email == "" || !strings.Contains(email, "@") {
		return nil, ErrInvalidEmail
	}

	if len(dto.Password) < 8 {
		return nil, ErrWeakPassword
	}

	displayName := strings.TrimSpace(dto.DisplayName)
	if displayName == "" {
		displayName = strings.Split(email, "@")[0]
	}

	// Check if user already exists
	existing, _ := s.userRepo.GetUserByEmail(ctx, email)
	if existing != nil {
		return nil, ErrEmailAlreadyExists
	}

	hash, err := HashPassword(dto.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	user := &domain.User{
		Email:        email,
		PasswordHash: &hash,
		DisplayName:  displayName,
		AuthProvider: domain.ProviderEmail,
		Role:         domain.RoleReader,
	}

	if err := s.userRepo.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to save user: %w", err)
	}

	access, refresh, exp, err := s.jwtService.GenerateTokenPair(user)
	if err != nil {
		return nil, err
	}

	return &domain.AuthResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    exp,
		User:         user,
	}, nil
}

func (s *AuthService) Login(ctx context.Context, dto domain.LoginDTO) (*domain.AuthResponse, error) {
	email := strings.TrimSpace(strings.ToLower(dto.Email))
	user, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if user.PasswordHash == nil {
		return nil, ErrInvalidCredentials
	}

	valid, err := VerifyPassword(dto.Password, *user.PasswordHash)
	if err != nil || !valid {
		return nil, ErrInvalidCredentials
	}

	access, refresh, exp, err := s.jwtService.GenerateTokenPair(user)
	if err != nil {
		return nil, err
	}

	return &domain.AuthResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    exp,
		User:         user,
	}, nil
}

func (s *AuthService) GoogleAuth(ctx context.Context, idToken string) (*domain.AuthResponse, error) {
	payload, err := s.googleVerifier.Verify(ctx, idToken)
	if err != nil {
		return nil, err
	}

	name := payload.Name
	if name == "" {
		name = strings.Split(payload.Email, "@")[0]
	}

	user, err := s.userRepo.UpsertGoogleUser(ctx, payload.Sub, payload.Email, name, payload.Picture)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert google user: %w", err)
	}

	access, refresh, exp, err := s.jwtService.GenerateTokenPair(user)
	if err != nil {
		return nil, err
	}

	return &domain.AuthResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    exp,
		User:         user,
	}, nil
}

func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*domain.AuthResponse, error) {
	userID, err := s.jwtService.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, err
	}

	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	access, refresh, exp, err := s.jwtService.GenerateTokenPair(user)
	if err != nil {
		return nil, err
	}

	return &domain.AuthResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    exp,
		User:         user,
	}, nil
}

func (s *AuthService) GetProfile(ctx context.Context, userID string) (*domain.User, error) {
	return s.userRepo.GetUserByID(ctx, userID)
}

func (s *AuthService) DeleteAccount(ctx context.Context, userID string) error {
	return s.userRepo.DeleteUser(ctx, userID)
}
