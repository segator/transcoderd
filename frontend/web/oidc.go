package web

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const frontendSessionCookie = "transcoderd_frontend"
const oidcLoginCookie = "transcoderd_oidc_login"
const oidcSessionLifetime = 15 * 24 * time.Hour
const oidcSessionHeader = "X-TranscoderD-OIDC-Session"
const oidcLoginLifetime = 10 * time.Minute

var ErrInvalidSession = errors.New("invalid or expired frontend session")

type AuthConfig struct {
	Mode       string     `mapstructure:"mode"`
	SessionKey string     `mapstructure:"sessionKey"`
	OIDC       OIDCConfig `mapstructure:"oidc"`
}

type OIDCConfig struct {
	IssuerURL     string   `mapstructure:"issuerURL"`
	ClientID      string   `mapstructure:"clientID"`
	ClientSecret  string   `mapstructure:"clientSecret"`
	Scopes        []string `mapstructure:"scopes"`
	AllowedGroups []string `mapstructure:"allowedGroups"`
}

type oidcAuth struct {
	verifier      *oidc.IDTokenVerifier
	oauth         oauth2.Config
	seal          cipher.AEAD
	binding       string
	apiToken      string
	allowedGroups []string
}

// The API retains session revocation; the frontend stores no user/session database.
type displayIdentity struct {
	Name     string `json:"name,omitempty"`
	Username string `json:"username,omitempty"`
	Email    string `json:"email,omitempty"`
	Subject  string `json:"subject,omitempty"`
}

type authPayload struct {
	Identity *displayIdentity `json:"identity,omitempty"`
	Expires  int64            `json:"expires"`
	Session  string           `json:"session,omitempty"`
	State    string           `json:"state,omitempty"`
	Nonce    string           `json:"nonce,omitempty"`
	PKCE     string           `json:"pkce,omitempty"`
}

func secureOIDCURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(raw, "#") || u.Opaque != "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	return u.Scheme == "http" && (u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback()))
}

func (s *Server) configureAuth(config Config) error {
	mode := config.Auth.Mode
	if mode == "" || mode == "token" {
		return nil
	}
	if mode != "oidc" {
		return errors.New("frontend.auth.mode must be token or oidc")
	}
	if s.public == nil || !secureOIDCURL(s.public.String()) {
		return errors.New("OIDC requires frontend.publicURL with HTTPS (HTTP is allowed only on loopback)")
	}
	if utf8.RuneCountInString(config.APIToken) < 32 || strings.TrimSpace(config.APIToken) != config.APIToken {
		return errors.New("OIDC requires frontend.apiToken of at least 32 characters without surrounding whitespace")
	}
	key, err := base64.StdEncoding.DecodeString(config.Auth.SessionKey)
	if err != nil || len(key) != 32 {
		return errors.New("OIDC requires frontend.auth.sessionKey containing a base64-encoded 32-byte key")
	}
	cfg := config.Auth.OIDC
	if !secureOIDCURL(cfg.IssuerURL) || strings.TrimSpace(cfg.ClientID) == "" || strings.TrimSpace(cfg.ClientSecret) == "" {
		return errors.New("OIDC requires a secure issuerURL, clientID and clientSecret")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("initialize session encryption: %w", err)
	}
	seal, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("initialize session encryption: %w", err)
	}
	ctx, cancel := context.WithTimeout(oidc.ClientContext(context.Background(), s.client), 10*time.Second)
	defer cancel()
	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return errors.New("OIDC discovery failed; check issuerURL and provider availability")
	}
	var discovery struct {
		JWKS string `json:"jwks_uri"`
	}
	endpoint := provider.Endpoint()
	if err := provider.Claims(&discovery); err != nil || !secureOIDCURL(discovery.JWKS) || !secureOIDCURL(endpoint.AuthURL) || !secureOIDCURL(endpoint.TokenURL) {
		return errors.New("OIDC discovery must provide secure authorization, token and JWKS endpoints")
	}
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}
	if !slices.Contains(scopes, oidc.ScopeOpenID) {
		scopes = append([]string{oidc.ScopeOpenID}, scopes...)
	}
	s.oidc = &oidcAuth{verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}), seal: seal,
		binding:  s.public.String() + "\x00" + cfg.IssuerURL + "\x00" + cfg.ClientID,
		apiToken: config.APIToken, allowedGroups: append([]string{}, cfg.AllowedGroups...),
		oauth: oauth2.Config{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, Endpoint: endpoint,
			RedirectURL: s.public.String() + "/dashboard/auth/callback", Scopes: scopes}}
	return nil
}

