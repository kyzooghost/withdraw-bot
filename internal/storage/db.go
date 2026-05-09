package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	_ "github.com/mattn/go-sqlite3"
	"github.com/pressly/goose/v3"
)

const (
	sqliteDialect = "sqlite3"
	migrationsDir = "migrations"
	maxOpenConns  = 1
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Open(ctx context.Context, path string) (*sql.DB, error) {
	db, err := sql.Open(sqliteDialect, path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(maxOpenConns)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}
	migrationsRoot, err := fs.Sub(migrationsFS, migrationsDir)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("open embedded migrations: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, migrationsRoot)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create sqlite migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply sqlite migrations: %w", err)
	}
	return db, nil
}
