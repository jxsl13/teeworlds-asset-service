package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/jxsl13/asset-service/config"
	"github.com/jxsl13/asset-service/http/api"
	httpserver "github.com/jxsl13/asset-service/http/server"
	postgresql "github.com/jxsl13/asset-service/sql"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// ── Database ──────────────────────────────────────────────────────────────

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("pgxpool: %w", err)
	}
	defer pool.Close()

	connCtx, connCancel := context.WithTimeout(ctx, 15*time.Second)
	defer connCancel()
	if err := pool.Ping(connCtx); err != nil {
		return fmt.Errorf("db ping: %w", err)
	}
	connCancel()

	// ── Migrations ────────────────────────────────────────────────────────────
	if err := postgresql.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}

	// ── DAO / queries ─────────────────────────────────────────────────────────
	sqlDB := stdlib.OpenDBFromPool(pool)
	queries := postgresql.New(sqlDB)

	// ── Ensure storage directories exist ──────────────────────────────────────
	for _, dir := range []string{cfg.StoragePath, cfg.TempUploadPath} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("create directory %q: %w", dir, err)
		}
	}

	// ── HTTP server ───────────────────────────────────────────────────────────
	srv, err := httpserver.New(sqlDB, queries, cfg.StoragePath, cfg.TempUploadPath, cfg.MaxStorageSize, cfg.AllowedResolutions, cfg.MaxUploadSizes, cfg.ThumbnailSizes)
	if err != nil {
		return fmt.Errorf("server: %w", err)
	}
	defer srv.Close()

	r := chi.NewRouter()

	// Structured request logging for every request.
	r.Use(httpserver.RequestLogger)

	// Extract client IP and store in context for audit logging.
	r.Use(httpserver.ClientIPMiddleware)

	// Parse HTMX request headers into context for all requests.
	r.Use(httpserver.HtmxMiddleware)

	// Serve embedded static assets (htmx.min.js etc.).
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(httpserver.StaticFS())))

	// Mount all API + UI routes (generated from OpenAPI spec).
	//
	// The ResponseErrorHandlerFunc handles truly unexpected errors returned
	// as Go errors from strict server handlers. It logs the actual error
	// server-side but sends only a generic message to the client.
	strict := api.NewStrictHandlerWithOptions(srv, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("unhandled response error", "method", r.Method, "path", r.URL.Path, "err", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal server error"})
		},
	})
	api.HandlerWithOptions(
		strict,
		api.ChiServerOptions{BaseRouter: r},
	)

	httpSrv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutting down", "signal", ctx.Err())
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	return nil
}
