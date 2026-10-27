package dashboard

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"transcoder/version"
	"transcoder/worker/control"
)

//go:embed wwwroot/*
var assets embed.FS

type admissionController interface {
	AdmissionStatus() control.AdmissionStatus
	PersistMaxActiveJobs(int) error
}

type Server struct {
	listener net.Listener
	http     *http.Server
}

type statusResponse struct {
	Configured bool                    `json:"configured"`
	Running    bool                    `json:"running"`
	Version    string                  `json:"version"`
	Worker     control.AdmissionStatus `json:"worker"`
}

// Start opens the optional browser dashboard. An address without a token is rejected.
func Start(address, token string, worker admissionController) (*Server, error) {
	if strings.TrimSpace(address) == "" {
		return nil, nil
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("worker.dashboard.token is required when worker.dashboard.listenAddress is configured")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen for worker dashboard: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", authorize(token, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, statusResponse{Configured: true, Running: true, Version: version.Version, Worker: worker.AdmissionStatus()})
	}))
	mux.HandleFunc("POST /api/worker/max-jobs", authorize(token, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			MaxJobs int `json:"maxJobs"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MaxJobs < 0 || req.MaxJobs > 1024 {
			http.Error(w, `{"error":"maxJobs must be an integer between 0 and 1024"}`, http.StatusBadRequest)
			return
		}
		if err := worker.PersistMaxActiveJobs(req.MaxJobs); err != nil {
			http.Error(w, `{"error":"worker could not apply the admission limit"}`, http.StatusConflict)
			return
		}
		writeJSON(w, statusResponse{Configured: true, Running: true, Version: version.Version, Worker: worker.AdmissionStatus()})
	}))
	webroot, err := fs.Sub(assets, "wwwroot")
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("open embedded dashboard assets: %w", err)
	}
	static := http.FileServer(http.FS(webroot))
	mux.Handle("/", static)
	server := &http.Server{Handler: securityHeaders(mux), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	return &Server{listener: listener, http: server}, nil
}

func (s *Server) Serve() error {
	if s == nil {
		return nil
	}
	if err := s.http.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Shutdown() error {
	if s == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.http.Shutdown(ctx)
}

func authorize(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		providedHash, expectedHash := sha256.Sum256([]byte(provided)), sha256.Sum256([]byte(token))
		if subtle.ConstantTimeCompare(providedHash[:], expectedHash[:]) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; style-src-attr 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; form-action 'none'")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}
