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
	"syscall"
	"time"

	"github.com/drywaters/permitpal/internal/config"
	"github.com/drywaters/permitpal/internal/metrics"
	"github.com/drywaters/permitpal/internal/repository"
	"github.com/drywaters/permitpal/internal/server"
	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	if err := run(context.Background()); err != nil {
		slog.Error("permitpal stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel(cfg)}))
	slog.SetDefault(logger)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, closeStore, err := repository.NewStore(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize store: %w", err)
	}
	defer closeStore()

	srv := server.New(cfg, store, logger).HTTPServer()
	// Bind the public port before the optional metrics listener, so a port
	// collision disables metrics instead of stopping PermitPal.
	listener, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("server failed: %w", err)
	}

	var poolCollectors []prometheus.Collector
	if pg, ok := store.(*repository.PostgresStore); ok {
		poolCollectors = append(poolCollectors, metrics.NewPoolCollector(pg.Pool()))
	}
	// The metrics listener is internal and separate from the public router, so
	// Traefik's public routes cannot reach /metrics.
	metricsSrv := metrics.Start(cfg.MetricsAddr, metrics.NewRegistry(poolCollectors...))

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("permitpal listening", "port", cfg.Port, "data_store", cfg.DataStore)
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case <-ctx.Done():
	case err := <-serverErr:
		logger.Error("server failed", "error", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown failed", "error", err)
	}
	if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("metrics shutdown failed", "error", err)
	}
	return nil
}

func slogLevel(cfg *config.Config) slog.Level {
	if cfg == nil {
		return slog.LevelInfo
	}
	switch cfg.LogLevel {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
