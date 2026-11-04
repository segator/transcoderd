package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
)

// Only public, immutable build assets belong here: no authenticated HTML or APIs.
var dashboardPublicAssets = []string{
	"app.js", "login.js", "media.js", "pwa.js", "styles.css", "media.css", "icon.svg",
	"manifest.webmanifest", "offline.html", "icon-192.png", "icon-512.png",
	"icon-maskable-512.png", "apple-touch-icon.png",
}

func dashboardServiceWorker() ([]byte, error) {
	template, err := dashboardAssets.ReadFile("dashboard/sw.js")
	if err != nil {
		return nil, err
	}
	digest := sha256.New()
	_, _ = digest.Write(template)
	paths := make([]string, 0, len(dashboardPublicAssets))
	for _, name := range dashboardPublicAssets {
		asset, err := dashboardAssets.ReadFile("dashboard/" + name)
		if err != nil {
			return nil, err
		}
		_, _ = digest.Write([]byte(name + "\x00"))
		_, _ = digest.Write(asset)
		paths = append(paths, "/dashboard/"+name)
	}
	raw, err := json.Marshal(paths)
	if err != nil {
		return nil, err
	}
	body := bytes.ReplaceAll(template, []byte("__DASHBOARD_CACHE_VERSION__"), []byte(fmt.Sprintf("%x", digest.Sum(nil))))
	return bytes.ReplaceAll(body, []byte("__DASHBOARD_PUBLIC_PATHS__"), raw), nil
}

func (s *Server) registerDashboardPWA(r *mux.Router) {
	r.HandleFunc("/sw.js", func(w http.ResponseWriter, r *http.Request) {
		body, err := dashboardServiceWorker()
		if err != nil {
			http.Error(w, "PWA unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Service-Worker-Allowed", "/dashboard/")
		_, _ = w.Write(body)
	}).Methods(http.MethodGet, http.MethodHead)
}

func dashboardPublicFileServer(assets http.FileSystem, name string) http.Handler {
	handler := http.FileServer(assets)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if name == "manifest.webmanifest" {
			w.Header().Set("Content-Type", "application/manifest+json")
		}
		handler.ServeHTTP(w, r)
	})
}
