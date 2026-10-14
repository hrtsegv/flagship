package flagship_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	flagship "github.com/hrtsegv/flagship"
	"github.com/hrtsegv/flagship/internal/app"
	"github.com/hrtsegv/flagship/internal/domain"
	"github.com/hrtsegv/flagship/internal/httpapi"
	"github.com/hrtsegv/flagship/internal/memory"
)

// waitForBool polls client.Bool(key, defaultValue) until it equals want,
// failing the test if that never happens within a short deadline. Passing
// a defaultValue different from want means a still-unbootstrapped client
// (which falls back to defaultValue) is correctly not mistaken for one
// that has already reached the expected state.
func waitForBool(t *testing.T, client *flagship.Client, key string, defaultValue, want bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if got := client.Bool(key, defaultValue); got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("Bool(%q, %v) never became %v within the timeout", key, defaultValue, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// newRealFlagshipServer starts a real control plane with one project,
// environment, and flag (new-checkout, disabled) already created, for
// tests that exercise Sync against the genuine server rather than a
// hand-rolled fake.
func newRealFlagshipServer(t *testing.T) (server *httptest.Server, flagSvc *app.FlagService, flag domain.Flag, environmentID string) {
	t.Helper()

	projectRepo := memory.NewProjectRepository()
	environmentRepo := memory.NewEnvironmentRepository()
	flagRepo := memory.NewFlagRepository()
	projectSvc := app.NewProjectService(projectRepo)
	environmentSvc := app.NewEnvironmentService(environmentRepo, projectRepo)
	flagSvc = app.NewFlagService(flagRepo, environmentRepo)
	server = httptest.NewServer(httpapi.NewServer(projectSvc, environmentSvc, flagSvc, nil))

	ctx := context.Background()
	project, err := projectSvc.Create(ctx, app.CreateProjectInput{Key: "ecommerce", Name: "Ecommerce"})
	if err != nil {
		t.Fatal(err)
	}
	env, err := environmentSvc.Create(ctx, project.ID, app.CreateEnvironmentInput{Key: "production", Name: "Production"})
	if err != nil {
		t.Fatal(err)
	}
	flag, err = flagSvc.Create(ctx, env.ID, app.CreateFlagInput{Key: "new-checkout", Name: "New Checkout", Enabled: false})
	if err != nil {
		t.Fatal(err)
	}

	return server, flagSvc, flag, env.ID
}

func TestSync_ReturnsErrorForInvalidConfig(t *testing.T) {
	err := flagship.Sync(context.Background(), flagship.NewClient(nil), flagship.SyncConfig{})
	if err == nil {
		t.Fatal("Sync() error = nil, want an error for a config with no BaseURL/EnvironmentID")
	}
}

func TestSync_ExpiredReplayRecoversWithoutAnotherMutation(t *testing.T) {
	server, svc, flag, environmentID := newRealFlagshipServer(t)
	defer server.Close()
	enabled := true
	for i := 0; i < 1001; i++ {
		if _, err := svc.Update(context.Background(), flag.ID, domain.FlagPatch{Enabled: &enabled}); err != nil {
			t.Fatal(err)
		}
	}
	client := flagship.NewClient(flagship.NewSnapshot("production", 1, []flagship.Flag{
		{Key: flag.Key, Enabled: false},
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = flagship.Sync(ctx, client, flagship.SyncConfig{BaseURL: server.URL, EnvironmentID: environmentID})
	}()
	waitForBool(t, client, flag.Key, false, true)
}

type pausedFlushWriter struct {
	http.ResponseWriter
	pause func()
}

func TestSync_ResetRejectsWrongEnvironment(t *testing.T) {
	for _, tc := range []struct {
		name, resetEnvironment, snapshotEnvironment string
	}{
		{"reset", "staging", "production"},
		{"recovery snapshot", "production", "staging"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v1/environments/e1/stream", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintf(w, "event: configuration.reset\ndata: {\"environment\":%q}\n\n", tc.resetEnvironment)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			})
			mux.HandleFunc("GET /api/v1/environments/e1/configuration", func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"version": 2, "environment": tc.snapshotEnvironment,
					"flags": []map[string]any{{"key": "a", "enabled": true}},
				})
			})
			server := httptest.NewServer(mux)
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			client := flagship.NewClient(flagship.NewSnapshot("production", 1, []flagship.Flag{{Key: "a", Enabled: false}}))
			err := flagship.Sync(ctx, client, flagship.SyncConfig{
				BaseURL: server.URL, EnvironmentID: "e1",
				MinBackoff: time.Hour, MaxBackoff: time.Hour,
			})
			if !errors.Is(err, flagship.ErrWrongEnvironment) {
				t.Fatalf("Sync() = %v, want ErrWrongEnvironment", err)
			}
			if client.Bool("a", true) {
				t.Fatal("wrong-environment reset changed the installed configuration")
			}
		})
	}
}

