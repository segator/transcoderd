package web

import (
	"encoding/json"
	"image/png"
	"strings"
	"testing"
)

func TestDashboardPWAManifestAndPublicAssets(t *testing.T) {
	s := newWorkerSettingsTestServer(t)
	w := dashboardRequest(s, "GET", "/dashboard/manifest.webmanifest", "", "", nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/manifest+json" {
		t.Fatalf("manifest = %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	var manifest struct {
		ID, Name, StartURL, Scope, Display string
		Icons                              []struct{ Src, Sizes, Type, Purpose string }
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	var startURL string
	if err := json.Unmarshal(raw["start_url"], &startURL); err != nil {
		t.Fatal(err)
	}
	if manifest.ID != "/dashboard/" || startURL != "/dashboard/" || manifest.Scope != "/dashboard/" || manifest.Display != "standalone" || manifest.Name == "" {
		t.Fatalf("manifest launch = %+v", manifest)
	}
	if len(manifest.Icons) != 3 {
		t.Fatalf("manifest icons = %d, want 3", len(manifest.Icons))
	}
	for _, icon := range manifest.Icons {
		response := dashboardRequest(s, "GET", icon.Src, "", "", nil)
		image, err := png.DecodeConfig(response.Body)
		if response.Code != 200 || err != nil {
			t.Fatalf("icon %s = %d, %v", icon.Src, response.Code, err)
		}
		want := 192
		if icon.Sizes == "512x512" {
			want = 512
		}
		if image.Width != want || image.Height != want {
			t.Errorf("icon %s = %dx%d, want %dx%d", icon.Src, image.Width, image.Height, want, want)
		}
	}
	apple := dashboardRequest(s, "GET", "/dashboard/apple-touch-icon.png", "", "", nil)
	image, err := png.DecodeConfig(apple.Body)
	if err != nil || image.Width != 180 || image.Height != 180 {
		t.Fatalf("Apple icon = %+v %v", image, err)
	}
	for _, name := range dashboardPublicAssets {
		response := dashboardRequest(s, "GET", "/dashboard/"+name, "", "", nil)
		if response.Code != 200 {
			t.Errorf("public asset %s = %d", name, response.Code)
		}
		if strings.Contains(response.Body.String(), "encoding123") {
			t.Errorf("asset %s exposes dashboard token", name)
		}
	}
}

func TestDashboardPWAWorkerVersionScopeAndAuth(t *testing.T) {
	s := newWorkerSettingsTestServer(t)
	w := dashboardRequest(s, "GET", "/dashboard/sw.js", "", "", nil)
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "javascript") || w.Header().Get("Service-Worker-Allowed") != "/dashboard/" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("service worker = %d %+v", w.Code, w.Header())
	}
	if strings.Contains(body, "__DASHBOARD_") {
		t.Fatal("unexpanded service worker template")
	}
	if again := dashboardRequest(s, "GET", "/dashboard/sw.js", "", "", nil).Body.String(); again != body {
		t.Error("worker version is not deterministic for this build")
	}
	for _, name := range dashboardPublicAssets {
		if !strings.Contains(body, `"/dashboard/`+name+`"`) {
			t.Errorf("worker omitted public asset %s", name)
		}
	}
	cookie := loginDashboard(t, s)
	page := dashboardRequest(s, "GET", "/dashboard/", "", "", cookie)
	for _, text := range []string{"manifest.webmanifest", "apple-touch-icon.png", "pwa.js"} {
		if !strings.Contains(page.Body.String(), text) {
			t.Errorf("dashboard omitted %s", text)
		}
	}
	policy := page.Header().Get("Content-Security-Policy")
	if !strings.Contains(policy, "worker-src 'self'") || !strings.Contains(policy, "manifest-src 'self'") {
		t.Errorf("PWA CSP = %s", policy)
	}
	if got := dashboardRequest(s, "POST", "/dashboard/api/logout", "", "", cookie).Code; got != 204 {
		t.Fatalf("logout = %d", got)
	}
	if got := dashboardRequest(s, "GET", "/dashboard/", "", "", cookie).Code; got != 303 {
		t.Fatalf("installed launch after logout = %d", got)
	}
	if got := dashboardRequest(s, "GET", "/dashboard/api/stats", "", "", cookie).Code; got != 401 {
		t.Fatalf("stats after logout = %d", got)
	}
	offline := dashboardRequest(s, "GET", "/dashboard/offline.html", "", "", nil)
	if offline.Code != 200 || !strings.Contains(offline.Body.String(), "Reconnect") {
		t.Fatalf("offline page = %d", offline.Code)
	}
}
