package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/koalastuff/koalabye/internal/audit"
	"github.com/koalastuff/koalabye/internal/auth"
	"github.com/koalastuff/koalabye/internal/config"
	"github.com/koalastuff/koalabye/internal/dashboard"
	"github.com/koalastuff/koalabye/internal/db"
	"github.com/koalastuff/koalabye/internal/i18n"
	"github.com/koalastuff/koalabye/internal/instance"
	"github.com/koalastuff/koalabye/internal/permissions"
	"github.com/koalastuff/koalabye/internal/setup"
)

type App struct {
	Config   config.Config
	Database *sql.DB
	Handler  http.Handler
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	database, err := db.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(database); err != nil {
		database.Close()
		return nil, err
	}

	queries := db.NewQuerier(database)
	catalog, err := i18n.Load()
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("load translations: %w", err)
	}
	sessions := auth.NewSessionManager(queries, cfg.SecureCookies)
	csrf := auth.NewCSRF(cfg.Secret, cfg.SecureCookies)
	auditLogger := audit.New(queries)
	permissionService := permissions.New(queries)
	setupHandler := setup.New(cfg, queries, sessions, csrf, catalog)

	bootstrapRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.BaseURL+"/setup", nil)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("create bootstrap request: %w", err)
	}
	if err := setupHandler.Bootstrap(bootstrapRequest); err != nil {
		database.Close()
		return nil, fmt.Errorf("bootstrap owner: %w", err)
	}

	authHandler := auth.NewHandler(cfg, queries, sessions, csrf, auditLogger)
	dashboardHandler := dashboard.New(cfg, queries, permissionService)
	instanceHandler := instance.New(cfg, queries, permissionService)
	handler := Routes(cfg, database, queries, sessions, csrf, catalog, setupHandler, authHandler, dashboardHandler, instanceHandler)

	return &App{Config: cfg, Database: database, Handler: handler}, nil
}

func (a *App) Server() *http.Server {
	return &http.Server{
		Addr:              a.Config.ListenAddr,
		Handler:           a.Handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(slog.Default().Handler(), slog.LevelError),
	}
}

func (a *App) Close() error {
	return a.Database.Close()
}
