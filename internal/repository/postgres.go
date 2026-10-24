package repository

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"
)

type PostgresDB struct {
	DB *sql.DB
}

// NewPostgresDB initializes a connection pool to PostgreSQL.
func NewPostgresDB(databaseURL string) (*PostgresDB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &PostgresDB{DB: db}, nil
}

// RunMigrationsFS executes pending SQL migrations using an embedded filesystem.
func (p *PostgresDB) RunMigrationsFS(migrationsFS fs.FS, dir string) error {
	driver, err := postgres.WithInstance(p.DB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("could not create postgres driver instance: %w", err)
	}

	sourceDriver, err := iofs.New(migrationsFS, dir)
	if err != nil {
		return fmt.Errorf("could not create iofs source driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", driver)
	if err != nil {
		return fmt.Errorf("could not initialize migration engine: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration up failed: %w", err)
	}

	return nil
}

// Ping checks if the PostgreSQL connection is active.
func (p *PostgresDB) Ping(ctx context.Context) error {
	return p.DB.PingContext(ctx)
}

// Close terminates the PostgreSQL connection pool.
func (p *PostgresDB) Close() error {
	return p.DB.Close()
}
