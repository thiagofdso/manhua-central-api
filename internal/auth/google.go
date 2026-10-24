package auth

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/api/idtoken"
)

var (
	ErrInvalidGoogleToken = errors.New("invalid or expired google id token")
)

type GoogleTokenPayload struct {
	Sub     string
	Email   string
	Name    string
	Picture string
}

type GoogleTokenVerifier interface {
	Verify(ctx context.Context, tokenString string) (*GoogleTokenPayload, error)
}

type GoogleVerifier struct {
	clientID string
}

func NewGoogleVerifier(clientID string) *GoogleVerifier {
	return &GoogleVerifier{clientID: clientID}
}

func (v *GoogleVerifier) Verify(ctx context.Context, tokenString string) (*GoogleTokenPayload, error) {
	if tokenString == "" {
		return nil, ErrInvalidGoogleToken
	}

	payload, err := idtoken.Validate(ctx, tokenString, v.clientID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidGoogleToken, err)
	}

	email, _ := payload.Claims["email"].(string)
	name, _ := payload.Claims["name"].(string)
	picture, _ := payload.Claims["picture"].(string)

	if payload.Subject == "" || email == "" {
		return nil, ErrInvalidGoogleToken
	}

	return &GoogleTokenPayload{
		Sub:     payload.Subject,
		Email:   email,
		Name:    name,
		Picture: picture,
	}, nil
}