func (a *oidcAuth) encode(purpose string, payload authPayload) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, a.seal.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := a.seal.Seal(nonce, nonce, raw, []byte(purpose+"\x00"+a.binding))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (a *oidcAuth) decode(r *http.Request, name string) (authPayload, error) {
	var payload authPayload
	cookie, err := r.Cookie(name)
	if err != nil || len(cookie.Value) > 4096 {
		return payload, ErrInvalidSession
	}
	raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	n := a.seal.NonceSize()
	if err != nil || len(raw) < n {
		return payload, ErrInvalidSession
	}
	raw, err = a.seal.Open(nil, raw[:n], raw[n:], []byte(name+"\x00"+a.binding))
	if err != nil || json.Unmarshal(raw, &payload) != nil || payload.Expires <= time.Now().Unix() {
		return authPayload{}, ErrInvalidSession
	}
	return payload, nil
}

func (s *Server) oidcSession(r *http.Request) (authPayload, error) {
	p, err := s.oidc.decode(r, frontendSessionCookie)
	if err != nil || p.Session == "" || len(p.Session) > 128 {
		return authPayload{}, ErrInvalidSession
	}
	return p, nil
}

func (s *Server) copyCredentials(out, in *http.Request) {
	if s.oidc == nil {
		copySession(out, in)
		out.Header.Set("Authorization", in.Header.Get("Authorization"))
		return
	}
	out.Header.Del("Authorization")
	if session, err := s.oidcSession(in); err == nil {
		out.AddCookie(&http.Cookie{Name: sessionCookie, Value: session.Session})
	}
}

func (s *Server) authCookie(r *http.Request, name, value string, expiry time.Time, maxAge int, sameSite http.SameSite) *http.Cookie {
	return &http.Cookie{Name: name, Value: value, Path: "/dashboard/", HttpOnly: true,
		Secure: s.secure(r), SameSite: sameSite, Expires: expiry, MaxAge: maxAge}
}

