package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func dashboardRequest(s *Server, method, path, body, bearer string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("X-Dashboard-Request", "1")
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func newTestFrontend(t *testing.T) *Server {
	t.Helper()
	var loggedIn atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dashboard/api/login":
			var input struct{ Token string }
			_ = json.NewDecoder(r.Body).Decode(&input)
			if input.Token != "encoding123" {
				w.WriteHeader(401)
				return
			}
			loggedIn.Store(true)
			http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "test-session", HttpOnly: true, Path: "/dashboard/", SameSite: http.SameSiteStrictMode})
			w.WriteHeader(204)
		case "/dashboard/api/logout":
			loggedIn.Store(false)
			w.WriteHeader(204)
		default:
			cookie, _ := r.Cookie(sessionCookie)
			if !loggedIn.Load() || cookie == nil || cookie.Value != "test-session" {
				w.WriteHeader(401)
				return
			}
			w.WriteHeader(204)
		}
	}))
	t.Cleanup(upstream.Close)
	s, err := NewServer(Config{APIURL: upstream.URL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func loginDashboard(t *testing.T, s *Server) *http.Cookie {
	t.Helper()
	w := dashboardRequest(s, "POST", "/dashboard/api/login", `{"token":"encoding123"}`, "", nil)
	if w.Code != 204 || len(w.Result().Cookies()) != 1 {
		t.Fatalf("login = %d %s", w.Code, w.Body.String())
	}
	return w.Result().Cookies()[0]
}

func TestFrontendOriginValidation(t *testing.T) {
	for _, origin := range []string{"", "file:///media", "ftp://api.test", "http://", "http://user:secret@api.test", "http://api.test/prefix", "http://api.test?token=secret", "http://api.test?", "http://api.test#", "http://api.test/#fragment"} {
		t.Run(origin, func(t *testing.T) {
			if _, err := NewServer(Config{APIURL: origin}); err == nil {
				t.Errorf("NewServer(%q) accepted invalid origin", origin)
			}
			if _, err := NewServer(Config{APIURL: "http://api.test", PublicURL: origin}); origin != "" && err == nil {
				t.Errorf("NewServer(public=%q) accepted invalid origin", origin)
			}
		})
	}
}

func TestFrontendProxyBoundaryAndOrigin(t *testing.T) {
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != "" || r.Header.Get("Forwarded") != "" || r.Header.Get("X-Forwarded-Host") != "" || r.Header.Get("X-Forwarded-Proto") != "" {
			t.Error("proxy forwarded browser/forwarding headers to API")
		}
		if len(r.Cookies()) != 1 || r.Cookies()[0].Name != sessionCookie || r.Header.Get("Authorization") != "Bearer operator" {
			t.Error("proxy did not isolate session/authorization headers")
		}
		w.WriteHeader(204)
	}))
	defer upstream.Close()
	s, err := NewServer(Config{APIURL: upstream.URL, PublicURL: "https://ui.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	tests := []struct {
		name, method, path, origin, header, site string
		want                                     int
	}{
		{"worker API", "POST", "/api/v1/job/", "", "1", "", 404},
		{"unknown dashboard API", "POST", "/dashboard/api/arbitrary", "", "1", "", 404},
		{"traversal", "POST", "/dashboard/api/../api/login", "", "1", "", 404},
		{"missing header", "POST", "/dashboard/api/login", "https://ui.example.test", "", "", 403},
		{"foreign origin", "POST", "/dashboard/api/login", "https://evil.test", "1", "", 403},
		{"null origin", "POST", "/dashboard/api/login", "null", "1", "", 403},
		{"cross-site metadata", "POST", "/dashboard/api/login", "https://ui.example.test", "1", "cross-site", 403},
		{"preflight", "OPTIONS", "/dashboard/api/login", "https://evil.test", "1", "", 404},
		{"same origin", "PUT", "/dashboard/api/workers/cpu/settings", "https://ui.example.test", "1", "same-origin", 204},
		{"explicit default port", "PUT", "/dashboard/api/workers/cpu/settings", "https://ui.example.test:443", "1", "same-origin", 204},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, tt.path, strings.NewReader(`{}`))
			r.Header.Set("Origin", tt.origin)
			r.Header.Set("X-Dashboard-Request", tt.header)
			r.Header.Set("Sec-Fetch-Site", tt.site)
			r.Header.Set("Forwarded", "host=evil.test;proto=https")
			r.Header.Set("X-Forwarded-Host", "evil.test")
			r.Header.Set("X-Forwarded-Proto", "https")
			r.Header.Set("Authorization", "Bearer operator")
			r.AddCookie(&http.Cookie{Name: "unrelated", Value: "private"})
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "session"})
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != tt.want {
				t.Errorf("ServeHTTP() = %d %s, want %d", w.Code, w.Body.String(), tt.want)
			}
		})
	}
	if requests.Load() != 2 {
		t.Errorf("upstream requests = %d, want 2", requests.Load())
	}
}

