package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// migrations holds the SQL migration files compiled into the binary.
// The path must match the actual directory structure relative to this file.
//
//go:embed migrations/*.sql
var migrations embed.FS

// RunMigrations applies all pending Goose migrations at service startup.
// Migrations are embedded in the binary — no filesystem path required.
// Safe to call on every startup; Goose is idempotent.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	connStr := pool.Config().ConnConfig.ConnString()

	sqlDB, err := sql.Open("pgx", connStr)
	if err != nil {
		return fmt.Errorf("migrate: open stdlib connection: %w", err)
	}
	defer sqlDB.Close()

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("migrate: ping: %w", err)
	}

	goose.SetBaseFS(migrations)
	goose.SetVerbose(false)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("migrate: set dialect: %w", err)
	}

	if err := goose.UpContext(ctx, sqlDB, "migrations"); err != nil {
		return fmt.Errorf("migrate: up: %w", err)
	}

	return nil
}