func (s *Server) oidcLogin(w http.ResponseWriter, r *http.Request) {
	payload := authPayload{Expires: time.Now().Add(oidcLoginLifetime).Unix(), State: rand.Text(), Nonce: rand.Text(), PKCE: oauth2.GenerateVerifier()}
	value, err := s.oidc.encode(oidcLoginCookie, payload)
	if err != nil {
		http.Error(w, "Could not start single sign-on. Please try again.", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, s.authCookie(r, oidcLoginCookie, value, time.Unix(payload.Expires, 0), int(oidcLoginLifetime.Seconds()), http.SameSiteLaxMode))
	location := s.oidc.oauth.AuthCodeURL(payload.State, oidc.Nonce(payload.Nonce), oauth2.S256ChallengeOption(payload.PKCE))
	http.Redirect(w, r, location, http.StatusSeeOther)
}

func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	transaction, err := s.oidc.decode(r, oidcLoginCookie)
	http.SetCookie(w, s.authCookie(r, oidcLoginCookie, "", time.Unix(1, 0), -1, http.SameSiteLaxMode))
	query := r.URL.Query()
	if err != nil || transaction.State == "" || transaction.Nonce == "" || transaction.PKCE == "" || query.Get("error") != "" || query.Get("code") == "" || subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(transaction.State)) != 1 {
		http.Error(w, "Single sign-on could not be verified. Return to /dashboard/login and try again.", http.StatusUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(oidc.ClientContext(r.Context(), s.client), 15*time.Second)
	defer cancel()
	token, err := s.oidc.oauth.Exchange(ctx, query.Get("code"), oauth2.VerifierOption(transaction.PKCE))
	if err != nil {
		http.Error(w, "Single sign-on failed. Return to /dashboard/login and try again.", http.StatusUnauthorized)
		return
	}
	raw, _ := token.Extra("id_token").(string)
	identity, err := s.oidc.verifier.Verify(ctx, raw)
	if err != nil || !s.oidc.identityValid(identity, transaction.Nonce) {
		http.Error(w, "Single sign-on identity could not be verified. Return to /dashboard/login and try again.", http.StatusUnauthorized)
		return
	}
	if !s.oidc.groupAllowed(identity) {
		http.Error(w, "Your account is not authorized to access this dashboard.", http.StatusForbidden)
		return
	}
	session, err := s.createAPISession(ctx)
	if err != nil {
		http.Error(w, "Cannot open a TranscoderD session. Please try again.", http.StatusBadGateway)
		return
	}
	expires := time.Now().Add(oidcSessionLifetime)
	if !session.Expires.IsZero() && session.Expires.Before(expires) {
		expires = session.Expires
	}
	var claims struct {
		Name     string `json:"name"`
		Username string `json:"preferred_username"`
		Email    string `json:"email"`
	}
	if err := identity.Claims(&claims); err != nil {
		http.Error(w, "Could not read your identity", http.StatusUnauthorized)
		return
	}
	bounded := func(value string) string {
		value = strings.TrimSpace(value)
		runes := []rune(value)
		if len(runes) > 200 {
			runes = runes[:200]
		}
		// Bound JSON expansion too (for example '<' and control characters),
		// keeping the encrypted session comfortably below browser cookie limits.
		for len(runes) > 0 {
			encoded, _ := json.Marshal(string(runes))
			if len(encoded) <= 384 {
				break
			}
			runes = runes[:len(runes)-1]
		}
		return string(runes)
	}
	display := &displayIdentity{Name: bounded(claims.Name), Username: bounded(claims.Username), Email: bounded(claims.Email), Subject: bounded(identity.Subject)}
	value, err := s.oidc.encode(frontendSessionCookie, authPayload{Session: session.Value, Expires: expires.Unix(), Identity: display})
	if err != nil {
		http.Error(w, "Could not open your session. Please try again.", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, s.authCookie(r, frontendSessionCookie, value, expires, int(time.Until(expires).Seconds()), http.SameSiteLaxMode))
	http.Redirect(w, r, "/dashboard/", http.StatusSeeOther)
}

func (a *oidcAuth) identityValid(identity *oidc.IDToken, nonce string) bool {
	if identity.Subject == "" || identity.IssuedAt.Unix() <= 0 || identity.IssuedAt.After(time.Now().Add(time.Minute)) || subtle.ConstantTimeCompare([]byte(identity.Nonce), []byte(nonce)) != 1 {
		return false
	}
	var claims struct {
		Party string `json:"azp"`
	}
	if err := identity.Claims(&claims); err != nil || (len(identity.Audience) > 1 && claims.Party == "") {
		return false
	}
	return claims.Party == "" || claims.Party == a.oauth.ClientID
}

func (a *oidcAuth) groupAllowed(identity *oidc.IDToken) bool {
	if len(a.allowedGroups) == 0 {
		return true
	}
	var claims struct {
		Groups []string `json:"groups"`
	}
	if err := identity.Claims(&claims); err != nil {
		return false
	}
	for _, group := range claims.Groups {
		if slices.Contains(a.allowedGroups, group) {
			return true
		}
	}
	return false
}

func (s *Server) createAPISession(ctx context.Context) (*http.Cookie, error) {
	u := *s.api
	u.Path = "/dashboard/api/login"
	input, _ := json.Marshal(struct {
		Token string `json:"token"`
	}{s.oidc.apiToken})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader(string(input)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Dashboard-Request", "1")
	req.Header.Set(oidcSessionHeader, "1")
	response, err := s.client.Do(req)
	if err != nil {
		return nil, errors.New("API login unavailable")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode == http.StatusNoContent {
		for _, cookie := range response.Cookies() {
			if cookie.Name == sessionCookie && cookie.Value != "" && len(cookie.Value) <= 128 && cookie.MaxAge >= 0 && (cookie.Expires.IsZero() || cookie.Expires.After(time.Now())) {
				return cookie, nil
			}
		}
	}
	return nil, errors.New("API login did not create a session")
}
