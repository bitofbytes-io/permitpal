package main

import (
	"context"
	"errors"
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
	cfg, err := config.Load()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel(cfg)}))
	slog.SetDefault(logger)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	store, closeStore, err := repository.NewStore(context.Background(), cfg)
	if err != nil {
		logger.Error("failed to initialize store", "error", err)
		os.Exit(1)
	}
	defer closeStore()

	srv := server.New(cfg, store, logger).HTTPServer()
	// Bind the public port before the optional metrics listener, so a port
	// collision disables metrics instead of stopping PermitPal.
	listener, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		logger.Error("server failed", "error", err)
		closeStore()
		os.Exit(1)
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

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case <-stop:
	case err := <-serverErr:
		logger.Error("server failed", "error", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server shutdown failed", "error", err)
	}
	if err := metricsSrv.Shutdown(ctx); err != nil {
		logger.Error("metrics shutdown failed", "error", err)
	}
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
