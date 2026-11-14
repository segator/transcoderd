package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"transcoder/model"
	"transcoder/server/scheduler"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

func (s *Server) registerDashboardAPI(r *mux.Router) {
	d := r.PathPrefix("/dashboard/").Subrouter()
	d.Use(dashboardHeaders)
	d.HandleFunc("/api/login", s.dashboardLogin).Methods(http.MethodPost)
	d.HandleFunc("/api/logout", s.protectDashboard(s.dashboardLogout)).Methods(http.MethodPost)
	d.HandleFunc("/api/session", s.protectDashboard(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).Methods(http.MethodGet)
	d.HandleFunc("/api/jobs", s.protectDashboard(s.dashboardJobs)).Methods(http.MethodGet)
	d.HandleFunc("/api/stats", s.protectDashboard(s.dashboardStats)).Methods(http.MethodGet)
	s.registerMedia(d)
	d.HandleFunc("/api/workers/{worker}/control", s.protectDashboard(s.dashboardWorkerControl)).Methods(http.MethodPost)
	d.HandleFunc("/api/workers/{worker}/settings", s.protectDashboard(s.dashboardWorkerSettings)).Methods(http.MethodPut)
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

func (s *Server) dashboardStats(w http.ResponseWriter, r *http.Request) {
	hours := 168
	q := r.URL.Query()
	var ranges []model.DashboardRange
	if q.Has("start") || q.Has("end") {
		start, errStart := time.Parse(time.RFC3339, q.Get("start"))
		end, errEnd := time.Parse(time.RFC3339, q.Get("end"))
		period, err := model.ResolveDashboardRange(0, time.Now(), model.DashboardRange{Start: start, End: end})
		if q.Has("hours") || errStart != nil || errEnd != nil || err != nil {
			http.Error(w, "Use start and end timestamps, start before end, at most 3660 days, without hours", 400)
			return
		}
		hours = 0
		ranges = append(ranges, period)
	}
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
	data, err := s.scheduler.GetDashboard(ctx, hours, ranges...)
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

func (s *Server) dashboardJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := model.DashboardJobsQuery{State: q.Get("state"), Search: strings.TrimSpace(q.Get("search")), Limit: 25}
	switch query.State {
	case "", "all", "active", "queued", "completed", "failed", "canceled":
	default:
		http.Error(w, "Invalid job state", 400)
		return
	}
	var err error
	if q.Has("offset") {
		query.Offset, err = strconv.Atoi(q.Get("offset"))
		if err != nil || query.Offset < 0 {
			http.Error(w, "Invalid job offset", 400)
			return
		}
	}
	if q.Has("limit") {
		query.Limit, err = strconv.Atoi(q.Get("limit"))
		if err != nil || query.Limit < 1 || query.Limit > 100 {
			http.Error(w, "Job limit must be 1–100", 400)
			return
		}
	}
	if len(query.Search) > 256 {
		http.Error(w, "Search is too long", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	page, err := s.scheduler.GetDashboardJobs(ctx, query)
	if err != nil {
		log.WithError(err).Error("Could not read dashboard jobs")
		http.Error(w, "Could not load jobs", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(page); err != nil {
		log.WithError(err).Debug("Could not write dashboard jobs")
	}
}

func (s *Server) dashboardWorkerControl(w http.ResponseWriter, r *http.Request) {
	if !s.dashboardMutationAllowed(w, r) {
		return
	}
	name := strings.TrimSpace(mux.Vars(r)["worker"])
	if name == "" || len(name) > 100 || strings.ContainsAny(name, "\r\n\x00") {
		http.Error(w, "Invalid worker name", 400)
		return
	}
	var input struct {
		Action string `json:"action"`
		JobID  string `json:"jobId"`
	}
	if err := decodeDashboardJSON(w, r, &input); err != nil {
		http.Error(w, "Invalid control request", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	c, err := s.scheduler.ControlWorker(ctx, name, input.Action, input.JobID)
	if err != nil {
		code := 500
		if errors.Is(err, scheduler.ErrInvalidWorkerSettings) {
			code = 400
		}
		if errors.Is(err, scheduler.ErrWorkerControlUnavailable) {
			code = 409
		}
		if code == 500 {
			log.WithError(err).Error("Could not queue worker control")
			http.Error(w, "Could not queue worker control", code)
		} else {
			http.Error(w, err.Error(), code)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(c); err != nil {
		log.WithError(err).Debug("Could not write worker control")
	}
}
