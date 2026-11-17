package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
	sessions := oauth.NewSessions(st, cfg.SessionCookieName)
	imageKeys, err := oauth.NewImageKeys(ctx, cfg.DatabaseURL, sessions)
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
	api.SetImageKeys(imageKeys)
	hub, err := athub.New(context.Background(), hubConfig, logger)
	if err != nil {
		return fmt.Errorf("ATHub initialization failed: %w", err)
	}
	defer hub.Close()
	// Full legacy-history verification can outlast the connection setup deadline.
	logger.Info("checking legacy creator data", "timeout", cfg.CreatplatMigrationCheckTimeout)
	migrationCtx, migrationCancel := context.WithTimeout(context.Background(), cfg.CreatplatMigrationCheckTimeout)
	err = creatorDB.CheckLegacyMigration(migrationCtx, cfg.DatabaseURL)
	migrationCancel()
	if err != nil {
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
	// One port for reverse proxies: each module keeps its own handler chain and
	// deadline, so a request behaves the same here as on the module's port.
	gateway := server.NewGateway(
		server.GatewayRoute{Name: "oauth", Prefixes: []string{"/api/v1", "/oauth", "/.well-known"}, Handler: oauthServer.Handler, Timeout: oauthServer.ReadTimeout},
		server.GatewayRoute{Name: "image", Prefixes: []string{"/api/v2", "/file"}, Handler: imageServer.Handler, Timeout: imageServer.ReadTimeout},
		server.GatewayRoute{Name: "creatplat", Prefixes: []string{"/api/v4"}, Handler: creatorServer.Handler, Timeout: creatorServer.ReadTimeout},
		server.GatewayRoute{Name: "athub", Prefixes: []string{"/api/v5"}, Handler: athubServer.Handler, Timeout: athubServer.ReadTimeout},
	)
	gatewayTimeout := max(requestTimeout, cfg.ImageHTTPTimeout)
	gatewayServer := &http.Server{Addr: cfg.GatewayHTTPAddr, Handler: gateway, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: gatewayTimeout, WriteTimeout: gatewayTimeout, IdleTimeout: 60 * time.Second}
	servers := []*http.Server{oauthServer, imageServer, creatorServer, athubServer, gatewayServer}
	if cfg.ImageLegacyHTTPAddr != "" {
		servers = append(servers, &http.Server{Addr: cfg.ImageLegacyHTTPAddr, Handler: images.Files(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: cfg.ImageHTTPTimeout, WriteTimeout: cfg.ImageHTTPTimeout, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10})
	}
	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go st.RunExpiryPurge(signalCtx, logger)
	if err := server.Run(signalCtx, cfg.HTTPShutdownTimeout, logger, servers...); err != nil {
		return err
	}
	logger.Info("server stopped")
	return nil
}
