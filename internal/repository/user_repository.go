package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user with this email already exists")
)

type UserRepository interface {
	CreateUser(ctx context.Context, user *domain.User) error
	GetUserByID(ctx context.Context, id string) (*domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	GetUserByGoogleID(ctx context.Context, googleID string) (*domain.User, error)
	UpsertGoogleUser(ctx context.Context, googleID, email, name, avatarURL string) (*domain.User, error)
	DeleteUser(ctx context.Context, id string) error
}

type PostgresUserRepository struct {
	db *sql.DB
}

func NewPostgresUserRepository(db *sql.DB) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

func (r *PostgresUserRepository) CreateUser(ctx context.Context, user *domain.User) error {
	query := `
		INSERT INTO users (email, password_hash, google_id, display_name, avatar_url, auth_provider, role)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at;
	`

	var passwordHash sql.NullString
	if user.PasswordHash != nil {
		passwordHash = sql.NullString{String: *user.PasswordHash, Valid: true}
	}

	var googleID sql.NullString
	if user.GoogleID != nil {
		googleID = sql.NullString{String: *user.GoogleID, Valid: true}
	}

	var avatarURL sql.NullString
	if user.AvatarURL != nil {
		avatarURL = sql.NullString{String: *user.AvatarURL, Valid: true}
	}

	err := r.db.QueryRowContext(ctx, query,
		user.Email,
		passwordHash,
		googleID,
		user.DisplayName,
		avatarURL,
		string(user.AuthProvider),
		string(user.Role),
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

func (r *PostgresUserRepository) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, google_id, display_name, avatar_url, auth_provider, role, created_at, updated_at
		FROM users
		WHERE id = $1;
	`
	return r.scanUserRow(r.db.QueryRowContext(ctx, query, id))
}

func (r *PostgresUserRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, google_id, display_name, avatar_url, auth_provider, role, created_at, updated_at
		FROM users
		WHERE email = $1;
	`
	return r.scanUserRow(r.db.QueryRowContext(ctx, query, email))
}

func (r *PostgresUserRepository) GetUserByGoogleID(ctx context.Context, googleID string) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, google_id, display_name, avatar_url, auth_provider, role, created_at, updated_at
		FROM users
		WHERE google_id = $1;
	`
	return r.scanUserRow(r.db.QueryRowContext(ctx, query, googleID))
}

func (r *PostgresUserRepository) UpsertGoogleUser(ctx context.Context, googleID, email, name, avatarURL string) (*domain.User, error) {
	query := `
		INSERT INTO users (email, google_id, display_name, avatar_url, auth_provider, role)
		VALUES ($1, $2, $3, $4, 'google', 'reader')
		ON CONFLICT (email) DO UPDATE SET
			google_id = EXCLUDED.google_id,
			display_name = CASE WHEN users.display_name = '' THEN EXCLUDED.display_name ELSE users.display_name END,
			avatar_url = COALESCE(EXCLUDED.avatar_url, users.avatar_url),
			updated_at = CURRENT_TIMESTAMP
		RETURNING id, email, password_hash, google_id, display_name, avatar_url, auth_provider, role, created_at, updated_at;
	`

	var avatarVal sql.NullString
	if avatarURL != "" {
		avatarVal = sql.NullString{String: avatarURL, Valid: true}
	}

	row := r.db.QueryRowContext(ctx, query, email, googleID, name, avatarVal)
	return r.scanUserRow(row)
}

func (r *PostgresUserRepository) DeleteUser(ctx context.Context, id string) error {
	query := `DELETE FROM users WHERE id = $1;`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *PostgresUserRepository) scanUserRow(row *sql.Row) (*domain.User, error) {
	var user domain.User
	var passwordHash sql.NullString
	var googleID sql.NullString
	var avatarURL sql.NullString
	var authProvider string
	var role string

	err := row.Scan(
		&user.ID,
		&user.Email,
		&passwordHash,
		&googleID,
		&user.DisplayName,
		&avatarURL,
		&authProvider,
		&role,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to scan user: %w", err)
	}

	if passwordHash.Valid {
		user.PasswordHash = &passwordHash.String
	}
	if googleID.Valid {
		user.GoogleID = &googleID.String
	}
	if avatarURL.Valid {
		user.AvatarURL = &avatarURL.String
	}
	user.AuthProvider = domain.AuthProvider(authProvider)
	user.Role = domain.UserRole(role)

	return &user, nil
}
