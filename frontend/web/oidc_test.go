package web

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
)

const testAPICredential = "private-api-credential-with-at-least-32-characters"
const testOIDCSecret = "private-oidc-client-credential"

type testCode struct{ nonce, challenge string }
type testOIDCProvider struct {
	server           *httptest.Server
	mu               sync.Mutex
	codes            map[string]testCode
	modify           func(map[string]any)
	invalidSignature bool
}

func newOIDCProvider(t *testing.T) *testOIDCProvider {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: key}, new(jose.SignerOptions).WithHeader("kid", "test-key"))
	if err != nil {
		t.Fatal(err)
	}
	p := &testOIDCProvider{codes: map[string]testCode{}}
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": p.server.URL, "authorization_endpoint": p.server.URL + "/authorize", "token_endpoint": p.server.URL + "/token", "jwks_uri": p.server.URL + "/keys", "id_token_signing_alg_values_supported": []string{"ES256"}})
		case "/keys":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "test-key", Algorithm: "ES256", Use: "sig"}}})
		case "/authorize":
			q := r.URL.Query()
			if q.Get("client_id") != "frontend-client" || q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" || q.Get("nonce") == "" || q.Get("code_challenge") == "" || !strings.Contains(q.Get("scope"), "openid") {
				http.Error(w, "invalid authorization request", http.StatusBadRequest)
				return
			}
			code := rand.Text()
			p.mu.Lock()
			p.codes[code] = testCode{q.Get("nonce"), q.Get("code_challenge")}
			p.mu.Unlock()
			callback, err := url.Parse(q.Get("redirect_uri"))
			if err != nil {
				http.Error(w, "invalid callback", http.StatusBadRequest)
				return
			}
			query := callback.Query()
			query.Set("state", q.Get("state"))
			query.Set("code", code)
			callback.RawQuery = query.Encode()
			http.Redirect(w, r, callback.String(), http.StatusSeeOther)
		case "/token":
			_ = r.ParseForm()
			id, secret, _ := r.BasicAuth()
			if id == "" {
				id, secret = r.Form.Get("client_id"), r.Form.Get("client_secret")
			}
			p.mu.Lock()
			code, ok := p.codes[r.Form.Get("code")]
			delete(p.codes, r.Form.Get("code"))
			p.mu.Unlock()
			challenge := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if !ok || id != "frontend-client" || secret != testOIDCSecret || r.Form.Get("grant_type") != "authorization_code" || base64.RawURLEncoding.EncodeToString(challenge[:]) != code.challenge {
				http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
				return
			}
			claims := map[string]any{"iss": p.server.URL, "sub": "operator-123", "aud": "frontend-client", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(), "nonce": code.nonce, "groups": []string{"operators"}}
			if p.modify != nil {
				p.modify(claims)
			}
			raw, _ := json.Marshal(claims)
			signed, err := signer.Sign(raw)
			if err != nil {
				http.Error(w, "signing failed", http.StatusInternalServerError)
				return
			}
			token, err := signed.CompactSerialize()
			if err != nil {
				http.Error(w, "signing failed", http.StatusInternalServerError)
				return
			}
			if p.invalidSignature {
				parts := strings.Split(token, ".")
				parts[2] = base64.RawURLEncoding.EncodeToString(make([]byte, 64))
				token = strings.Join(parts, ".")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"token_type": "Bearer", "access_token": "provider-access-token", "id_token": token, "expires_in": 3600})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(p.server.Close)
	return p
}

type testSessionAPI struct {
	server    *httptest.Server
	mu        sync.Mutex
	sessions  map[string]bool
	logins    atomic.Int32
	available atomic.Bool
}

