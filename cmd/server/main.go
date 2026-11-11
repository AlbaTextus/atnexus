package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"albatextus/athub"
	"github.com/AlbaTextus/atnexus/internal/athubaccess"
	"github.com/AlbaTextus/atnexus/internal/config"
	"github.com/AlbaTextus/atnexus/internal/httpapi"
	"github.com/AlbaTextus/atnexus/internal/server"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, err := store.NewPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database initialization failed: %w", err)
	}
	defer st.Close()
	if err := st.Ping(ctx); err != nil {
		return fmt.Errorf("database is unavailable: %w", err)
	}
	if err := st.CheckATHubSchema(ctx); err != nil {
		return err
	}
	if err := st.CheckNovelHistorySchema(ctx); err != nil {
		return fmt.Errorf("novel history schema is not ready; apply service migration 20261110050000_novel_history: %w", err)
	}
	access, err := athubaccess.New(st, cfg.ATHubHTTPAddr, logger)
	if err != nil {
		return err
	}
	access.EnableReadSessions(cfg.ClientSecretEncryptionKey)
	hubConfig := athub.Config{DatabaseURL: cfg.ATHubDatabaseURL, WrapHandler: access.Wrap, RecordChanges: access.RecordChanges}
	if cfg.Redis.Enabled {
		if err := st.EnableRedisSessionCache(ctx, cfg.Redis.URL, cfg.Redis.KeyPrefix, cfg.Redis.SessionTTL); err != nil {
			return fmt.Errorf("redis cache initialization failed: %w", err)
		}
		logger.Info("redis session cache enabled", "ttl", cfg.Redis.SessionTTL)
	}
	api, err := httpapi.New(cfg, st, logger)
	if err != nil {
		return fmt.Errorf("http api initialization failed: %w", err)
	}
	hub, err := athub.New(context.Background(), hubConfig, logger)
	if err != nil {
		return fmt.Errorf("ATHub initialization failed: %w", err)
	}
	defer hub.Close()
	api.SetATHubReviewExecutor(access.ApplyReview)
	// ATHub owns its 15s database deadline and accepts uploads up to 8 MiB.
	requestTimeout := max(cfg.HTTPRequestTimeout, 30*time.Second)
	oauthServer := &http.Server{Addr: cfg.HTTPAddr, Handler: api.Router(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: cfg.HTTPRequestTimeout, WriteTimeout: cfg.HTTPRequestTimeout, IdleTimeout: 60 * time.Second}
	contentOrigins := append([]string{cfg.AuthUIURL}, cfg.WebAuthnRPOrigins...)
	athubServer := &http.Server{Addr: cfg.ATHubHTTPAddr, Handler: server.NewContentHandler(hub.Handler(), hub.Ping, contentOrigins...), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: requestTimeout, WriteTimeout: requestTimeout, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := server.Run(signalCtx, cfg.HTTPShutdownTimeout, logger, oauthServer, athubServer); err != nil {
		return err
	}
	logger.Info("server stopped")
	return nil
}
