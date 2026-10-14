package flagship_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	flagship "github.com/hrtsegv/flagship"
	"github.com/hrtsegv/flagship/internal/app"
	"github.com/hrtsegv/flagship/internal/domain"
	"github.com/hrtsegv/flagship/internal/httpapi"
	"github.com/hrtsegv/flagship/internal/memory"
)

func TestFetchSnapshot_RejectsIncompleteOrInvalidState(t *testing.T) {
	for _, payload := range []string{
		`{"version":-1,"environment":"production","flags":[]}`,
		`{"version":1,"environment":"","flags":[]}`,
		`{"environment":"production","flags":[]}`,
		`{"version":1,"environment":"production"}`,
		`{"version":1,"environment":"production","flags":null}`,
		`{"version":1,"environment":"production","flags":[{"key":"","enabled":true}]}`,
		`{"version":1,"environment":"production","flags":[{"key":"a"}]}`,
		`{"version":1,"environment":"production","flags":[{"key":"a","enabled":null}]}`,
		`{"version":1,"environment":"production","flags":[{"key":"a","enabled":true},{"key":"a","enabled":false}]}`,
		`{"version":1,"environment":"production","flags":[]} {}`,
	} {
		t.Run(payload, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(payload))
			}))
			defer server.Close()
			if snapshot, err := flagship.FetchSnapshot(context.Background(), server.Client(), server.URL, "e1"); err == nil || snapshot != nil {
				t.Fatalf("invalid snapshot = (%v, %v), want nil and an error", snapshot, err)
			}
		})
	}
}

func TestFetchSnapshot_AcceptsEmptyEnvironmentConfigurationAndUnknownFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":0,"environment":"production","flags":[],"future_field":true}`))
	}))
	defer server.Close()
	snapshot, err := flagship.FetchSnapshot(context.Background(), server.Client(), server.URL, "e1")
	if err != nil {
		t.Fatal(err)
	}
	client := flagship.NewClient(snapshot)
	if snapshot.Version() != 0 || snapshot.Environment() != "production" || !client.Bool("missing", true) {
		t.Fatal("empty configuration did not preserve identity, version, or fallback")
	}
}

func TestFetchSnapshot_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version":     3,
			"environment": "production",
			"flags":       []map[string]any{{"key": "new-checkout", "enabled": true}},
		})
	}))
	defer server.Close()

	snap, err := flagship.FetchSnapshot(context.Background(), nil, server.URL, "env-1")
	if err != nil {
		t.Fatalf("FetchSnapshot() unexpected error: %v", err)
	}
	if snap.Version() != 3 {
		t.Errorf("Version() = %d, want 3", snap.Version())
	}
	if snap.Environment() != "production" {
		t.Errorf("Environment() = %q, want %q", snap.Environment(), "production")
	}

	client := flagship.NewClient(snap)
	if got := client.Bool("new-checkout", false); got != true {
		t.Errorf("Bool() = %v, want true", got)
	}
}

func TestFetchSnapshot_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := flagship.FetchSnapshot(context.Background(), nil, server.URL, "missing")
	if err == nil {
		t.Fatal("FetchSnapshot() error = nil, want an error for a non-200 response")
	}
}

func TestFetchSnapshot_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{not json"))
	}))
	defer server.Close()

	_, err := flagship.FetchSnapshot(context.Background(), nil, server.URL, "env-1")
	if err == nil {
		t.Fatal("FetchSnapshot() error = nil, want an error for a malformed body")
	}
}

// TestApplyEvent_GapRecovery_AgainstRealServer exercises the full recovery
// path end-to-end: a client detects a version gap, recovers by calling
// FetchSnapshot against a real running control plane (not a stub), and
// installs the result via UpdateSnapshot.
func TestApplyEvent_GapRecovery_AgainstRealServer(t *testing.T) {
	projectRepo := memory.NewProjectRepository()
	environmentRepo := memory.NewEnvironmentRepository()
	flagRepo := memory.NewFlagRepository()
	projectSvc := app.NewProjectService(projectRepo)
	environmentSvc := app.NewEnvironmentService(environmentRepo, projectRepo)
	flagSvc := app.NewFlagService(flagRepo, environmentRepo)
	server := httptest.NewServer(httpapi.NewServer(projectSvc, environmentSvc, flagSvc, nil))
	defer server.Close()

	ctx := context.Background()
	project, err := projectSvc.Create(ctx, app.CreateProjectInput{Key: "ecommerce", Name: "Ecommerce"})
	if err != nil {
		t.Fatal(err)
	}
	env, err := environmentSvc.Create(ctx, project.ID, app.CreateEnvironmentInput{Key: "production", Name: "Production"})
	if err != nil {
		t.Fatal(err)
	}
	flag, err := flagSvc.Create(ctx, env.ID, app.CreateFlagInput{Key: "new-checkout", Name: "New Checkout", Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	// Two updates bump the environment to version 3 (1 create + 2 updates)
	// while the client below only ever saw version 1.
	enabled, disabled := true, false
	if _, err := flagSvc.Update(ctx, flag.ID, domain.FlagPatch{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	if _, err := flagSvc.Update(ctx, flag.ID, domain.FlagPatch{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}

	client := flagship.NewClient(flagship.NewSnapshot("production", 1, []flagship.Flag{{Key: "new-checkout", Enabled: false}}))

	_, err = client.ApplyEvent(flagship.ConfigEvent{
		Environment: "production", Version: 4, Type: flagship.EventFlagUpdated, FlagKey: "new-checkout", Enabled: true,
	})
	if err == nil {
		t.Fatal("ApplyEvent() error = nil, want ErrVersionGap")
	}

	recovered, err := flagship.FetchSnapshot(ctx, server.Client(), server.URL, env.ID)
	if err != nil {
		t.Fatalf("FetchSnapshot() unexpected error: %v", err)
	}
	if !client.UpdateSnapshot(recovered) {
		t.Fatal("UpdateSnapshot() = false, want true installing the recovered snapshot")
	}
	if got := client.Bool("new-checkout", true); got != false {
		t.Errorf("Bool() after recovery = %v, want false (the flag's actual current state)", got)
	}
}
