// ASF-STAC Proxy server entry point
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

	"github.com/rkm/asf-stac-proxy/internal/api"
	"github.com/rkm/asf-stac-proxy/internal/asf"
	"github.com/rkm/asf-stac-proxy/internal/backend"
	"github.com/rkm/asf-stac-proxy/internal/cmr"
	"github.com/rkm/asf-stac-proxy/internal/config"
	"github.com/rkm/asf-stac-proxy/internal/stac"
	"github.com/rkm/asf-stac-proxy/internal/translate"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Set up logger
	logger := setupLogger(cfg.Logging.Level, cfg.Logging.Format)

	logger.Info("starting ASF-STAC proxy",
		"version", cfg.STAC.Version,
		"host", cfg.Server.Host,
		"port", cfg.Server.Port,
	)

	// Load collection definitions
	collections, err := config.LoadCollections("./collections")
	if err != nil {
		logger.Warn("failed to load collections from ./collections, using empty registry", "error", err)
		collections = config.NewCollectionRegistry()
	}
	logger.Info("loaded collections", "count", collections.Count())

	// Create translator
	translator := translate.NewTranslator(cfg, collections, logger)

	// Create cursor store for server-side pagination cursor storage
	// TTL: 1 hour (cursors expire after this time)
	// Cleanup interval: 5 minutes (check for expired cursors)
	cursorStore := stac.NewMemoryCursorStore(1*time.Hour, 5*time.Minute)
	defer cursorStore.Stop()
	logger.Info("initialized cursor store", "ttl", "1h", "cleanup_interval", "5m")

	// Create search backend based on configuration
	var searchBackend backend.SearchBackend
	switch cfg.Backend.Type {
	case "cmr":
		cmrClient := cmr.NewClient(cfg.CMR.BaseURL, cfg.CMR.Provider, cfg.CMR.Timeout).WithLogger(logger)
		searchBackend = cmr.NewCMRBackend(cmrClient, collections, cfg, logger)
		logger.Info("using CMR backend", "base_url", cfg.CMR.BaseURL, "provider", cfg.CMR.Provider)
	default:
		asfClient := asf.NewClient(cfg.ASF.BaseURL, cfg.ASF.Timeout).WithLogger(logger)
		searchBackend = backend.NewASFBackend(asfClient, collections, translator, cfg, logger)
		logger.Info("using ASF backend", "base_url", cfg.ASF.BaseURL)
	}

	// Create handlers with backend and cursor store
	handlers := api.NewHandlers(cfg, searchBackend, translator, collections, logger).
		WithCursorStore(cursorStore)

	// Create router
	router := api.NewRouter(handlers, logger)

	// Create server
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case sig := <-quit:
		logger.Info("received shutdown signal", "signal", sig)
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	logger.Info("shutting down server", "timeout", cfg.Server.ShutdownTimeout)
	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown error: %w", err)
	}

	logger.Info("server stopped")
	return nil
}

func setupLogger(level, format string) *slog.Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: logLevel}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}
