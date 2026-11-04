package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"transcoder/server/scheduler"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

//go:embed dashboard/*
var dashboardAssets embed.FS

func (s *Server) registerDashboard(r *mux.Router) {
	s.dashboard = &dashboardAuth{sessions: make(map[[32]byte]time.Time)}
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard/", http.StatusSeeOther)
	}).Methods(http.MethodGet)
	r.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard/", http.StatusSeeOther)
	}).Methods(http.MethodGet)
	d := r.PathPrefix("/dashboard/").Subrouter()
	d.Use(dashboardHeaders)
	d.HandleFunc("/login", s.serveDashboardLogin).Methods(http.MethodGet)
	d.HandleFunc("/api/login", s.dashboardLogin).Methods(http.MethodPost)
	d.HandleFunc("/api/logout", s.protectDashboard(s.dashboardLogout, false)).Methods(http.MethodPost)
	d.HandleFunc("/api/stats", s.protectDashboard(s.dashboardStats, false)).Methods(http.MethodGet)
	s.registerMedia(d)
	s.registerDashboardPWA(d)
	d.HandleFunc("/api/workers/{worker}/settings", s.protectDashboard(s.dashboardWorkerSettings, false)).Methods(http.MethodPut)
	d.HandleFunc("/", s.protectDashboard(s.serveDashboard, true)).Methods(http.MethodGet)
	assets, _ := fs.Sub(dashboardAssets, "dashboard")
	for _, name := range dashboardPublicAssets {
		d.Handle("/"+name, http.StripPrefix("/dashboard/", dashboardPublicFileServer(http.FS(assets), name))).Methods(http.MethodGet)
	}
}

func dashboardHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; media-src 'self' blob:; connect-src 'self'; font-src 'self'; manifest-src 'self'; worker-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) serveDashboardLogin(w http.ResponseWriter, r *http.Request) {
	if s.dashboardAuthenticated(r) {
		http.Redirect(w, r, "/dashboard/", http.StatusSeeOther)
		return
	}
	serveDashboardHTML(w, "login.html")
}

func (s *Server) serveDashboard(w http.ResponseWriter, r *http.Request) {
	serveDashboardHTML(w, "index.html")
}

func serveDashboardHTML(w http.ResponseWriter, name string) {
	b, err := dashboardAssets.ReadFile("dashboard/" + name)
	if err != nil {
		http.Error(w, "Dashboard asset unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}

func (s *Server) dashboardStats(w http.ResponseWriter, r *http.Request) {
	hours := 168
	if raw := r.URL.Query().Get("hours"); raw != "" {
		var err error
		hours, err = strconv.Atoi(raw)
		if err != nil || (hours != 24 && hours != 168 && hours != 720) {
			http.Error(w, "hours must be 24, 168, or 720", http.StatusBadRequest)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	data, err := s.scheduler.GetDashboard(ctx, hours)
	if err != nil {
		log.WithError(err).Error("Could not read dashboard statistics")
		http.Error(w, "Could not load statistics", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.WithError(err).Error("Could not write dashboard statistics")
	}
}

func (s *Server) dashboardWorkerSettings(w http.ResponseWriter, r *http.Request) {
	if !s.dashboardMutationAllowed(w, r) {
		return
	}
	name := strings.TrimSpace(mux.Vars(r)["worker"])
	if name == "" || len(name) > 100 || strings.ContainsAny(name, "\r\n\x00") {
		http.Error(w, "Invalid worker name", http.StatusBadRequest)
		return
	}
	var input workerSettingsRequest
	if err := decodeDashboardJSON(w, r, &input); err != nil {
		http.Error(w, "Invalid worker settings", http.StatusBadRequest)
		return
	}
	if err := input.ValidateWrite(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.scheduler.SetWorkerSettings(ctx, name, input); err != nil {
		if errors.Is(err, scheduler.ErrInvalidWorkerSettings) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.WithError(err).WithField("worker", name).Error("Could not update worker capacity")
		http.Error(w, "Could not save worker settings", http.StatusInternalServerError)
		return
	}
	fields := log.Fields{"worker": name}
	if input.Independent() {
		fields["cpu_jobs"], fields["gpu_jobs"] = *input.CPUJobs, *input.GPUJobs
	} else {
		fields["max_active_jobs"] = *input.MaxActiveJobs
	}
	log.WithFields(fields).Info("Dashboard saved worker capacity")
	w.WriteHeader(http.StatusNoContent)
}
