package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