func newSessionAPI(t *testing.T) *testSessionAPI {
	t.Helper()
	a := &testSessionAPI{sessions: map[string]bool{}}
	a.available.Store(true)
	a.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.available.Load() {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("OIDC proxy forwarded a bearer credential")
		}
		if r.URL.Path == "/dashboard/api/login" {
			var input struct{ Token string }
			_ = json.NewDecoder(r.Body).Decode(&input)
			if input.Token != testAPICredential || r.Header.Get("X-Dashboard-Request") != "1" {
				http.Error(w, "invalid login", http.StatusUnauthorized)
				return
			}
			a.logins.Add(1)
			session := rand.Text()
			a.mu.Lock()
			a.sessions[session] = true
			a.mu.Unlock()
			http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: session, Expires: time.Now().Add(oidcSessionLifetime), HttpOnly: true, Path: "/dashboard/"})
			w.WriteHeader(204)
			return
		}
		cookie, _ := r.Cookie(sessionCookie)
		a.mu.Lock()
		valid := cookie != nil && a.sessions[cookie.Value]
		a.mu.Unlock()
		if !valid {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/dashboard/api/logout" {
			a.mu.Lock()
			delete(a.sessions, cookie.Value)
			a.mu.Unlock()
			w.WriteHeader(204)
			return
		}
		if r.URL.Path == "/dashboard/api/session" || r.Method == "PUT" {
			w.WriteHeader(204)
			return
		}
		if strings.Contains(r.URL.Path, "/media/file/") {
			if r.Header.Get("Range") != "bytes=2-6" {
				t.Error("OIDC proxy lost media range")
			}
			w.Header().Set("Content-Range", "bytes 2-6/16")
			w.WriteHeader(206)
			_, _ = w.Write([]byte("23456"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"summary":{}}`))
	}))
	t.Cleanup(a.server.Close)
	return a
}

func oidcTestConfig(p *testOIDCProvider, a *testSessionAPI) Config {
	return Config{APIURL: a.server.URL, PublicURL: "https://frontend.example.test", APIToken: testAPICredential,
		Auth: AuthConfig{Mode: "oidc", SessionKey: base64.StdEncoding.EncodeToString(make([]byte, 32)), OIDC: OIDCConfig{IssuerURL: p.server.URL, ClientID: "frontend-client", ClientSecret: testOIDCSecret, AllowedGroups: []string{"operators"}}}}
}

func oidcTestFrontend(t *testing.T, config Config) *Server {
	t.Helper()
	s, err := NewServer(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func oidcAuthorization(t *testing.T, s *Server, p *testOIDCProvider) (string, *http.Cookie) {
	t.Helper()
	start := dashboardRequest(s, "GET", "/dashboard/auth/login", "", "", nil)
	if start.Code != 303 || len(start.Result().Cookies()) != 1 {
		t.Fatalf("OIDC start = %d", start.Code)
	}
	cookie := start.Result().Cookies()[0]
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge != 600 {
		t.Fatal("OIDC transaction cookie lacks cross-site callback protections")
	}
	location, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if location.Query().Get("redirect_uri") != "https://frontend.example.test/dashboard/auth/callback" {
		t.Fatal("OIDC callback did not use configured public HTTPS origin")
	}
	r := httptest.NewRequest("GET", location.String(), nil)
	w := httptest.NewRecorder()
	p.server.Config.Handler.ServeHTTP(w, r)
	if w.Code != 303 {
		t.Fatalf("provider authorization = %d", w.Code)
	}
	return w.Header().Get("Location"), cookie
}

func oidcLoginSession(t *testing.T, s *Server, p *testOIDCProvider) *http.Cookie {
	t.Helper()
	callback, cookie := oidcAuthorization(t, s, p)
	w := dashboardRequest(s, "GET", callback, "", "", cookie)
	if w.Code != 303 || w.Header().Get("Location") != "/dashboard/" {
		t.Fatalf("OIDC callback = %d %s", w.Code, w.Body.String())
	}
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == frontendSessionCookie {
			if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Domain != "" || cookie.Path != "/dashboard/" || cookie.MaxAge <= 0 {
				t.Fatal("OIDC session cookie lacks required protections")
			}
			return cookie
		}
	}
	t.Fatal("OIDC callback omitted frontend session")
	return nil
}

func TestOIDCSessionsAcrossReplicasAndRevocation(t *testing.T) {
	p, a := newOIDCProvider(t), newSessionAPI(t)
	config := oidcTestConfig(p, a)
	first := oidcTestFrontend(t, config)
	second := oidcTestFrontend(t, config)
	page := dashboardRequest(first, "GET", "/dashboard/login", "", "", nil)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "Sign in with SSO") || strings.Contains(page.Body.String(), `id="token"`) {
		t.Fatal("OIDC login page did not select SSO-only UI")
	}
	callback, transaction := oidcAuthorization(t, first, p)
	result := dashboardRequest(second, "GET", callback, "", "", transaction)
	if result.Code != 303 {
		t.Fatalf("cross-replica callback = %d %s", result.Code, result.Body.String())
	}
	var cookie *http.Cookie
	for _, c := range result.Result().Cookies() {
		if c.Name == frontendSessionCookie {
			cookie = c
		}
		if c.Name == sessionCookie {
			t.Fatal("API session cookie escaped OIDC encryption")
		}
	}
	if cookie == nil {
		t.Fatal("no frontend cookie")
	}
	for _, s := range []*Server{first, second} {
		for path, want := range map[string]int{"/dashboard/": 200, "/dashboard/api/session": 204, "/dashboard/api/stats": 200} {
			w := dashboardRequest(s, "GET", path, "", "", cookie)
			if w.Code != want {
				t.Errorf("OIDC %s = %d, want %d", path, w.Code, want)
			}
			for _, secret := range []string{testAPICredential, testOIDCSecret, "provider-access-token", cookie.Value} {
				if strings.Contains(w.Body.String(), secret) {
					t.Error("response exposed an auth secret")
				}
			}
		}
	}
	if w := dashboardRequest(second, "GET", callback, "", "", transaction); w.Code != 401 {
		t.Fatalf("replayed code = %d, want 401", w.Code)
	}
	r := httptest.NewRequest("GET", "/dashboard/api/media/file/movie", nil)
	r.AddCookie(cookie)
	r.Header.Set("Range", "bytes=2-6")
	w := httptest.NewRecorder()
	second.ServeHTTP(w, r)
	if w.Code != 206 || w.Body.String() != "23456" || w.Header().Get("Content-Range") != "bytes 2-6/16" {
		t.Fatal("OIDC media range did not survive proxy")
	}
	w = dashboardRequest(second, "POST", "/dashboard/api/logout", "", "", cookie)
	if w.Code != 204 || !strings.Contains(w.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("OIDC logout = %d, want revoked and expired cookie", w.Code)
	}
	if w := dashboardRequest(first, "GET", "/dashboard/api/stats", "", "", cookie); w.Code != 401 {
		t.Fatalf("replayed cookie after logout = %d", w.Code)
	}
	cookie = oidcLoginSession(t, first, p)
	p.server.Close()
	if w := dashboardRequest(second, "GET", "/dashboard/api/stats", "", "", cookie); w.Code != 200 {
		t.Fatal("provider outage interrupted an existing API session")
	}
	a.mu.Lock()
	a.sessions = map[string]bool{}
	a.mu.Unlock()
	if w := dashboardRequest(second, "GET", "/dashboard/", "", "", cookie); w.Code != 303 || w.Header().Get("Location") != "/dashboard/login" {
		t.Fatal("API restart did not revoke cross-replica session")
	}
}

func TestOIDCRejectsUnverifiedIdentities(t *testing.T) {
	tests := []struct {
		name             string
		modify           func(map[string]any)
		invalidSignature bool
		want             int
	}{
		{"issuer", func(c map[string]any) { c["iss"] = "https://other.example" }, false, 401},
		{"audience", func(c map[string]any) { c["aud"] = "another-client" }, false, 401},
		{"expiry", func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() }, false, 401},
		{"nonce", func(c map[string]any) { c["nonce"] = "another-login" }, false, 401},
		{"subject", func(c map[string]any) { c["sub"] = "" }, false, 401},
		{"authorized party", func(c map[string]any) { c["azp"] = "another-client" }, false, 401},
		{"multiple audiences without party", func(c map[string]any) { c["aud"] = []string{"frontend-client", "other"} }, false, 401},
		{"future issuance", func(c map[string]any) { c["iat"] = time.Now().Add(time.Hour).Unix() }, false, 401},
		{"group missing", func(c map[string]any) { delete(c, "groups") }, false, 403},
		{"group wrong", func(c map[string]any) { c["groups"] = []string{"visitors"} }, false, 403},
		{"signature", nil, true, 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, a := newOIDCProvider(t), newSessionAPI(t)
			p.modify, p.invalidSignature = tt.modify, tt.invalidSignature
			s := oidcTestFrontend(t, oidcTestConfig(p, a))
			callback, cookie := oidcAuthorization(t, s, p)
			w := dashboardRequest(s, "GET", callback, "", "", cookie)
			if w.Code != tt.want || a.logins.Load() != 0 {
				t.Fatalf("invalid identity = %d, API logins=%d; want %d,0", w.Code, a.logins.Load(), tt.want)
			}
		})
	}
}

func TestOIDCRejectsCallbacksAndBypasses(t *testing.T) {
	p, a := newOIDCProvider(t), newSessionAPI(t)
	s := oidcTestFrontend(t, oidcTestConfig(p, a))
	for _, mode := range []string{"missing cookie", "state", "expired", "PKCE", "provider error"} {
		t.Run(mode, func(t *testing.T) {
			callback, cookie := oidcAuthorization(t, s, p)
			u, _ := url.Parse(callback)
			q := u.Query()
			switch mode {
			case "missing cookie":
				cookie = nil
			case "state":
				q.Set("state", "unrelated")
			case "provider error":
				q.Set("error", "access_denied")
			default:
				r := httptest.NewRequest("GET", "/", nil)
				r.AddCookie(cookie)
				payload, err := s.oidc.decode(r, oidcLoginCookie)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "expired" {
					payload.Expires = time.Now().Add(-time.Minute).Unix()
				} else {
					payload.PKCE = "unrelated-verifier"
				}
				cookie.Value, err = s.oidc.encode(oidcLoginCookie, payload)
				if err != nil {
					t.Fatal(err)
				}
			}
			u.RawQuery = q.Encode()
			w := dashboardRequest(s, "GET", u.String(), "", "", cookie)
			if w.Code != 401 || a.logins.Load() != 0 {
				t.Fatalf("invalid callback = %d; API logins=%d", w.Code, a.logins.Load())
			}
		})
	}
	for _, path := range []string{"/dashboard/api/stats", "/dashboard/api/session", "/dashboard/api/media/file/video"} {
		w := dashboardRequest(s, "GET", path, "", testAPICredential, &http.Cookie{Name: sessionCookie, Value: "legacy-api-session"})
		if w.Code != 401 {
			t.Errorf("OIDC bypass %s = %d", path, w.Code)
		}
	}
	if w := dashboardRequest(s, "POST", "/dashboard/api/login", `{"token":"`+testAPICredential+`"}`, "", nil); w.Code != 403 {
		t.Fatal("OIDC accepted token fallback")
	}
	cookie := oidcLoginSession(t, s, p)
	mutated := *cookie
	mutated.Value = "A" + mutated.Value[1:]
	if mutated.Value == cookie.Value {
		mutated.Value = "B" + mutated.Value[1:]
	}
	if w := dashboardRequest(s, "GET", "/dashboard/api/stats", "", "", &mutated); w.Code != 401 {
		t.Fatal("tampered session was accepted")
	}
	r := httptest.NewRequest("PUT", "/dashboard/api/workers/cpu/settings", strings.NewReader(`{"cpuJobs":1,"gpuJobs":0}`))
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://attacker.test")
	r.Header.Set("X-Dashboard-Request", "1")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("OIDC mutation lost origin protection")
	}
	a.available.Store(false)
	if w := dashboardRequest(s, "GET", "/dashboard/", "", "", cookie); w.Code != 503 {
		t.Fatal("OIDC served private HTML while API unavailable")
	}
}

func TestOIDCConfigurationFailsClosed(t *testing.T) {
	p, a := newOIDCProvider(t), newSessionAPI(t)
	tests := []struct {
		name   string
		change func(*Config)
	}{
		{"unknown mode", func(c *Config) { c.Auth.Mode = "invalid" }},
		{"missing public origin", func(c *Config) { c.PublicURL = "" }},
		{"HTTP public origin", func(c *Config) { c.PublicURL = "http://frontend.example.test" }},
		{"short API key", func(c *Config) { c.APIToken = "encoding123" }},
		{"short Unicode API key", func(c *Config) { c.APIToken = strings.Repeat("ñ", 16) }},
		{"invalid session key", func(c *Config) { c.Auth.SessionKey = "invalid" }},
		{"short decoded key", func(c *Config) { c.Auth.SessionKey = base64.StdEncoding.EncodeToString(make([]byte, 16)) }},
		{"HTTP issuer", func(c *Config) { c.Auth.OIDC.IssuerURL = "http://provider.example.test/" }},
		{"issuer credentials", func(c *Config) { c.Auth.OIDC.IssuerURL = "https://user:secret@provider.example.test/" }},
		{"issuer query", func(c *Config) { c.Auth.OIDC.IssuerURL = p.server.URL + "?secret=hidden" }},
		{"empty issuer query", func(c *Config) { c.Auth.OIDC.IssuerURL = p.server.URL + "?" }},
		{"empty issuer fragment", func(c *Config) { c.Auth.OIDC.IssuerURL = p.server.URL + "#" }},
		{"missing client ID", func(c *Config) { c.Auth.OIDC.ClientID = "" }},
		{"missing client secret", func(c *Config) { c.Auth.OIDC.ClientSecret = "" }},
		{"discovery fails", func(c *Config) { c.Auth.OIDC.IssuerURL = a.server.URL }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := oidcTestConfig(p, a)
			tt.change(&config)
			s, err := NewServer(config)
			if err == nil {
				s.Close()
				t.Fatal("invalid OIDC configuration accepted")
			}
			if strings.Contains(err.Error(), testAPICredential) || strings.Contains(err.Error(), testOIDCSecret) {
				t.Fatal("configuration error exposed secrets")
			}
		})
	}
}

func TestOIDCSessionExpiryAndKeyRotation(t *testing.T) {
	p, a := newOIDCProvider(t), newSessionAPI(t)
	config := oidcTestConfig(p, a)
	s := oidcTestFrontend(t, config)
	cookie := oidcLoginSession(t, s, p)
	rotated := config
	rotated.Auth.SessionKey = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32)))
	other := oidcTestFrontend(t, rotated)
	if w := dashboardRequest(other, "GET", "/dashboard/api/stats", "", "", cookie); w.Code != 401 {
		t.Fatal("rotated key accepted old cookie")
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(cookie)
	payload, err := s.oidcSession(r)
	if err != nil {
		t.Fatal(err)
	}
	payload.Expires = time.Now().Add(-time.Second).Unix()
	cookie.Value, err = s.oidc.encode(frontendSessionCookie, payload)
	if err != nil {
		t.Fatal(err)
	}
	if w := dashboardRequest(s, "GET", "/dashboard/api/stats", "", "", cookie); w.Code != 401 {
		t.Fatal("expired frontend session accepted")
	}
	for _, path := range []string{"/dashboard/sw.js", "/dashboard/manifest.webmanifest", "/dashboard/login"} {
		w := dashboardRequest(s, "GET", path, "", "", nil)
		raw, err := io.ReadAll(w.Result().Body)
		if err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || strings.Contains(string(raw), testAPICredential) || strings.Contains(string(raw), testOIDCSecret) {
			t.Errorf("OIDC public/PWA resource %s failed privacy", path)
		}
	}
}

func TestOIDCDisplayIdentity(t *testing.T) {
	p, a := newOIDCProvider(t), newSessionAPI(t)
	p.modify = func(claims map[string]any) {
		claims["name"] = "Isaac Aymerich"
		claims["preferred_username"] = "isaac"
		claims["email"] = "isaac@example.test"
	}
	config := oidcTestConfig(p, a)
	first, second := oidcTestFrontend(t, config), oidcTestFrontend(t, config)
	cookie := oidcLoginSession(t, first, p)
	for _, s := range []*Server{first, second} {
		w := dashboardRequest(s, "GET", "/dashboard/api/identity", "", "", cookie)
		var got struct{ Mode, Name, Username, Email string }
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || got.Mode != "oidc" || got.Name != "Isaac Aymerich" || got.Username != "isaac" || got.Email != "isaac@example.test" {
			t.Fatalf("identity = %d %+v", w.Code, got)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("identity response is cacheable")
		}
		for _, secret := range []string{testAPICredential, testOIDCSecret, "provider-access-token", cookie.Value} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("identity exposes credentials")
			}
		}
	}
	if w := dashboardRequest(second, "GET", "/dashboard/api/identity", "", "", nil); w.Code != 401 {
		t.Fatalf("anonymous identity = %d", w.Code)
	}
	if w := dashboardRequest(second, "POST", "/dashboard/api/logout", "", "", cookie); w.Code != 204 {
		t.Fatalf("logout = %d", w.Code)
	}
	if w := dashboardRequest(first, "GET", "/dashboard/api/identity", "", "", cookie); w.Code != 401 {
		t.Fatalf("revoked identity = %d", w.Code)
	}
}

func TestTokenDisplayIdentity(t *testing.T) {
	a := newSessionAPI(t)
	s, err := NewServer(Config{APIURL: a.server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a.mu.Lock()
	a.sessions["test-session"] = true
	a.mu.Unlock()
	cookie := &http.Cookie{Name: sessionCookie, Value: "test-session"}
	w := dashboardRequest(s, "GET", "/dashboard/api/identity", "", "", cookie)
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"mode":"token"}` {
		t.Fatalf("token identity = %d %s", w.Code, w.Body.String())
	}
}

func TestOIDCDisplayClaimsStayWithinCookieLimit(t *testing.T) {
	p, a := newOIDCProvider(t), newSessionAPI(t)
	p.modify = func(claims map[string]any) {
		for _, key := range []string{"name", "preferred_username", "email", "sub"} {
			claims[key] = strings.Repeat("<😀", 300)
		}
	}
	s := oidcTestFrontend(t, oidcTestConfig(p, a))
	cookie := oidcLoginSession(t, s, p)
	if len(cookie.String()) >= 4096 {
		t.Fatalf("session cookie = %d bytes, want below browser limit", len(cookie.String()))
	}
	w := dashboardRequest(s, "GET", "/dashboard/api/identity", "", "", cookie)
	if w.Code != 200 {
		t.Fatalf("bounded identity = %d %s", w.Code, w.Body.String())
	}
}