func TestFrontendSecureCookies(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "session", Path: "/", Domain: "api.internal"})
		http.SetCookie(w, &http.Cookie{Name: "unrelated", Value: "private"})
		w.WriteHeader(204)
	}))
	defer upstream.Close()
	for _, tt := range []struct {
		name, public, request string
		want                  bool
	}{
		{"direct TLS", "", "https://ui.test/dashboard/api/login", true},
		{"TLS termination", "https://ui.test", "http://internal/dashboard/api/login", true},
		{"untrusted forwarding headers", "", "http://ui.test/dashboard/api/login", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewServer(Config{APIURL: upstream.URL, PublicURL: tt.public})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			r := httptest.NewRequest("POST", tt.request, nil)
			r.Header.Set("X-Dashboard-Request", "1")
			r.Header.Set("X-Forwarded-Proto", "https")
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			cookies := w.Result().Cookies()
			if w.Code != 204 || len(cookies) != 1 {
				t.Fatalf("login proxy = %d cookies=%v", w.Code, cookies)
			}
			c := cookies[0]
			if c.Secure != tt.want || !c.HttpOnly || c.Domain != "" || c.Path != "/dashboard/" || c.SameSite != http.SameSiteStrictMode {
				t.Errorf("cookie = %+v, want Secure=%v and frontend-scoped protected cookie", c, tt.want)
			}
		})
	}
}

func TestFrontendRejectsRedirectsAndFailsClosed(t *testing.T) {
	var escaped atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { escaped.Add(1) }))
	defer other.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, http.StatusFound) }))
	s, err := NewServer(Config{APIURL: upstream.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	defer upstream.Close()
	for _, p := range []string{"/dashboard/api/stats", "/dashboard/"} {
		w := dashboardRequest(s, "GET", p, "", "", nil)
		if w.Code < 500 || strings.Contains(w.Body.String(), "dashboard-content") {
			t.Errorf("redirected %s = %d, want unavailable without private page", p, w.Code)
		}
	}
	if escaped.Load() != 0 {
		t.Error("followed API redirect")
	}
	upstream.Close()
	for p, want := range map[string]int{"/dashboard/": 503, "/dashboard/api/stats": 502, "/dashboard/styles.css": 200, "/dashboard/login": 200} {
		w := dashboardRequest(s, "GET", p, "", "", nil)
		if w.Code != want {
			t.Errorf("unavailable API %s = %d, want %d", p, w.Code, want)
		}
	}
}

func TestFrontendStreamingCancellationAndRanges(t *testing.T) {
	canceled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/file/") {
			if r.Header.Get("Range") != "bytes=2-6" || r.Header.Get("If-Range") != `"revision"` {
				t.Error("range/conditional headers were not preserved")
			}
			w.Header().Set("Content-Range", "bytes 2-6/16")
			w.WriteHeader(206)
			_, _ = w.Write([]byte("23456"))
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("first-chunk"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(canceled)
	}))
	defer upstream.Close()
	s, err := NewServer(Config{APIURL: upstream.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	front := httptest.NewServer(s)
	defer front.Close()
	r := httptest.NewRequest("GET", "/dashboard/api/media/file/clip", nil)
	r.Header.Set("Range", "bytes=2-6")
	r.Header.Set("If-Range", `"revision"`)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 206 || w.Body.String() != "23456" || w.Header().Get("Content-Range") != "bytes 2-6/16" {
		t.Fatalf("range = %d %s %+v", w.Code, w.Body.String(), w.Header())
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", front.URL+"/dashboard/api/media/preview/clip?start=42", nil)
	response, err := front.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	chunk := make([]byte, len("first-chunk"))
	if _, err := io.ReadFull(response.Body, chunk); err != nil || string(chunk) != "first-chunk" {
		t.Fatalf("first stream chunk = %q, %v", chunk, err)
	}
	cancel()
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("frontend disconnect did not cancel API stream")
	}
}
