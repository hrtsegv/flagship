// Command flagship runs the Flagship control-plane HTTP API backed by
// PostgreSQL.
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

	"github.com/hrtsegv/flagship/internal/app"
	"github.com/hrtsegv/flagship/internal/httpapi"
	"github.com/hrtsegv/flagship/internal/postgres"
)

type config struct {
	databaseURL     string
	httpAddr        string
	shutdownTimeout time.Duration
}

func configFromEnv() (config, error) {
	cfg := config{
		databaseURL:     os.Getenv("DATABASE_URL"),
		httpAddr:        os.Getenv("HTTP_ADDR"),
		shutdownTimeout: 10 * time.Second,
	}
	if cfg.databaseURL == "" {
		return config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.httpAddr == "" {
		cfg.httpAddr = ":8080"
	}
	if raw := os.Getenv("SHUTDOWN_TIMEOUT"); raw != "" {
		timeout, err := time.ParseDuration(raw)
		if err != nil || timeout <= 0 {
			return config{}, errors.New("SHUTDOWN_TIMEOUT must be a positive duration")
		}
		cfg.shutdownTimeout = timeout
	}
	return cfg, nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := configFromEnv()
	if err != nil {
		return err
	}
	if err := postgres.Migrate(cfg.databaseURL); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	pool, err := postgres.Connect(ctx, cfg.databaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	projectRepo := postgres.NewProjectRepository(pool)
	environmentRepo := postgres.NewEnvironmentRepository(pool)
	flagRepo := postgres.NewFlagRepository(pool)

	projectSvc := app.NewProjectService(projectRepo)
	environmentSvc := app.NewEnvironmentService(environmentRepo, projectRepo)
	flagSvc := app.NewFlagService(flagRepo, environmentRepo)

	handler := httpapi.NewServer(projectSvc, environmentSvc, flagSvc, pool.Ping)

	listener, err := net.Listen("tcp", cfg.httpAddr)
	if err != nil {
		return fmt.Errorf("listening: %w", err)
	}
	defer func() { _ = listener.Close() }()
	logger.Info("server listening", "address", listener.Addr().String())
	if err := serve(ctx, listener, handler, cfg.shutdownTimeout); err != nil {
		return err
	}
	logger.Info("server stopped")
	return nil
}

func serve(ctx context.Context, listener net.Listener, handler http.Handler, shutdownTimeout time.Duration) error {
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		return fmt.Errorf("serving HTTP: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		<-done
		return fmt.Errorf("shutting down HTTP server: %w", err)
	}
	if err := <-done; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serving HTTP: %w", err)
	}
	return nil
}