func (w *pausedFlushWriter) Flush() {
	w.ResponseWriter.(http.Flusher).Flush()
	if w.pause != nil {
		pause := w.pause
		w.pause = nil
		pause()
	}
}

func (w *pausedFlushWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func TestSync_SlowConsumerRecoversFinalDroppedUpdate(t *testing.T) {
	original, svc, flag, environmentID := newRealFlagshipServer(t)
	handler := original.Config.Handler
	original.Close()

	subscribed := make(chan struct{})
	release := make(chan struct{})
	resume := sync.OnceFunc(func() { close(release) })
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/environments/"+environmentID+"/stream" && connections.Add(1) == 1 {
			w = &pausedFlushWriter{ResponseWriter: w, pause: func() {
				close(subscribed)
				select {
				case <-release:
				case <-r.Context().Done():
				}
			}}
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer resume()
	client := flagship.NewClient(flagship.NewSnapshot("production", 1, []flagship.Flag{
		{Key: flag.Key, Enabled: false},
	}))
	go func() {
		_ = flagship.Sync(ctx, client, flagship.SyncConfig{
			BaseURL: server.URL, EnvironmentID: environmentID,
			MinBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
		})
	}()
	select {
	case <-subscribed:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not subscribe")
	}
	// The handler cannot drain its 16-event buffer until resume is called.
	// The final mutation is lost by the old implementation; no later writes
	// are made to reveal that gap.
	for i := 0; i < 17; i++ {
		enabled := i == 16
		if _, err := svc.Update(ctx, flag.ID, domain.FlagPatch{Enabled: &enabled}); err != nil {
			t.Fatal(err)
		}
	}
	resume()
	waitForBool(t, client, flag.Key, false, true)
	if connections.Load() < 2 {
		t.Fatal("overflowed stream was not reconnected")
	}
}

func TestSync_StopsOnContextCancellation(t *testing.T) {
	server, _, _, environmentID := newRealFlagshipServer(t)
	defer server.Close()

	client := flagship.NewClient(nil)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- flagship.Sync(ctx, client, flagship.SyncConfig{BaseURL: server.URL, EnvironmentID: environmentID})
	}()

	waitForBool(t, client, "new-checkout", true, false) // wait for it to actually connect and bootstrap
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Sync() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Sync() did not return after context cancellation")
	}
}

