package web

import (
	"context"
	"crypto/hmac"
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

	log "github.com/sirupsen/logrus"
)

const dashboardCookie = "transcoderd_dashboard"
const dashboardSessionLifetime = 12 * time.Hour
const oidcSessionLifetime = 15 * 24 * time.Hour
const oidcSessionHeader = "X-TranscoderD-OIDC-Session"
const oidcSessionPrefix = "oidc_"
const maxDashboardSessions = 1024

type dashboardSessionStore interface {
	CreateOIDCDashboardSession(ctx context.Context, credentialID, hash [32]byte, expires time.Time, limit int) (bool, error)
	HasOIDCDashboardSession(context.Context, [32]byte) (bool, error)
	DeleteOIDCDashboardSession(context.Context, [32]byte) error
}

// This boundary owns API sessions independently of token or OIDC browser login.
type dashboardAuth struct {
	mu         sync.Mutex
	sessions   map[[32]byte]time.Time
	persistent dashboardSessionStore
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

func (s *Server) oidcSessionKey(value string) [32]byte {
	// Credential rotation also revokes sessions that were issued with the old key.
	mac := hmac.New(sha256.New, []byte(s.dashboardToken()))
	_, _ = mac.Write([]byte(value))
	return [32]byte(mac.Sum(nil))
}

func (s *Server) dashboardAuthenticated(r *http.Request) (bool, error) {
	if auth := r.Header.Get("Authorization"); auth != "" {
		return strings.HasPrefix(auth, "Bearer ") && s.validDashboardToken(strings.TrimPrefix(auth, "Bearer ")), nil
	}
	cookie, err := r.Cookie(dashboardCookie)
	if err != nil || len(cookie.Value) > 128 {
		return false, nil
	}
	if strings.HasPrefix(cookie.Value, oidcSessionPrefix) {
		return s.dashboard.persistent.HasOIDCDashboardSession(r.Context(), s.oidcSessionKey(cookie.Value))
	}
	key := sha256.Sum256([]byte(cookie.Value))
	s.dashboard.mu.Lock()
	defer s.dashboard.mu.Unlock()
	expires, ok := s.dashboard.sessions[key]
	if ok && time.Now().Before(expires) {
		return true, nil
	}
	delete(s.dashboard.sessions, key)
	return false, nil
}

func (s *Server) protectDashboard(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authenticated, err := s.dashboardAuthenticated(r)
		if err != nil {
			log.WithError(err).Error("Could not validate dashboard session")
			http.Error(w, "Cannot validate your session", http.StatusServiceUnavailable)
			return
		}
		if !authenticated {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
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
	if r.Header.Get(oidcSessionHeader) == "1" {
		s.dashboardOIDCLogin(w, r, input.Token)
		return
	}
	value := rand.Text()
	now := time.Now()
	expires := now.Add(dashboardSessionLifetime)
	s.dashboard.mu.Lock()
	for key, expiry := range s.dashboard.sessions {
		if !now.Before(expiry) {
			delete(s.dashboard.sessions, key)
		}
	}
	if len(s.dashboard.sessions) >= maxDashboardSessions {
		s.dashboard.mu.Unlock()
		http.Error(w, "Too many sessions", http.StatusServiceUnavailable)
		return
	}
	s.dashboard.sessions[sha256.Sum256([]byte(value))] = expires
	s.dashboard.mu.Unlock()
	http.SetCookie(w, s.dashboardSessionCookie(r, value, expires, int(dashboardSessionLifetime.Seconds())))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) dashboardOIDCLogin(w http.ResponseWriter, r *http.Request, token string) {
	if len(token) < 32 || strings.TrimSpace(token) != token {
		http.Error(w, "Invalid OIDC API credential", http.StatusUnauthorized)
		return
	}
	value := oidcSessionPrefix + rand.Text()
	expires := time.Now().Add(oidcSessionLifetime).Truncate(time.Second)
	credentialID := s.oidcSessionKey("transcoderd oidc dashboard session credential")
	created, err := s.dashboard.persistent.CreateOIDCDashboardSession(r.Context(), credentialID, s.oidcSessionKey(value), expires, maxDashboardSessions)
	if err != nil {
		log.WithError(err).Error("Could not create OIDC dashboard session")
		http.Error(w, "Cannot open a TranscoderD session", http.StatusServiceUnavailable)
		return
	}
	if !created {
		http.Error(w, "Too many sessions", http.StatusServiceUnavailable)
		return
	}
	http.SetCookie(w, s.dashboardSessionCookie(r, value, expires, int(oidcSessionLifetime.Seconds())))
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
	if cookie, err := r.Cookie(dashboardCookie); err == nil && len(cookie.Value) <= 128 {
		if strings.HasPrefix(cookie.Value, oidcSessionPrefix) {
			if err := s.dashboard.persistent.DeleteOIDCDashboardSession(r.Context(), s.oidcSessionKey(cookie.Value)); err != nil {
				log.WithError(err).Error("Could not revoke OIDC dashboard session")
				http.Error(w, "Cannot revoke your session", http.StatusServiceUnavailable)
				return
			}
		} else {
			s.dashboard.mu.Lock()
			delete(s.dashboard.sessions, sha256.Sum256([]byte(cookie.Value)))
			s.dashboard.mu.Unlock()
		}
	}
	http.SetCookie(w, s.dashboardSessionCookie(r, "", time.Unix(1, 0), -1))
	w.WriteHeader(http.StatusNoContent)
}
