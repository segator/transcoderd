package web

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	frontend "transcoder/frontend/web"

	"github.com/go-jose/go-jose/v4"
)

func TestOIDCFrontendUsesRevocableAPISessions(t *testing.T) {
	const apiCredential = "private-dashboard-api-credential-for-oidc-tests"
	api, _ := mediaTestServer(t)
	api.config.DashboardToken = apiCredential
	backend := httptest.NewServer(api.srv.Handler)
	defer backend.Close()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: key}, new(jose.SignerOptions).WithHeader("kid", "integration-key"))
	if err != nil {
		t.Fatal(err)
	}
	var nonce, challenge atomic.Value
	nonce.Store("")
	challenge.Store("")
	var issuer *httptest.Server
	issuer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer.URL, "authorization_endpoint": issuer.URL + "/authorize", "token_endpoint": issuer.URL + "/token", "jwks_uri": issuer.URL + "/keys", "id_token_signing_alg_values_supported": []string{"ES256"}})
		case "/keys":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "integration-key", Algorithm: "ES256", Use: "sig"}}})
		case "/token":
			_ = r.ParseForm()
			id, secret, _ := r.BasicAuth()
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if id != "client" || secret != "oidc-client-secret" || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge.Load().(string) {
				http.Error(w, "invalid grant", http.StatusBadRequest)
				return
			}
			claims, _ := json.Marshal(map[string]any{"iss": issuer.URL, "sub": "verified-operator", "aud": "client", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(), "nonce": nonce.Load().(string)})
			signed, err := signer.Sign(claims)
			if err != nil {
				http.Error(w, "signing failed", http.StatusInternalServerError)
				return
			}
			raw, err := signed.CompactSerialize()
			if err != nil {
				http.Error(w, "signing failed", http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id_token": raw, "access_token": "unused-provider-token", "token_type": "Bearer", "expires_in": 3600})
		default:
			http.NotFound(w, r)
		}
	}))
	defer issuer.Close()
	config := frontend.Config{APIURL: backend.URL, PublicURL: "https://ui.example.test", APIToken: apiCredential, Auth: frontend.AuthConfig{Mode: "oidc", SessionKey: base64.StdEncoding.EncodeToString(make([]byte, 32)), OIDC: frontend.OIDCConfig{IssuerURL: issuer.URL, ClientID: "client", ClientSecret: "oidc-client-secret"}}}
	first, err := frontend.NewServer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := frontend.NewServer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	call := func(s *frontend.Server, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("X-Dashboard-Request", "1")
		r.Header.Set("Origin", config.PublicURL)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	start := call(first, "GET", "/dashboard/auth/login", "", nil)
	if start.Code != 303 || len(start.Result().Cookies()) != 1 {
		t.Fatalf("OIDC start = %d", start.Code)
	}
	authorize, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	q := authorize.Query()
	nonce.Store(q.Get("nonce"))
	challenge.Store(q.Get("code_challenge"))
	callback := "/dashboard/auth/callback?code=integration-code&state=" + url.QueryEscape(q.Get("state"))
	result := call(second, "GET", callback, "", start.Result().Cookies()[0])
	if result.Code != 303 {
		t.Fatalf("OIDC callback to real API = %d %s", result.Code, result.Body.String())
	}
	var cookie *http.Cookie
	for _, c := range result.Result().Cookies() {
		if c.Name == "transcoderd_frontend" {
			cookie = c
		}
		if c.Name == dashboardCookie || strings.Contains(c.Value, apiCredential) {
			t.Fatal("backend cookie or API credential exposed to browser")
		}
	}
	if cookie == nil {
		t.Fatal("missing frontend session")
	}
	for _, path := range []string{"/dashboard/", "/dashboard/api/stats", "/dashboard/api/media/roots"} {
		w := call(first, "GET", path, "", cookie)
		if w.Code != 200 || strings.Contains(w.Body.String(), apiCredential) {
			t.Errorf("OIDC real API %s = %d", path, w.Code)
		}
	}
	w := call(second, "PUT", "/dashboard/api/workers/cpu-mini/settings", `{"cpuJobs":2,"gpuJobs":1}`, cookie)
	if w.Code != 204 {
		t.Fatalf("OIDC settings write = %d %s", w.Code, w.Body.String())
	}
	if w := call(first, "POST", "/dashboard/api/logout", "", cookie); w.Code != 204 {
		t.Fatalf("OIDC real API logout = %d", w.Code)
	}
	if w := call(second, "GET", "/dashboard/api/stats", "", cookie); w.Code != 401 {
		t.Fatalf("revoked session on second replica = %d", w.Code)
	}
	if w := dashboardRequest(api, "GET", "/api/v1/worker/settings", "", apiCredential, nil); w.Code != 401 {
		t.Fatal("dashboard service credential authorized worker API")
	}
}
