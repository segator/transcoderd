package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"transcoder/server/repository"
	"transcoder/server/scheduler"
)

func newWorkerSettingsTestServer(t *testing.T) *Server {
	t.Helper()
	repo := repository.NewInMemoryRepository()
	sch, err := scheduler.NewScheduler(&scheduler.Config{}, repo)
	if err != nil {
		t.Fatal(err)
	}
	server := NewWebServer(&Config{Token: "test-token"}, sch, nil)
	server.ctx = context.Background()
	return server
}

func TestWorkerSettingsRoutesPersistPerWorker(t *testing.T) {
	server := newWorkerSettingsTestServer(t)

	get := httptest.NewRequest(http.MethodGet, "/api/v1/worker/settings", nil)
	get.Header.Set("Authorization", "Bearer test-token")
	get.Header.Set("workerName", "gpu-worker")
	response := httptest.NewRecorder()
	server.srv.Handler.ServeHTTP(response, get)
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"maxActiveJobs":null}` {
		t.Fatalf("GET settings = %d %q, want 200 with null setting", response.Code, response.Body.String())
	}

	put := httptest.NewRequest(http.MethodPut, "/api/v1/worker/settings", strings.NewReader(`{"maxActiveJobs":0}`))
	put.Header.Set("Authorization", "Bearer test-token")
	put.Header.Set("workerName", "gpu-worker")
	response = httptest.NewRecorder()
	server.srv.Handler.ServeHTTP(response, put)
	if response.Code != http.StatusNoContent {
		t.Fatalf("PUT settings status = %d, want 204: %s", response.Code, response.Body.String())
	}

	get = httptest.NewRequest(http.MethodGet, "/api/v1/worker/settings", nil)
	get.Header.Set("Authorization", "Bearer test-token")
	get.Header.Set("workerName", "gpu-worker")
	response = httptest.NewRecorder()
	server.srv.Handler.ServeHTTP(response, get)
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"maxActiveJobs":0}` {
		t.Fatalf("GET persisted settings = %d %q, want 200 with zero", response.Code, response.Body.String())
	}

	get.Header.Set("workerName", "cpu-worker")
	response = httptest.NewRecorder()
	server.srv.Handler.ServeHTTP(response, get)
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"maxActiveJobs":null}` {
		t.Fatalf("GET other worker settings = %d %q, want no setting", response.Code, response.Body.String())
	}
}

func TestWorkerSettingsRoutesRequireAuthenticationAndValidateInput(t *testing.T) {
	server := newWorkerSettingsTestServer(t)

	tests := []struct {
		name   string
		token  string
		worker string
		body   string
		want   int
	}{
		{name: "unauthorized", worker: "gpu-worker", body: `{"maxActiveJobs":1}`, want: http.StatusUnauthorized},
		{name: "missing worker", token: "Bearer test-token", body: `{"maxActiveJobs":1}`, want: http.StatusBadRequest},
		{name: "missing maximum", token: "Bearer test-token", worker: "gpu-worker", body: `{}`, want: http.StatusBadRequest},
		{name: "negative maximum", token: "Bearer test-token", worker: "gpu-worker", body: `{"maxActiveJobs":-1}`, want: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPut, "/api/v1/worker/settings", strings.NewReader(tt.body))
			if tt.token != "" {
				request.Header.Set("Authorization", tt.token)
			}
			if tt.worker != "" {
				request.Header.Set("workerName", tt.worker)
			}
			response := httptest.NewRecorder()
			server.srv.Handler.ServeHTTP(response, request)
			if response.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.want, response.Body.String())
			}
		})
	}
}
