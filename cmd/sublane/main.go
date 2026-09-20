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
	"path/filepath"
	"syscall"
	"time"

	"github.com/murongg/SubLane/internal/accounts"
	"github.com/murongg/SubLane/internal/apikey"
	"github.com/murongg/SubLane/internal/audit"
	"github.com/murongg/SubLane/internal/auth"
	"github.com/murongg/SubLane/internal/config"
	"github.com/murongg/SubLane/internal/gateway"
	"github.com/murongg/SubLane/internal/groups"
	"github.com/murongg/SubLane/internal/oauth"
	"github.com/murongg/SubLane/internal/server"
	"github.com/murongg/SubLane/internal/storage"
	"github.com/murongg/SubLane/internal/upstream"
	"github.com/murongg/SubLane/internal/versions"
	"github.com/murongg/SubLane/web"
)

var version = "0.1.0-dev"

func main() {
	if err := run(); err != nil {
		slog.Error("SubLane stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(version)
		return nil
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	if len(os.Args) > 1 {
		if len(os.Args) != 3 || os.Args[1] != "reset-admin-password" || os.Args[2] != "--password-stdin" {
			return errors.New("usage: sublane [--version | reset-admin-password --password-stdin]")
		}
		info, err := os.Stdin.Stat()
		if err != nil || info.Mode()&os.ModeCharDevice != 0 {
			return errors.New("supply the new password through a pipe or redirected file; terminal input is not accepted")
		}
		if err := recoverPassword(context.Background(), cfg.DataDir, os.Stdin); err != nil {
			return err
		}
		fmt.Println("Administrator password reset. All administrator browser sessions were revoked; API keys are unchanged.")
		return nil
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := storage.Open(ctx, filepath.Join(cfg.DataDir, "sublane.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	authentication, err := auth.New(db)
	if err != nil {
		return err
	}
	cipher, err := openVault(ctx, db, cfg.DataDir)
	if err != nil {
		return err
	}
	keys := apikey.New(db, cipher)
	if err := keys.Verify(ctx); err != nil {
		return fmt.Errorf("verify API key secrets: %w", err)
	}
	subscriptions := accounts.New(db, cipher)
	if err := subscriptions.Verify(ctx); err != nil {
		return fmt.Errorf("verify upstream credentials: %w", err)
	}
	releases := versions.NewReleases(nil)
	defer releases.Close()
	codexVersions, err := versions.New(ctx, db, upstream.DefaultCodexVersion, releases.Latest)
	if err != nil {
		return err
	}
	codexVersions.Start()
	defer codexVersions.Close()
	provider := upstream.NewWithVersion(codexVersions.Current)
	defer provider.Close()
	if err := provider.Start(ctx); err != nil {
		return err
	}
	forwarding := gateway.New(ctx, db, subscriptions, provider)
	defer forwarding.Close()
	authorization := oauth.New(subscriptions, provider)
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.New(server.Options{CodexVersions: codexVersions, Audit: audit.New(db), Groups: groups.New(db), Assets: web.Assets(), Version: version, StartedAt: time.Now(), Ping: db.PingContext, Auth: authentication, Keys: keys, PublicURL: cfg.PublicURL, Accounts: subscriptions, OAuth: authorization, Gateway: forwarding}),
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	failed := make(chan error, 1)
	go func() { failed <- srv.ListenAndServe() }()
	logger.Info("SubLane starting", "address", cfg.Addr, "version", version)
	select {
	case err := <-failed:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			_ = srv.Close()
			return err
		}
	}
	logger.Info("SubLane stopped")
	return nil
}
