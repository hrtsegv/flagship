package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/hrtsegv/flagship/internal/app"
	"github.com/hrtsegv/flagship/internal/httpapi"
	"github.com/hrtsegv/flagship/internal/memory"
)

func TestConfigFromEnv(t *testing.T) {
	for _, tc := range []struct {
		name, databaseURL, address, timeout string
		wantAddress                         string
		wantTimeout                         time.Duration
		wantError                           bool
	}{
		{name: "defaults", databaseURL: "postgres://test", wantAddress: ":8080", wantTimeout: 10 * time.Second},
		{name: "overrides", databaseURL: "postgres://test", address: "127.0.0.1:9000", timeout: "3s", wantAddress: "127.0.0.1:9000", wantTimeout: 3 * time.Second},
		{name: "missing database", wantError: true},
		{name: "invalid timeout", databaseURL: "postgres://test", timeout: "bad", wantError: true},
		{name: "zero timeout", databaseURL: "postgres://test", timeout: "0s", wantError: true},
		{name: "negative timeout", databaseURL: "postgres://test", timeout: "-1s", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", tc.databaseURL)
			t.Setenv("HTTP_ADDR", tc.address)
			t.Setenv("SHUTDOWN_TIMEOUT", tc.timeout)
			cfg, err := configFromEnv()
			if (err != nil) != tc.wantError {
				t.Fatalf("config error = %v, wantError = %v", err, tc.wantError)
			}
			if err == nil && (cfg.databaseURL != tc.databaseURL || cfg.httpAddr != tc.wantAddress || cfg.shutdownTimeout != tc.wantTimeout) {
				t.Fatalf("unexpected configuration: %+v", cfg)
			}
		})
	}
}

func TestServe_ShutdownClosesActiveSSEStream(t *testing.T) {
	projectRepo := memory.NewProjectRepository()
	envRepo := memory.NewEnvironmentRepository()
	flagRepo := memory.NewFlagRepository()
	projects := app.NewProjectService(projectRepo)
	environments := app.NewEnvironmentService(envRepo, projectRepo)
	flags := app.NewFlagService(flagRepo, envRepo)
	project, err := projects.Create(context.Background(), app.CreateProjectInput{Key: "p", Name: "Project"})
	if err != nil {
		t.Fatal(err)
	}
	env, err := environments.Create(context.Background(), project.ID, app.CreateEnvironmentInput{Key: "production", Name: "Production"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, listener, httpapi.NewServer(projects, environments, flags, nil), time.Second)
	}()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + listener.Addr().String() + "/api/v1/environments/" + env.ID + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d", resp.StatusCode)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("shutdown = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("active SSE stream prevented graceful shutdown")
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("stream did not close cleanly: %v", err)
	}
}

func TestServe_ShutdownDeadlineClosesUnresponsiveConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	release := make(chan struct{})
	defer close(release)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-release // Deliberately ignore request cancellation.
	})
	done := make(chan error, 1)
	go func() { done <- serve(ctx, listener, handler, 20*time.Millisecond) }()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("shutdown = %v, want deadline exceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown deadline did not bound the server's lifetime")
	}
}
