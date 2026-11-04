package web

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	frontend "transcoder/frontend/web"
	"transcoder/server/library"
)

func TestAPIOnlyBackendAndRemoteFrontend(t *testing.T) {
	api, _ := mediaTestServer(t)
	upstream := httptest.NewServer(api)
	defer upstream.Close()
	ui, err := frontend.NewServer(frontend.Config{APIURL: upstream.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer ui.Close()
	front := httptest.NewServer(ui)
	defer front.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	request := func(method, path, body string, headers map[string]string) (int, string, http.Header) {
		t.Helper()
		r, err := http.NewRequestWithContext(t.Context(), method, front.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Origin", front.URL)
		r.Header.Set("X-Dashboard-Request", "1")
		for key, value := range headers {
			r.Header.Set(key, value)
		}
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		content, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, string(content), response.Header
	}
	for _, p := range []string{"/", "/dashboard", "/dashboard/", "/dashboard/login", "/dashboard/app.js", "/dashboard/index.html", "/dashboard/sw.js", "/dashboard/manifest.webmanifest", "/dashboard/offline.html"} {
		if w := dashboardRequest(api, "GET", p, "", "encoding123", nil); w.Code != 404 {
			t.Errorf("API-only backend %s = %d, want 404", p, w.Code)
		}
	}
	if status, _, _ := request("GET", "/dashboard/api/session", "", nil); status != 401 {
		t.Errorf("anonymous session = %d, want 401", status)
	}
	if status, _, h := request("GET", "/dashboard/", "", nil); status != 303 || h.Get("Location") != "/dashboard/login" {
		t.Fatalf("anonymous frontend = %d %+v", status, h)
	}
	if status, _, _ := request("POST", "/dashboard/api/login", `{"token":"wrong"}`, nil); status != 401 {
		t.Errorf("wrong login = %d, want 401", status)
	}
	if status, _, h := request("POST", "/dashboard/api/login", `{"token":"encoding123"}`, nil); status != 204 || !strings.Contains(h.Get("Set-Cookie"), "HttpOnly") || !strings.Contains(h.Get("Set-Cookie"), "SameSite=Strict") {
		t.Fatalf("proxied login = %d %+v", status, h)
	}
	for _, p := range []string{"/dashboard/", "/dashboard/api/stats", "/dashboard/api/media/roots", "/dashboard/api/media/browse"} {
		if status, body, h := request("GET", p, "", nil); status != 200 || h.Get("Cache-Control") != "no-store" || strings.Contains(body, "encoding123") {
			t.Fatalf("authenticated frontend %s = %d %s", p, status, body)
		}
	}
	if status, _, _ := request("GET", "/dashboard/api/session", "", nil); status != 204 {
		t.Errorf("authenticated session = %d, want 204", status)
	}
	id := library.FileID("source", "clip.mkv")
	if status, body, h := request("GET", "/dashboard/api/media/file/"+id, "", map[string]string{"Range": "bytes=2-6"}); status != 206 || body != "23456" || h.Get("Content-Range") != "bytes 2-6/16" {
		t.Fatalf("remote original range = %d %q %+v", status, body, h)
	}
	for _, name := range []string{"remote-cpu", "node\\with spaces-ñ"} {
		if status, _, _ := request("PUT", "/dashboard/api/workers/"+url.PathEscape(name)+"/settings", `{"cpuJobs":2,"gpuJobs":3}`, nil); status != 204 {
			t.Fatalf("remote settings for %s = %d, want 204", name, status)
		}
		settings, err := api.scheduler.GetWorkerSettings(t.Context(), name)
		if err != nil || settings.CPUJobs == nil || settings.GPUJobs == nil || *settings.CPUJobs != 2 || *settings.GPUJobs != 3 {
			t.Fatalf("remote persisted settings = %+v, %v", settings, err)
		}
	}
	if status, _, _ := request("GET", "/dashboard/api/stats", "", map[string]string{"Authorization": "Bearer worker-secret"}); status != 401 {
		t.Errorf("worker credential on frontend = %d, want 401", status)
	}
	if status, _, _ := request("POST", "/dashboard/api/logout", "", nil); status != 204 {
		t.Fatalf("remote logout = %d", status)
	}
	if status, _, _ := request("GET", "/dashboard/", "", nil); status != 303 {
		t.Errorf("private HTML after logout = %d, want 303", status)
	}
	if status, _, _ := request("GET", "/dashboard/api/media/file/"+id, "", nil); status != 401 {
		t.Errorf("media after logout = %d, want 401", status)
	}
	// Expiry remains authoritative on the API; the frontend keeps no auth store.
	request("POST", "/dashboard/api/login", `{"token":"encoding123"}`, nil)
	api.dashboard.mu.Lock()
	for key := range api.dashboard.sessions {
		api.dashboard.sessions[key] = time.Now().Add(-time.Second)
	}
	api.dashboard.mu.Unlock()
	if status, _, _ := request("GET", "/dashboard/", "", nil); status != 303 {
		t.Errorf("expired session on frontend = %d, want 303", status)
	}
}
