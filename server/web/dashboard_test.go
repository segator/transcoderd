package web

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func dashboardRequest(s *Server, method, path, body, bearer string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	r.Header.Set("X-Dashboard-Request", "1")
	w := httptest.NewRecorder()
	s.srv.Handler.ServeHTTP(w, r)
	return w
}

func loginDashboard(t *testing.T, s *Server) *http.Cookie {
	t.Helper()
	w := dashboardRequest(s, http.MethodPost, "/dashboard/api/login", `{"token":"encoding123"}`, "", nil)
	if w.Code != http.StatusNoContent || len(w.Result().Cookies()) != 1 {
		t.Fatalf("dashboardLogin() = %d %s, want 204 and cookie", w.Code, w.Body.String())
	}
	return w.Result().Cookies()[0]
}

func TestDashboardAuthenticationAndCredentialSeparation(t *testing.T) {
	s := newWorkerSettingsTestServer(t)
	tests := []struct {
		name, method, path, token, body string
		want                            int
	}{
		{name: "anonymous page", method: "GET", path: "/dashboard/", want: 404},
		{name: "anonymous stats", method: "GET", path: "/dashboard/api/stats", want: 401},
		{name: "anonymous write", method: "PUT", path: "/dashboard/api/workers/cpu/settings", body: `{"maxActiveJobs":0}`, want: 401},
		{name: "worker token rejected", method: "GET", path: "/dashboard/api/stats", token: "test-token", want: 401},
		{name: "dashboard bearer", method: "GET", path: "/dashboard/api/stats", token: "encoding123", want: 200},
		{name: "dashboard token cannot access worker API", method: "GET", path: "/api/v1/worker/settings", token: "encoding123", want: 401},
		{name: "wrong login", method: "POST", path: "/dashboard/api/login", body: `{"token":"wrong"}`, want: 401},
		{name: "empty login", method: "POST", path: "/dashboard/api/login", body: `{"token":""}`, want: 401},
		{name: "extra login fields", method: "POST", path: "/dashboard/api/login", body: `{"token":"encoding123","x":1}`, want: 400},
		{name: "multiple objects", method: "POST", path: "/dashboard/api/login", body: `{"token":"encoding123"} {}`, want: 400},
		{name: "bounded login", method: "POST", path: "/dashboard/api/login", body: `{"token":"` + strings.Repeat("a", 5000) + `"}`, want: 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := dashboardRequest(s, tt.method, tt.path, tt.body, tt.token, nil)
			if w.Code != tt.want {
				t.Fatalf("dashboard %s %s = %d %q, want %d", tt.method, tt.path, w.Code, w.Body.String(), tt.want)
			}
		})
	}
	cookie := loginDashboard(t, s)
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/dashboard/" || cookie.MaxAge != 43200 {
		t.Errorf("session cookie = %#v, want HttpOnly SameSiteStrict 12-hour dashboard cookie", cookie)
	}
	for _, path := range []string{"/dashboard/api/stats"} {
		w := dashboardRequest(s, "GET", path, "", "", cookie)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("authenticated %s = %d, want 200 without caching", path, w.Code)
		}
		if strings.Contains(w.Body.String(), "encoding123") || strings.Contains(w.Body.String(), "test-token") {
			t.Error("dashboard response exposes a token")
		}
	}
	if w := dashboardRequest(s, "POST", "/dashboard/api/logout", "", "", cookie); w.Code != 204 {
		t.Fatalf("logout = %d, want 204", w.Code)
	}
	if w := dashboardRequest(s, "GET", "/dashboard/api/stats", "", "", cookie); w.Code != 401 {
		t.Fatalf("replayed logged-out session = %d, want 401", w.Code)
	}
	cookie = loginDashboard(t, s)
	s.dashboard.mu.Lock()
	s.dashboard.sessions[sha256.Sum256([]byte(cookie.Value))] = time.Now().Add(-time.Second)
	s.dashboard.mu.Unlock()
	if w := dashboardRequest(s, "GET", "/dashboard/api/stats", "", "", cookie); w.Code != 401 {
		t.Fatalf("expired session = %d, want 401", w.Code)
	}
}

func TestDashboardCustomTokenAndSecureCookie(t *testing.T) {
	s := newWorkerSettingsTestServer(t)
	s.config.DashboardToken = "custom-private-token"
	s.config.Domain = "https://transcoder.example.com"
	if w := dashboardRequest(s, "GET", "/dashboard/api/stats", "", "encoding123", nil); w.Code != 401 {
		t.Fatalf("default token after customization = %d, want 401", w.Code)
	}
	w := dashboardRequest(s, "POST", "/dashboard/api/login", `{"token":"custom-private-token"}`, "", nil)
	if w.Code != 204 || !w.Result().Cookies()[0].Secure {
		t.Fatalf("HTTPS login = %d, want secure cookie", w.Code)
	}
}

func TestDashboardRejectsCrossOriginMutations(t *testing.T) {
	s := newWorkerSettingsTestServer(t)
	cookie := loginDashboard(t, s)
	for _, path := range []string{"/dashboard/api/login", "/dashboard/api/logout", "/dashboard/api/workers/gpu/settings"} {
		method, body := "POST", `{"token":"encoding123"}`
		if strings.Contains(path, "workers") {
			method, body = "PUT", `{"maxActiveJobs":0}`
		}
		for _, origin := range []string{"https://evil.example", "null", "http://example.com.evil"} {
			r := httptest.NewRequest(method, path, strings.NewReader(body))
			r.AddCookie(cookie)
			r.Header.Set("Origin", origin)
			r.Header.Set("X-Dashboard-Request", "1")
			w := httptest.NewRecorder()
			s.srv.Handler.ServeHTTP(w, r)
			if w.Code != 403 {
				t.Errorf("cross-origin %s %s = %d, want 403", origin, path, w.Code)
			}
		}
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		s.srv.Handler.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Errorf("missing request header %s = %d, want 403", path, w.Code)
		}
	}
}