func TestSync_AppliesLiveEventsFromRealServer(t *testing.T) {
	server, flagSvc, flag, environmentID := newRealFlagshipServer(t)
	defer server.Close()

	client := flagship.NewClient(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = flagship.Sync(ctx, client, flagship.SyncConfig{BaseURL: server.URL, EnvironmentID: environmentID})
	}()

	waitForBool(t, client, "new-checkout", true, false) // bootstrapped to the initial (disabled) state

	enabled := true
	if _, err := flagSvc.Update(context.Background(), flag.ID, domain.FlagPatch{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}

	waitForBool(t, client, "new-checkout", false, true) // picked up live, no polling
}

func TestSync_ReconnectsWithBackoffAfterStreamDrop(t *testing.T) {
	var connections atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/environments/e1/configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version": 1, "environment": "production",
			"flags": []map[string]any{{"key": "new-checkout", "enabled": false}},
		})
	})
	mux.HandleFunc("GET /api/v1/environments/e1/stream", func(w http.ResponseWriter, r *http.Request) {
		n := connections.Add(1)
		flusher := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		if n == 1 {
			return // drop immediately, before sending anything
		}
		_, _ = fmt.Fprintf(w, "id: 2\nevent: configuration.updated\ndata: %s\n\n",
			`{"environment":"production","version":2,"type":"flag_updated","flag_key":"new-checkout","enabled":true}`)
		flusher.Flush()
		<-r.Context().Done()
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := flagship.NewClient(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var errCount atomic.Int32
	go func() {
		_ = flagship.Sync(ctx, client, flagship.SyncConfig{
			BaseURL: server.URL, EnvironmentID: "e1",
			OnError:    func(error) { errCount.Add(1) },
			MinBackoff: time.Millisecond,
			MaxBackoff: 5 * time.Millisecond,
		})
	}()

	waitForBool(t, client, "new-checkout", false, true)

	if n := connections.Load(); n < 2 {
		t.Fatalf("connections = %d, want at least 2 (a reconnect after the first drop)", n)
	}
	if errCount.Load() == 0 {
		t.Error("OnError was never called, want it invoked for the dropped first connection")
	}
}

func TestSync_FatalOnWrongEnvironmentEvent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/environments/e1/configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version": 1, "environment": "production",
			"flags": []map[string]any{{"key": "new-checkout", "enabled": false}},
		})
	})
	mux.HandleFunc("GET /api/v1/environments/e1/stream", func(w http.ResponseWriter, r *http.Request) {
		flusher := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "id: 2\nevent: configuration.updated\ndata: %s\n\n",
			`{"environment":"staging","version":2,"type":"flag_updated","flag_key":"new-checkout","enabled":true}`)
		flusher.Flush()
		<-r.Context().Done()
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := flagship.NewClient(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- flagship.Sync(ctx, client, flagship.SyncConfig{
			BaseURL: server.URL, EnvironmentID: "e1",
			// Large enough that a hang here means Sync retried instead
			// of failing fast, not that the test got unlucky on timing.
			MinBackoff: time.Hour, MaxBackoff: time.Hour,
		})
	}()

	select {
	case err := <-done:
		if !errors.Is(err, flagship.ErrWrongEnvironment) {
			t.Fatalf("Sync() error = %v, want ErrWrongEnvironment", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Sync() did not return promptly for a wrong-environment event; did it retry instead of failing fast?")
	}
}

// TestSync_RecoversFromGapWithoutReconnecting proves the property called
// out in sync.go: a version gap is recovered by fetching a snapshot on
// the *same* connection, not by reconnecting.
func TestSync_RecoversFromGapWithoutReconnecting(t *testing.T) {
	var streamConnections atomic.Int32
	var configVersion atomic.Int32
	configVersion.Store(1)
	releaseGapEvent := make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/environments/e1/configuration", func(w http.ResponseWriter, r *http.Request) {
		v := configVersion.Load()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version": v, "environment": "production",
			"flags": []map[string]any{{"key": "new-checkout", "enabled": v >= 5}},
		})
	})
	mux.HandleFunc("GET /api/v1/environments/e1/stream", func(w http.ResponseWriter, r *http.Request) {
		streamConnections.Add(1)
		flusher := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		<-releaseGapEvent // wait until the test has bumped configVersion
		// Client bootstraps at v1; jumping straight to v5 here is a gap.
		_, _ = fmt.Fprintf(w, "id: 5\nevent: configuration.updated\ndata: %s\n\n",
			`{"environment":"production","version":5,"type":"flag_updated","flag_key":"new-checkout","enabled":true}`)
		flusher.Flush()
		<-r.Context().Done()
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := flagship.NewClient(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = flagship.Sync(ctx, client, flagship.SyncConfig{BaseURL: server.URL, EnvironmentID: "e1"}) }()

	// Only bump the "authoritative" version (and release the gap event)
	// once the client has actually bootstrapped at v1, so the recovery
	// fetch triggered below is guaranteed to observe v5, not a stale v1.
	waitForBool(t, client, "new-checkout", true, false)
	configVersion.Store(5)
	close(releaseGapEvent)

	waitForBool(t, client, "new-checkout", false, true)

	if n := streamConnections.Load(); n != 1 {
		t.Fatalf("stream connections = %d, want exactly 1: gap recovery must not require reconnecting", n)
	}
}
