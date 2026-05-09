package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
	"github.com/pressly/goose/v3"
)

const (
	sqliteDialect = "sqlite3"
	migrationsDir = "migrations"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Open(ctx context.Context, path string) (*sql.DB, error) {
	db, err := sql.Open(sqliteDialect, path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect(sqliteDialect); err != nil {
		db.Close()
		return nil, fmt.Errorf("set migration dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply sqlite migrations: %w", err)
	}
	return db, nil
}
