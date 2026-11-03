package web

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const dashboardCookie = "transcoderd_dashboard"
const dashboardSessionLifetime = 12 * time.Hour

// This boundary owns browser identity independently of dashboard data handlers,
// so a future OIDC provider can replace login/session handling here.
type dashboardAuth struct {
	mu       sync.Mutex
	sessions map[[32]byte]time.Time
}

func (s *Server) dashboardToken() string {
	if s.config.DashboardToken == "" {
		return "encoding123"
	}
	return s.config.DashboardToken
}

func (s *Server) validDashboardToken(token string) bool {
	got, want := sha256.Sum256([]byte(token)), sha256.Sum256([]byte(s.dashboardToken()))
	return token != "" && subtle.ConstantTimeCompare(got[:], want[:]) == 1
}

func (s *Server) dashboardAuthenticated(r *http.Request) bool {
	if auth := r.Header.Get("Authorization"); auth != "" {
		return strings.HasPrefix(auth, "Bearer ") && s.validDashboardToken(strings.TrimPrefix(auth, "Bearer "))
	}
	cookie, err := r.Cookie(dashboardCookie)
	if err != nil || len(cookie.Value) > 128 {
		return false
	}
	key := sha256.Sum256([]byte(cookie.Value))
	s.dashboard.mu.Lock()
	defer s.dashboard.mu.Unlock()
	expires, ok := s.dashboard.sessions[key]
	if ok && time.Now().Before(expires) {
		return true
	}
	delete(s.dashboard.sessions, key)
	return false
}

func (s *Server) protectDashboard(handler http.HandlerFunc, page bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.dashboardAuthenticated(r) {
			if page {
				http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
			} else {
				http.Error(w, "Authentication required", http.StatusUnauthorized)
			}
			return
		}
		handler(w, r)
	}
}

func (s *Server) dashboardMutationAllowed(w http.ResponseWriter, r *http.Request) bool {
	// A custom header prevents simple form submissions; no CORS access is granted.
	if r.Header.Get("X-Dashboard-Request") != "1" || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		http.Error(w, "Same-origin request required", http.StatusForbidden)
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		scheme := "http"
		configured, _ := url.Parse(s.config.Domain)
		if r.TLS != nil || (configured != nil && configured.Scheme == "https") {
			scheme = "https"
		}
		if err != nil || u.Host != r.Host || u.Scheme != scheme || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			http.Error(w, "Same-origin request required", http.StatusForbidden)
			return false
		}
	}
	return true
}

func decodeDashboardJSON(w http.ResponseWriter, r *http.Request, target any) error {
	return decodeLimitedJSON(w, r, target, 4096)
}

func decodeLimitedJSON(w http.ResponseWriter, r *http.Request, target any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("expected a single JSON object")
	}
	return nil
}

func (s *Server) dashboardLogin(w http.ResponseWriter, r *http.Request) {
	if !s.dashboardMutationAllowed(w, r) {
		return
	}
	var input struct {
		Token string `json:"token"`
	}
	if err := decodeDashboardJSON(w, r, &input); err != nil {
		http.Error(w, "Invalid login request", http.StatusBadRequest)
		return
	}
	if !s.validDashboardToken(input.Token) {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}
	value := rand.Text()
	expires := time.Now().Add(dashboardSessionLifetime)
	s.dashboard.mu.Lock()
	for key, expiry := range s.dashboard.sessions {
		if time.Now().After(expiry) {
			delete(s.dashboard.sessions, key)
		}
	}
	if len(s.dashboard.sessions) >= 1024 {
		s.dashboard.mu.Unlock()
		http.Error(w, "Too many sessions", http.StatusServiceUnavailable)
		return
	}
	s.dashboard.sessions[sha256.Sum256([]byte(value))] = expires
	s.dashboard.mu.Unlock()
	http.SetCookie(w, s.dashboardSessionCookie(r, value, expires, int(dashboardSessionLifetime.Seconds())))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) dashboardSessionCookie(r *http.Request, value string, expires time.Time, maxAge int) *http.Cookie {
	configured, _ := url.Parse(s.config.Domain)
	return &http.Cookie{Name: dashboardCookie, Value: value, Path: "/dashboard/", HttpOnly: true,
		Secure:   r.TLS != nil || (configured != nil && configured.Scheme == "https"),
		SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: maxAge}
}

func (s *Server) dashboardLogout(w http.ResponseWriter, r *http.Request) {
	if !s.dashboardMutationAllowed(w, r) {
		return
	}
	if cookie, err := r.Cookie(dashboardCookie); err == nil {
		s.dashboard.mu.Lock()
		delete(s.dashboard.sessions, sha256.Sum256([]byte(cookie.Value)))
		s.dashboard.mu.Unlock()
	}
	http.SetCookie(w, s.dashboardSessionCookie(r, "", time.Unix(1, 0), -1))
	w.WriteHeader(http.StatusNoContent)
}
