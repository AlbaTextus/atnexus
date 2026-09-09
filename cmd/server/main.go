package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AlbaTextus/connect/internal/config"
	"github.com/AlbaTextus/connect/internal/httpapi"
	"github.com/AlbaTextus/connect/internal/store"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, err := store.NewPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	defer st.Close()
	if err := st.Ping(ctx); err != nil {
		logger.Error("database is unavailable", "error", err)
		os.Exit(1)
	}
	api, err := httpapi.New(cfg, st, logger)
	if err != nil {
		logger.Error("http api initialization failed", "error", err)
		os.Exit(1)
	}
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: api.Router(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: cfg.HTTPRequestTimeout, WriteTimeout: cfg.HTTPRequestTimeout, IdleTimeout: 60 * time.Second}
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("oauth server listening", "addr", cfg.HTTPAddr, "issuer", cfg.IssuerURL)
		serverErr <- server.ListenAndServe()
	}()
	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); runWalletSettlements(signalCtx, st, logger) }()
	defer func() { stop(); <-workerDone }()
	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server stopped", "error", err)
			os.Exit(1)
		}
	case <-signalCtx.Done():
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.HTTPShutdownTimeout)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	fmt.Println("oauth server stopped")
}
