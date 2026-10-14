package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealth_LivenessDoesNotDependOnDatabase(t *testing.T) {
	called := false
	h := NewServer(nil, nil, nil, func(context.Context) error {
		called = true
		return errors.New("database unavailable")
	})
	rec := doRequest(t, h, http.MethodGet, "/healthz", nil)
	if rec.Code != http.StatusOK || called {
		t.Fatalf("liveness = %d, database checked = %v", rec.Code, called)
	}
}

func TestHealth_Readiness(t *testing.T) {
	for _, tc := range []struct {
		name   string
		check  func(context.Context) error
		status int
	}{
		{"healthy", func(context.Context) error { return nil }, http.StatusOK},
		{"database unavailable", func(context.Context) error { return errors.New("private connection details") }, http.StatusServiceUnavailable},
		{"unconfigured", nil, http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewServer(nil, nil, nil, tc.check)
			rec := doRequest(t, h, http.MethodGet, "/readyz", nil)
			if rec.Code != tc.status || strings.Contains(rec.Body.String(), "private") {
				t.Fatalf("readiness = %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHealth_ReadinessPropagatesCancellationAndDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h := NewServer(nil, nil, nil, func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 2*time.Second {
			t.Error("readiness probe has no bounded deadline")
		}
		return ctx.Err()
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil).WithContext(ctx))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("canceled readiness = %d, want 503", rec.Code)
	}
}