func TestDashboardWorkerLimitsAndValidation(t *testing.T) {
	s := newWorkerSettingsTestServer(t)
	cookie := loginDashboard(t, s)
	for name, max := range map[string]int{"gpu-5080": 3, "cpu-server": 0} {
		body, _ := json.Marshal(map[string]int{"maxActiveJobs": max})
		w := dashboardRequest(s, "PUT", "/dashboard/api/workers/"+name+"/settings", string(body), "", cookie)
		if w.Code != 204 {
			t.Fatalf("save %s = %d %s, want 204", name, w.Code, w.Body.String())
		}
		got, err := s.scheduler.GetWorkerMaxActiveJobs(t.Context(), name)
		if err != nil || got == nil || *got != max {
			t.Errorf("saved %s = %v, %v; want %d", name, got, err, max)
		}
	}
	for _, body := range []string{`{}`, `null`, `{"maxActiveJobs":-1}`, `{"maxActiveJobs":1.1}`, `{"maxActiveJobs":"2"}`, `{"maxActiveJobs":1} {}`, `{"maxActiveJobs":1,"extra":1}`} {
		w := dashboardRequest(s, "PUT", "/dashboard/api/workers/gpu-5080/settings", body, "", cookie)
		if w.Code != 400 {
			t.Errorf("invalid body %s = %d, want 400", body, w.Code)
		}
	}
	for _, hours := range []string{"1", "-24", "garbage", "9999999999999999999999"} {
		if w := dashboardRequest(s, "GET", "/dashboard/api/stats?hours="+hours, "", "", cookie); w.Code != 400 {
			t.Errorf("invalid period %s = %d, want 400", hours, w.Code)
		}
	}
	for _, hours := range []string{"24", "168", "720"} {
		if w := dashboardRequest(s, "GET", "/dashboard/api/stats?hours="+hours, "", "", cookie); w.Code != 200 {
			t.Errorf("period %s = %d, want 200", hours, w.Code)
		}
	}
	for _, asset := range []string{"app.js", "login.js", "styles.css"} {
		w := dashboardRequest(s, "GET", "/dashboard/"+asset, "", "", nil)
		if w.Code != 404 {
			t.Errorf("embedded asset %s = %d, want 404 on API-only backend", asset, w.Code)
		}
	}
	if w := dashboardRequest(s, "GET", "/dashboard/index.html", "", "", nil); w.Code != 404 {
		t.Errorf("direct dashboard HTML = %d, want 404", w.Code)
	}
}

func TestDashboardJobsQueryValidation(t *testing.T) {
	s := newWorkerSettingsTestServer(t)
	cookie := loginDashboard(t, s)
	tests := []struct {
		query string
		want  int
	}{
		{"", 200}, {"?state=completed&offset=0&limit=25&search=movie", 200},
		{"?state=invalid", 400}, {"?offset=-1", 400}, {"?offset=abc", 400}, {"?limit=0", 400}, {"?limit=101", 400}, {"?search=" + strings.Repeat("x", 257), 400},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			w := dashboardRequest(s, "GET", "/dashboard/api/jobs"+tt.query, "", "", cookie)
			if w.Code != tt.want {
				t.Fatalf("dashboardJobs() = %d %s, want %d", w.Code, w.Body.String(), tt.want)
			}
			if tt.want == 200 && (w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `"jobs":[]`)) {
				t.Errorf("dashboardJobs() = %s, headers %v", w.Body.String(), w.Header())
			}
		})
	}
	if got := dashboardRequest(s, "GET", "/dashboard/api/jobs", "", "", nil); got.Code != 401 {
		t.Fatalf("anonymous jobs = %d", got.Code)
	}
}

func TestDashboardRangeValidation(t *testing.T) {
	s := newWorkerSettingsTestServer(t)
	tests := []struct {
		query string
		want  int
	}{
		{"start=2026-10-01T00:00:00Z&end=2026-10-03T00:00:00Z", 200},
		{"start=2026-10-01T00:00:00Z", 400},
		{"end=2026-10-03T00:00:00Z", 400},
		{"start=bad&end=2026-10-03T00:00:00Z", 400},
		{"start=2026-10-03T00:00:00Z&end=2026-10-01T00:00:00Z", 400},
		{"start=2026-10-01T00:00:00Z&end=2026-10-01T00:00:00Z", 400},
		{"start=2000-10-01T00:00:00Z&end=2026-10-01T00:00:00Z", 400},
		{"hours=24&start=2026-10-01T00:00:00Z&end=2026-10-03T00:00:00Z", 400},
	}
	for _, tt := range tests {
		w := dashboardRequest(s, "GET", "/dashboard/api/stats?"+tt.query, "", "encoding123", nil)
		if w.Code != tt.want {
			t.Errorf("range %s = %d %s, want %d", tt.query, w.Code, w.Body.String(), tt.want)
		}
	}
}
