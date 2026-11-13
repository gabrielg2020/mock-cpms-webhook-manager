package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/config"
	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/database"
	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/seed"
	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/web"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, cfg.DatabasePath)
	if err != nil {
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	if err := database.Migrate(ctx, db); err != nil {
		logger.Error("failed to migrate database", "error", err)
		os.Exit(1)
	}
	logger.Info("database ready", "path", cfg.DatabasePath)

	if cfg.SeedDemoData {
		passwords := seed.Passwords{Admin: cfg.SeedAdminPassword, Member: cfg.SeedMemberPassword}
		if err := seed.DemoData(ctx, db, passwords); err != nil {
			logger.Error("failed to seed demo data", "error", err)
			os.Exit(1)
		}
		logger.Info("demo data ready")
	}

	app, err := web.New(logger, db.PingContext)
	if err != nil {
		logger.Error("failed to set up the web application", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           app.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", cfg.ListenAddr, "environment", cfg.Environment)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		logger.Error("server failed", "error", err)
		os.Exit(1)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("server stopped")
}
