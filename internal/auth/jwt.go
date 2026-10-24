package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
)

var (
	ErrInvalidToken = errors.New("invalid or malformed token")
	ErrExpiredToken = errors.New("token has expired")
)

type UserClaims struct {
	UserID      string          `json:"sub"`
	Email       string          `json:"email"`
	Role        domain.UserRole `json:"role"`
	DisplayName string          `json:"name"`
	jwt.RegisteredClaims
}

type RefreshClaims struct {
	UserID string `json:"sub"`
	jwt.RegisteredClaims
}

type JWTService struct {
	secretKey        []byte
	refreshKey       []byte
	accessExpiresIn  time.Duration
	refreshExpiresIn time.Duration
}

func NewJWTService(secretKey, refreshKey string, accessExpiry, refreshExpiry time.Duration) *JWTService {
	if accessExpiry <= 0 {
		accessExpiry = 15 * time.Minute
	}
	if refreshExpiry <= 0 {
		refreshExpiry = 7 * 24 * time.Hour
	}
	return &JWTService{
		secretKey:        []byte(secretKey),
		refreshKey:       []byte(refreshKey),
		accessExpiresIn:  accessExpiry,
		refreshExpiresIn: refreshExpiry,
	}
}

// GenerateTokenPair creates both an access token and a refresh token for the user.
func (s *JWTService) GenerateTokenPair(user *domain.User) (string, string, int64, error) {
	now := time.Now()
	accessExp := now.Add(s.accessExpiresIn)
	refreshExp := now.Add(s.refreshExpiresIn)

	accessClaims := &UserClaims{
		UserID:      user.ID,
		Email:       user.Email,
		Role:        user.Role,
		DisplayName: user.DisplayName,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(accessExp),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "manhua-manager-pro",
			Subject:   user.ID,
		},
	}

	accessTokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessToken, err := accessTokenObj.SignedString(s.secretKey)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to sign access token: %w", err)
	}

	refreshClaims := &RefreshClaims{
		UserID: user.ID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(refreshExp),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "manhua-manager-pro",
			Subject:   user.ID,
		},
	}

	refreshTokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshToken, err := refreshTokenObj.SignedString(s.refreshKey)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to sign refresh token: %w", err)
	}

	expiresInSeconds := int64(s.accessExpiresIn.Seconds())
	return accessToken, refreshToken, expiresInSeconds, nil
}

// ValidateAccessToken parses and validates an HMAC access token.
func (s *JWTService) ValidateAccessToken(tokenStr string) (*UserClaims, error) {
	claims := &UserClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secretKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// ValidateRefreshToken parses and validates an HMAC refresh token.
func (s *JWTService) ValidateRefreshToken(tokenStr string) (string, error) {
	claims := &RefreshClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.refreshKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return "", ErrExpiredToken
		}
		return "", ErrInvalidToken
	}

	if !token.Valid || claims.UserID == "" {
		return "", ErrInvalidToken
	}

	return claims.UserID, nil
}
