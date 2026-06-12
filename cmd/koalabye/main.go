package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/koalastuff/koalabye/internal/app"
	"github.com/koalastuff/koalabye/internal/config"
	"github.com/koalastuff/koalabye/internal/version"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	build := version.Current()
	slog.Info("starting KoalaBye", "version", build.Version, "commit", build.Commit, "build_date", build.BuildDate)
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	application, err := app.New(context.Background(), cfg)
	if err != nil {
		slog.Error("start application", "error", err)
		os.Exit(1)
	}
	defer application.Close()

	server := application.Server()
	errs := make(chan error, 1)
	go func() {
		slog.Info("KoalaBye listening", "address", cfg.ListenAddr, "mode", cfg.Mode)
		errs <- server.ListenAndServe()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case signal := <-signals:
		slog.Info("shutting down", "signal", signal.String())
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server stopped", "error", err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown", "error", err)
	}
}
