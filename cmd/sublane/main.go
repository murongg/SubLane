package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/murongg/SubLane/internal/config"
	"github.com/murongg/SubLane/internal/server"
	"github.com/murongg/SubLane/internal/storage"
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
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := storage.Open(ctx, filepath.Join(cfg.DataDir, "sublane.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.New(server.Options{Assets: web.Assets(), Version: version, StartedAt: time.Now(), Ping: db.PingContext}),
		ReadHeaderTimeout: 5 * time.Second,
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
