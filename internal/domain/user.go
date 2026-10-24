package domain

import (
	"time"
)

type UserRole string

const (
	RoleAdmin  UserRole = "admin"
	RoleReader UserRole = "reader"
)

type AuthProvider string

const (
	ProviderEmail  AuthProvider = "email"
	ProviderGoogle AuthProvider = "google"
)

type User struct {
	ID           string       `json:"id"`
	Email        string       `json:"email"`
	PasswordHash *string      `json:"-"`
	GoogleID     *string      `json:"googleId,omitempty"`
	DisplayName  string       `json:"displayName"`
	AvatarURL    *string      `json:"avatarUrl,omitempty"`
	AuthProvider AuthProvider `json:"authProvider"`
	Role         UserRole     `json:"role"`
	CreatedAt    time.Time    `json:"createdAt"`
	UpdatedAt    time.Time    `json:"updatedAt"`
}

type AuthResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int64  `json:"expiresIn"`
	User         *User  `json:"user"`
}

type RegisterDTO struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
}

type LoginDTO struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type GoogleAuthDTO struct {
	IDToken string `json:"idToken"`
}

type RefreshDTO struct {
	RefreshToken string `json:"refreshToken"`
}
