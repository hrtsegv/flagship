package httpapi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/hrtsegv/flagship/internal/app"
	"github.com/hrtsegv/flagship/internal/memory"
)

func newTestServer() http.Handler {
	projectRepo := memory.NewProjectRepository()
	environmentRepo := memory.NewEnvironmentRepository()
	flagRepo := memory.NewFlagRepository()

	projectSvc := app.NewProjectService(projectRepo)
	environmentSvc := app.NewEnvironmentService(environmentRepo, projectRepo)
	flagSvc := app.NewFlagService(flagRepo, environmentRepo)

	return NewServer(projectSvc, environmentSvc, flagSvc)
}

func doRequest(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("failed to decode response body %q: %v", rec.Body.String(), err)
	}
	return v
}

func createProject(t *testing.T, h http.Handler) projectResponse {
	t.Helper()
	rec := doRequest(t, h, http.MethodPost, "/api/v1/projects", createProjectRequest{
		Key: "ecommerce", Name: "Ecommerce", Description: "Store",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("createProject: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	return decodeBody[projectResponse](t, rec)
}

func createEnvironment(t *testing.T, h http.Handler, projectID string) environmentResponse {
	t.Helper()
	rec := doRequest(t, h, http.MethodPost, "/api/v1/projects/"+projectID+"/environments", createEnvironmentRequest{
		Key: "production", Name: "Production",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("createEnvironment: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	return decodeBody[environmentResponse](t, rec)
}

func createFlag(t *testing.T, h http.Handler, environmentID string) flagResponse {
	t.Helper()
	rec := doRequest(t, h, http.MethodPost, "/api/v1/environments/"+environmentID+"/flags", createFlagRequest{
		Key: "new-checkout", Name: "New Checkout", Enabled: false,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("createFlag: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	return decodeBody[flagResponse](t, rec)
}

func TestProjects_CreateAndList(t *testing.T) {
	h := newTestServer()

	p := createProject(t, h)
	if p.Key != "ecommerce" {
		t.Errorf("Key = %q, want %q", p.Key, "ecommerce")
	}
	if p.ID == "" {
		t.Error("expected a generated ID")
	}

	rec := doRequest(t, h, http.MethodGet, "/api/v1/projects", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", rec.Code)
	}
	list := decodeBody[[]projectResponse](t, rec)
	if len(list) != 1 {
		t.Fatalf("list length = %d, want 1", len(list))
	}
}

func TestProjects_Create_DuplicateKeyReturns409(t *testing.T) {
	h := newTestServer()
	createProject(t, h)

	rec := doRequest(t, h, http.MethodPost, "/api/v1/projects", createProjectRequest{Key: "ecommerce", Name: "Again"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	errResp := decodeBody[errorResponse](t, rec)
	if errResp.Error.Code != "duplicate_key" {
		t.Errorf("error.code = %q, want %q", errResp.Error.Code, "duplicate_key")
	}
}

func TestProjects_Create_MissingFieldsReturns400(t *testing.T) {
	h := newTestServer()

	rec := doRequest(t, h, http.MethodPost, "/api/v1/projects", createProjectRequest{Name: "No Key"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	errResp := decodeBody[errorResponse](t, rec)
	if errResp.Error.Code != "validation_error" {
		t.Errorf("error.code = %q, want %q", errResp.Error.Code, "validation_error")
	}
}

func TestProjects_Create_InvalidJSONReturns400(t *testing.T) {
	h := newTestServer()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader([]byte("{not json")))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestEnvironments_CreateAndList(t *testing.T) {
	h := newTestServer()
	project := createProject(t, h)

	env := createEnvironment(t, h, project.ID)
	if env.ProjectID != project.ID {
		t.Errorf("ProjectID = %q, want %q", env.ProjectID, project.ID)
	}

	rec := doRequest(t, h, http.MethodGet, "/api/v1/projects/"+project.ID+"/environments", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", rec.Code)
	}
	list := decodeBody[[]environmentResponse](t, rec)
	if len(list) != 1 {
		t.Fatalf("list length = %d, want 1", len(list))
	}
}

func TestEnvironments_Create_ProjectNotFoundReturns404(t *testing.T) {
	h := newTestServer()

	rec := doRequest(t, h, http.MethodPost, "/api/v1/projects/missing/environments", createEnvironmentRequest{
		Key: "production", Name: "Production",
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

func TestEnvironments_Create_DuplicateKeyReturns409(t *testing.T) {
	h := newTestServer()
	project := createProject(t, h)
	createEnvironment(t, h, project.ID)

	rec := doRequest(t, h, http.MethodPost, "/api/v1/projects/"+project.ID+"/environments", createEnvironmentRequest{
		Key: "production", Name: "Again",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

func TestFlags_CreateGetListUpdate(t *testing.T) {
	h := newTestServer()
	project := createProject(t, h)
	env := createEnvironment(t, h, project.ID)

	flag := createFlag(t, h, env.ID)
	if flag.Version != 1 {
		t.Errorf("Version = %d, want 1", flag.Version)
	}
	if flag.Enabled {
		t.Error("Enabled = true, want false")
	}

	// Get
	rec := doRequest(t, h, http.MethodGet, "/api/v1/flags/"+flag.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	got := decodeBody[flagResponse](t, rec)
	if got.Key != "new-checkout" {
		t.Errorf("Key = %q, want %q", got.Key, "new-checkout")
	}

	// List
	rec = doRequest(t, h, http.MethodGet, "/api/v1/environments/"+env.ID+"/flags", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", rec.Code)
	}
	list := decodeBody[[]flagResponse](t, rec)
	if len(list) != 1 {
		t.Fatalf("list length = %d, want 1", len(list))
	}

	// Update (PATCH enabled=true)
	rec = doRequest(t, h, http.MethodPatch, "/api/v1/flags/"+flag.ID, updateFlagRequest{Enabled: boolPtr(true)})
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	updated := decodeBody[flagResponse](t, rec)
	if !updated.Enabled {
		t.Error("Enabled = false, want true after update")
	}
	if updated.Version != 2 {
		t.Errorf("Version = %d, want 2 after update", updated.Version)
	}
}

func TestFlags_Create_EnvironmentNotFoundReturns404(t *testing.T) {
	h := newTestServer()

	rec := doRequest(t, h, http.MethodPost, "/api/v1/environments/missing/flags", createFlagRequest{
		Key: "new-checkout", Name: "New Checkout",
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

func TestFlags_Create_DuplicateKeyReturns409(t *testing.T) {
	h := newTestServer()
	project := createProject(t, h)
	env := createEnvironment(t, h, project.ID)
	createFlag(t, h, env.ID)

	rec := doRequest(t, h, http.MethodPost, "/api/v1/environments/"+env.ID+"/flags", createFlagRequest{
		Key: "new-checkout", Name: "Again",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

func TestFlags_Get_NotFoundReturns404(t *testing.T) {
	h := newTestServer()

	rec := doRequest(t, h, http.MethodGet, "/api/v1/flags/missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

func TestFlags_Update_NotFoundReturns404(t *testing.T) {
	h := newTestServer()

	rec := doRequest(t, h, http.MethodPatch, "/api/v1/flags/missing", updateFlagRequest{Enabled: boolPtr(true)})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

func TestFlags_Delete_ReturnsNoContentAndRemovesFlag(t *testing.T) {
	h := newTestServer()
	project := createProject(t, h)
	env := createEnvironment(t, h, project.ID)
	flag := createFlag(t, h, env.ID)

	rec := doRequest(t, h, http.MethodDelete, "/api/v1/flags/"+flag.ID, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204, body = %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Errorf("delete body = %q, want empty", rec.Body.String())
	}

	rec = doRequest(t, h, http.MethodGet, "/api/v1/flags/"+flag.ID, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want 404", rec.Code)
	}
}

func TestFlags_Delete_NotFoundReturns404(t *testing.T) {
	h := newTestServer()

	rec := doRequest(t, h, http.MethodDelete, "/api/v1/flags/missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

func TestFlags_Delete_KeyBecomesReusable(t *testing.T) {
	h := newTestServer()
	project := createProject(t, h)
	env := createEnvironment(t, h, project.ID)
	flag := createFlag(t, h, env.ID)

	doRequest(t, h, http.MethodDelete, "/api/v1/flags/"+flag.ID, nil)

	rec := doRequest(t, h, http.MethodPost, "/api/v1/environments/"+env.ID+"/flags", createFlagRequest{
		Key: "new-checkout", Name: "New Checkout Again",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("recreate after delete status = %d, want 201, body = %s", rec.Code, rec.Body.String())
	}
}

func TestConfiguration_ReflectsDeletion(t *testing.T) {
	h := newTestServer()
	project := createProject(t, h)
	env := createEnvironment(t, h, project.ID)
	flag := createFlag(t, h, env.ID)

	doRequest(t, h, http.MethodDelete, "/api/v1/flags/"+flag.ID, nil)

	rec := doRequest(t, h, http.MethodGet, "/api/v1/environments/"+env.ID+"/configuration", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	cfg := decodeBody[configurationResponse](t, rec)
	if cfg.Version != 2 {
		t.Errorf("Version = %d, want 2 after create+delete", cfg.Version)
	}
	if len(cfg.Flags) != 0 {
		t.Errorf("Flags = %+v, want empty after deletion", cfg.Flags)
	}
}

func TestConfiguration_ReturnsCurrentFlagsAndVersion(t *testing.T) {
	h := newTestServer()
	project := createProject(t, h)
	env := createEnvironment(t, h, project.ID)
	createFlag(t, h, env.ID)

	rec := doRequest(t, h, http.MethodGet, "/api/v1/environments/"+env.ID+"/configuration", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	cfg := decodeBody[configurationResponse](t, rec)
	if cfg.Version != 1 {
		t.Errorf("Version = %d, want 1", cfg.Version)
	}
	if cfg.Environment != "production" {
		t.Errorf("Environment = %q, want %q", cfg.Environment, "production")
	}
	if len(cfg.Flags) != 1 || cfg.Flags[0].Key != "new-checkout" || cfg.Flags[0].Enabled {
		t.Errorf("Flags = %+v, want one disabled new-checkout flag", cfg.Flags)
	}
}

func TestConfiguration_VersionIncrementsAfterFlagUpdate(t *testing.T) {
	h := newTestServer()
	project := createProject(t, h)
	env := createEnvironment(t, h, project.ID)
	flag := createFlag(t, h, env.ID)

	doRequest(t, h, http.MethodPatch, "/api/v1/flags/"+flag.ID, updateFlagRequest{Enabled: boolPtr(true)})

	rec := doRequest(t, h, http.MethodGet, "/api/v1/environments/"+env.ID+"/configuration", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	cfg := decodeBody[configurationResponse](t, rec)
	if cfg.Version != 2 {
		t.Errorf("Version = %d, want 2 after one flag update", cfg.Version)
	}
	if len(cfg.Flags) != 1 || !cfg.Flags[0].Enabled {
		t.Errorf("Flags = %+v, want one enabled new-checkout flag", cfg.Flags)
	}
}

func TestConfiguration_EnvironmentNotFoundReturns404(t *testing.T) {
	h := newTestServer()

	rec := doRequest(t, h, http.MethodGet, "/api/v1/environments/missing/configuration", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

func boolPtr(b bool) *bool { return &b }

// readSSEFrame reads one Server-Sent Events frame (id/event/data lines
// terminated by a blank line) and decodes its data as a configEventPayload.
// Tests using this rely on streamHeartbeatInterval being much longer than
// any test run, so a heartbeat comment never interleaves with a real frame.
func readSSEFrame(t *testing.T, r *bufio.Reader) (id int, event string, payload configEventPayload) {
	t.Helper()

	var data string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("reading SSE frame: %v", err)
		}
		line = strings.TrimRight(line, "\n")

		switch {
		case line == "":
			if err := json.Unmarshal([]byte(data), &payload); err != nil {
				t.Fatalf("decoding SSE data %q: %v", data, err)
			}
			return id, event, payload
		case strings.HasPrefix(line, "id: "):
			parsed, err := strconv.Atoi(strings.TrimPrefix(line, "id: "))
			if err != nil {
				t.Fatalf("parsing SSE id: %v", err)
			}
			id = parsed
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		}
	}
}

func TestStream_EnvironmentNotFoundReturns404(t *testing.T) {
	h := newTestServer()

	rec := doRequest(t, h, http.MethodGet, "/api/v1/environments/missing/stream", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

func TestStream_InvalidLastEventIDReturns400BeforeEnvironmentCheck(t *testing.T) {
	h := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/environments/missing/stream", nil)
	req.Header.Set("Last-Event-ID", "not-a-number")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (before the 404 an unknown environment would otherwise cause), body = %s", rec.Code, rec.Body.String())
	}
}

func TestStream_NegativeLastEventIDReturns400(t *testing.T) {
	h := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/environments/missing/stream", nil)
	req.Header.Set("Last-Event-ID", "-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

// TestStream_ReplaysThenDeliversLiveWithoutDuplicating drives the full
// protocol end-to-end over a real connection: a client resumes from a
// version it has already applied, is replayed exactly what it missed,
// and then receives a further live update with nothing repeated or
// skipped at the handoff. Direct handler calls via httptest.ResponseRecorder
// can't exercise this (the handler's live loop never returns until the
// request context is canceled, which a synchronous ServeHTTP call never
// does), so this uses a real httptest.Server and a real HTTP client.
func TestStream_ReplaysThenDeliversLiveWithoutDuplicating(t *testing.T) {
	h := newTestServer()
	server := httptest.NewServer(h)
	defer server.Close()

	project := createProject(t, h)
	env := createEnvironment(t, h, project.ID)
	flag := createFlag(t, h, env.ID) // v1: enabled=false

	doRequest(t, h, http.MethodPatch, "/api/v1/flags/"+flag.ID, updateFlagRequest{Enabled: boolPtr(true)})  // v2
	doRequest(t, h, http.MethodPatch, "/api/v1/flags/"+flag.ID, updateFlagRequest{Enabled: boolPtr(false)}) // v3

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/environments/"+env.ID+"/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Last-Event-ID", "1")

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/event-stream")
	}

	reader := bufio.NewReader(resp.Body)

	id, event, p2 := readSSEFrame(t, reader)
	if id != 2 || event != "configuration.updated" || p2.Version != 2 || !p2.Enabled {
		t.Fatalf("first replayed frame = (id=%d, event=%s, payload=%+v), want id=2 enabled=true", id, event, p2)
	}
	id, _, p3 := readSSEFrame(t, reader)
	if id != 3 || p3.Version != 3 || p3.Enabled {
		t.Fatalf("second replayed frame = (id=%d, payload=%+v), want id=3 enabled=false", id, p3)
	}

	// A live update (v4) after replay finished must arrive without
	// repeating v2/v3.
	doRequest(t, h, http.MethodPatch, "/api/v1/flags/"+flag.ID, updateFlagRequest{Enabled: boolPtr(true)})
	id, _, p4 := readSSEFrame(t, reader)
	if id != 4 || p4.Version != 4 || !p4.Enabled || p4.Environment != "production" || p4.FlagKey != "new-checkout" {
		t.Fatalf("live frame = (id=%d, payload=%+v), want id=4 for production/new-checkout enabled=true", id, p4)
	}
}

// TestStream_NotReplayableStillDeliversLiveEvents covers the fallback
// path: a client that can't be replayed (here, because it claims a
// version the server never reached) still gets a working live stream
// rather than an error or a hang. The SDK's own gap detection is what
// makes this safe -- the server doesn't need to signal anything special.
func TestStream_NotReplayableStillDeliversLiveEvents(t *testing.T) {
	h := newTestServer()
	server := httptest.NewServer(h)
	defer server.Close()

	project := createProject(t, h)
	env := createEnvironment(t, h, project.ID)
	flag := createFlag(t, h, env.ID) // v1

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/environments/"+env.ID+"/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Last-Event-ID", "99") // ahead of the server: not replayable

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)

	doRequest(t, h, http.MethodPatch, "/api/v1/flags/"+flag.ID, updateFlagRequest{Enabled: boolPtr(true)}) // v2
	id, _, p2 := readSSEFrame(t, reader)
	if id != 2 || !p2.Enabled {
		t.Fatalf("live frame after non-replayable connect = (id=%d, payload=%+v), want id=2 enabled=true", id, p2)
	}
}
