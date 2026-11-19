// Package db opens Postgres, applies SQL migrations, and runs queries.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// ErrNotUpdated means a conditional write matched no row.
var ErrNotUpdated = errorsSentinel("not updated")

type errorsSentinel string

func (e errorsSentinel) Error() string { return string(e) }

// Querier is the subset of pgx used by queries. A pool and a transaction both implement it.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Store is a connection pool.
type Store struct {
	Pool *pgxpool.Pool
}

// NewStore wraps an existing pool. Tests use this so they can share one database.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool}
}

// Connect pings Postgres and returns a pool.
func Connect(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Store{Pool: pool}, nil
}

// Close releases the pool.
func (s *Store) Close() {
	s.Pool.Close()
}

// Begin starts a transaction.
func (s *Store) Begin(ctx context.Context) (pgx.Tx, error) {
	return s.Pool.Begin(ctx)
}

// Migrate applies SQL files in dir with goose.
func Migrate(databaseURL, dir string) error {
	sqlDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open for migrate: %w", err)
	}
	defer sqlDB.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.Up(sqlDB, dir); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// Seed runs a SQL script. The script is expected to be idempotent.
func Seed(ctx context.Context, databaseURL, path string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read seed: %w", err)
	}
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("seed connect: %w", err)
	}
	defer conn.Close(ctx)

	if _, err := conn.PgConn().Exec(ctx, string(body)).ReadAll(); err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	return nil
}
