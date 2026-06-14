package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/koalastuff/koalabye/migrations"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func Open(ctx context.Context, path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	dsn := "file:" + filepath.ToSlash(path) + "?_txlock=immediate"
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	database.SetMaxOpenConns(10)
	database.SetConnMaxLifetime(0)

	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := database.ExecContext(ctx, pragma); err != nil {
			database.Close()
			return nil, fmt.Errorf("configure sqlite (%s): %w", pragma, err)
		}
	}
	if err := database.PingContext(ctx); err != nil {
		database.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return database, nil
}

var migrateMu sync.Mutex

func Migrate(database *sql.DB) error {
	migrateMu.Lock()
	defer migrateMu.Unlock()
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set migration dialect: %w", err)
	}
	if err := goose.Up(database, "."); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	if err := ensureSchema(database); err != nil {
		return fmt.Errorf("ensure schema: %w", err)
	}
	return nil
}

func ensureSchema(database *sql.DB) error {
	ctx := context.Background()
	missing, err := missingColumns(ctx, database, "campaign_branding",
		"public_heading", "public_intro")
	if err != nil {
		return err
	}
	for _, col := range missing {
		if _, err := database.ExecContext(ctx,
			fmt.Sprintf("ALTER TABLE campaign_branding ADD COLUMN %s TEXT NULL", col)); err != nil {
			return fmt.Errorf("add column %s: %w", col, err)
		}
	}
	return nil
}

func missingColumns(ctx context.Context, database *sql.DB, table string, cols ...string) ([]string, error) {
	var missing []string
	for _, col := range cols {
		var n int
		if err := database.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?", table, col).Scan(&n); err != nil {
			return nil, err
		}
		if n == 0 {
			missing = append(missing, col)
		}
	}
	return missing, nil
}

func Now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
