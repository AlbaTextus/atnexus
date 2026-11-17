package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/AlbaTextus/atnexus/internal/config"
	"github.com/AlbaTextus/atnexus/internal/server"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/modules/athub"
	"github.com/AlbaTextus/atnexus/modules/athub/access"
	creatorapi "github.com/AlbaTextus/atnexus/modules/creatplat/api"
	creatorstorage "github.com/AlbaTextus/atnexus/modules/creatplat/storage"
	imagebackend "github.com/AlbaTextus/atnexus/modules/image"
	"github.com/AlbaTextus/atnexus/modules/oauth"
	"github.com/AlbaTextus/atnexus/modules/oauth/api"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	// The runtime image has no shell or HTTP client, so the binary probes itself.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := healthcheck(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// All listeners start together, so the account API answering means startup finished.
func healthcheck() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	host, port, err := net.SplitHostPort(cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("invalid HTTP_ADDR: %w", err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get("http://" + net.JoinHostPort(host, port) + "/healthz")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("unhealthy: status %d", response.StatusCode)
	}
	return nil
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
	sessions := oauth.NewSessions(st, cfg.SessionCookieName)
	imageKeys, err := oauth.NewImageKeysWithPool(ctx, st.Pool(), sessions)
	if err != nil {
		return err
	}
	defer imageKeys.Close()
	images, err := imagebackend.New(ctx, imagebackend.Config{DatabaseURL: cfg.ImageDatabaseURL, LocalRoot: cfg.ImageLocalRoot, TempRoot: cfg.ImageTempRoot, PublicURL: cfg.ImagePublicURL, MaxUploadBytes: cfg.ImageMaxUploadBytes, MaxChunkBytes: cfg.ImageMaxChunkBytes, MaxChunks: cfg.ImageMaxChunks, UploadTTL: cfg.ImageUploadTTL, MaxSessionsPerOwner: cfg.ImageMaxUploadSessions, MaxConns: int32(cfg.ImageDatabaseMaxConns)}, imagebackend.Dependencies{Sessions: sessions, Keys: imageKeys, Logger: logger})
	if err != nil {
		return fmt.Errorf("image initialization failed: %w", err)
	}
	defer images.Close()
	if err := images.CheckSeparateDatabases(ctx, cfg.DatabaseURL, cfg.CreatplatDatabaseURL, cfg.ATHubDatabaseURL); err != nil {
		return err
	}
	if err := st.Ping(ctx); err != nil {
		return fmt.Errorf("database is unavailable: %w", err)
	}
	if err := st.CheckATHubSchema(ctx); err != nil {
		return err
	}
	creatorDB, err := creatorstorage.New(ctx, cfg.CreatplatDatabaseURL, st)
	if err != nil {
		return fmt.Errorf("creator database initialization failed: %w", err)
	}
	defer creatorDB.Close()
	if err := creatorDB.RequireSeparateDatabases(ctx, cfg.DatabaseURL, cfg.ATHubDatabaseURL); err != nil {
		return err
	}
	access, err := athubaccess.New(st, cfg.ATHubHTTPAddr, logger)
	if err != nil {
		return err
	}
	access.EnableReadSessions(cfg.ClientSecretEncryptionKey)
	access.EnableSessions(sessions)
	access.EnableAPIKeys(oauth.NewAPIKeys(st))
	access.EnableCreatorStorage(creatorDB)
	hubConfig := athub.Config{DatabaseURL: cfg.ATHubDatabaseURL, WrapHandler: access.Wrap, RecordChanges: access.RecordChanges}
	api, err := httpapi.New(cfg, st, logger)
	if err != nil {
		return fmt.Errorf("http api initialization failed: %w", err)
	}
	api.SetImageKeys(imageKeys)
	hub, err := athub.New(context.Background(), hubConfig, logger)
	if err != nil {
		return fmt.Errorf("ATHub initialization failed: %w", err)
	}
	defer hub.Close()
	// Startup compares only row counts; the full row-by-row digest runs in the
	// background once the listeners are up.
	if err = creatorDB.CheckLegacyMigrationCounts(ctx, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("creator migration check failed: %w", err)
	}
	creators := creatorapi.New(cfg, st, logger)
	creators.SetStorage(creatorDB)
	creators.SetATHubReviewExecutor(access.ApplyReview)
	creators.SetATHubReceiptForgetter(hub.ForgetMutation)
	creators.SetATHubSubmitter(access.SubmitUpload)
	creators.SetATHubNovelEditor(access.SubmitNovelEdit)
	creators.SetATHubIllustrationSubmitter(access.SubmitIllustration)
	creators.SetATHubIllustrationDeleter(access.DeleteIllustration)
	creators.SetATHubIllustrationLayoutEditor(access.SubmitIllustrationLayout)
	creators.SetHistoryHandler(http.HandlerFunc(access.ServeHistory))
	// ATHub owns its 15s database deadline and accepts uploads up to 8 MiB.
	requestTimeout := max(cfg.HTTPRequestTimeout, 30*time.Second)
	contentOrigins := append([]string{cfg.AuthUIURL}, cfg.WebAuthnRPOrigins...)
	oauthServer := &http.Server{Addr: cfg.HTTPAddr, Handler: server.ModuleHTTP(api.Router(), cfg.HTTPRequestTimeout, logger, contentOrigins...), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: cfg.HTTPRequestTimeout, WriteTimeout: cfg.HTTPRequestTimeout, IdleTimeout: 60 * time.Second}
	athubServer := &http.Server{Addr: cfg.ATHubHTTPAddr, Handler: server.ModuleHTTP(server.NewATHubHandler(hub.Handler(), hub.Ping), requestTimeout, logger, contentOrigins...), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: requestTimeout, WriteTimeout: requestTimeout, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	creatorServer := &http.Server{Addr: cfg.CreatplatHTTPAddr, Handler: server.ModuleHTTP(creators.Router(), requestTimeout, logger, contentOrigins...), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: requestTimeout, WriteTimeout: requestTimeout, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	// File responses and multipart requests must stream without TimeoutHandler's
	// full response buffer. Listener deadlines still bound transfers.
	imageAPI := server.ModuleHTTP(images.Router(), 0, logger, contentOrigins...)
	imageHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/file/") || strings.HasPrefix(r.URL.Path, "/api/v2/file/") {
			images.Files().ServeHTTP(w, r)
			return
		}
		imageAPI.ServeHTTP(w, r)
	})
	if cfg.ImageHTTPTimeout <= 0 {
		return fmt.Errorf("IMAGE_HTTP_TIMEOUT must be positive")
	}
	imageServer := &http.Server{Addr: cfg.ImageHTTPAddr, Handler: imageHandler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: cfg.ImageHTTPTimeout, WriteTimeout: cfg.ImageHTTPTimeout, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	servers := []*http.Server{oauthServer, imageServer, creatorServer, athubServer}
	if cfg.ImageLegacyHTTPAddr != "" {
		servers = append(servers, &http.Server{Addr: cfg.ImageLegacyHTTPAddr, Handler: images.Files(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: cfg.ImageHTTPTimeout, WriteTimeout: cfg.ImageHTTPTimeout, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10})
	}
	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	// Background work stops with the listeners and finishes before the deferred
	// pool closes run: on every return path the context is cancelled first,
	// then the work is awaited.
	var background sync.WaitGroup
	defer background.Wait()
	defer stop()
	background.Go(func() { st.RunExpiryPurge(signalCtx, logger) })
	// Edited legacy rows mean the old service is still writing: stop serving,
	// exactly as the startup check used to refuse to start.
	var legacyMismatch error
	background.Go(func() {
		logger.Info("verifying legacy creator data", "timeout", cfg.CreatplatMigrationCheckTimeout)
		checkCtx, cancel := context.WithTimeout(signalCtx, cfg.CreatplatMigrationCheckTimeout)
		defer cancel()
		err := creatorDB.CheckLegacyMigration(checkCtx, cfg.DatabaseURL)
		switch {
		case err == nil:
			logger.Info("legacy creator data verified")
		case errors.Is(err, creatorstorage.ErrLegacyDataMismatch):
			legacyMismatch = fmt.Errorf("creator migration check failed: %w", err)
			logger.Error("legacy creator data mismatch; stopping", "error", err)
			stop()
		case signalCtx.Err() == nil:
			// Could not finish (timeout, connectivity): not evidence of a mismatch.
			logger.Error("legacy creator data verification did not complete", "error", err)
		}
	})
	if err := server.Run(signalCtx, cfg.HTTPShutdownTimeout, logger, servers...); err != nil {
		return err
	}
	background.Wait()
	if legacyMismatch != nil {
		return legacyMismatch
	}
	logger.Info("server stopped")
	return nil
}
