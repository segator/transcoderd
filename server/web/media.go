package web

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"

	"transcoder/model"
	"transcoder/server/library"
	"transcoder/server/repository"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

func (s *Server) registerMedia(r *mux.Router) {
	if backend, ok := s.scheduler.(library.Backend); ok {
		s.library = library.New(backend, s.config.MediaRoots, s.config.PreviewMaxStreams)
	}
	r.HandleFunc("/api/media/roots", s.protectDashboard(s.mediaRoots, false)).Methods("GET")
	r.HandleFunc("/api/media/browse", s.protectDashboard(s.mediaBrowse, false)).Methods("GET")
	r.HandleFunc("/api/media/info/{id}", s.protectDashboard(s.mediaInfo, false)).Methods("GET")
	r.HandleFunc("/api/media/file/{id}", s.protectDashboard(s.mediaOriginal, false)).Methods("GET", "HEAD")
	r.HandleFunc("/api/media/preview/{id}", s.protectDashboard(s.mediaPreview, false)).Methods("GET")
	r.HandleFunc("/api/media/jobs/{id}", s.protectDashboard(s.mediaJobAssets, false)).Methods("GET")
	r.HandleFunc("/api/media/scans", s.protectDashboard(s.mediaStartScan, false)).Methods("POST")
	r.HandleFunc("/api/media/batches", s.protectDashboard(s.mediaStartBatch, false)).Methods("POST")
	r.HandleFunc("/api/media/operations/{id}", s.protectDashboard(s.mediaOperation, false)).Methods("GET")
	r.HandleFunc("/api/media/operations/{id}", s.protectDashboard(s.mediaCancel, false)).Methods("DELETE")
}

func mediaJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.WithError(err).Debug("Could not write media response")
	}
}

func mediaError(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	message := "No se pudo completar la operación de biblioteca"
	switch {
	case errors.Is(err, library.ErrInvalidPath):
		code, message = 400, "Ruta de vídeo no válida"
	case errors.Is(err, library.ErrUnavailable):
		code, message = 503, "Biblioteca no disponible"
	case errors.Is(err, library.ErrBusy):
		code, message = 429, "Hay otras operaciones o reproducciones en curso. Inténtalo en unos segundos."
	case errors.Is(err, library.ErrOperationNotFound):
		code, message = 404, "La operación ya no está disponible; vuelve a preparar la selección"
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, repository.ErrElementNotFound):
		code, message = 404, "El archivo ya no está disponible"
	case errors.Is(err, fs.ErrPermission):
		code, message = 403, "No hay acceso a este vídeo"
	case errors.Is(err, library.ErrChanged):
		code, message = 409, "El archivo cambió desde la selección"
	}
	if code == 500 {
		log.WithError(err).Warn("Media operation failed")
	}
	http.Error(w, message, code)
}

func (s *Server) mediaReady(w http.ResponseWriter) bool {
	if s.library == nil {
		mediaError(w, library.ErrUnavailable)
		return false
	}
	return true
}

func (s *Server) mediaRoots(w http.ResponseWriter, r *http.Request) {
	if !s.mediaReady(w) {
		return
	}
	mediaJSON(w, map[string]any{"roots": s.library.Roots(), "presets": library.Presets()})
}

func (s *Server) mediaBrowse(w http.ResponseWriter, r *http.Request) {
	if !s.mediaReady(w) {
		return
	}
	q := r.URL.Query()
	root := q.Get("root")
	if root == "" {
		root = "source"
	}
	offset := 0
	if value := q.Get("offset"); value != "" {
		var err error
		offset, err = strconv.Atoi(value)
		if err != nil {
			mediaError(w, library.ErrInvalidPath)
			return
		}
	}
	data, err := s.library.Browse(r.Context(), root, q.Get("path"), offset, 100)
	if err != nil {
		mediaError(w, err)
		return
	}
	mediaJSON(w, data)
}

func (s *Server) mediaInfo(w http.ResponseWriter, r *http.Request) {
	if !s.mediaReady(w) {
		return
	}
	file, err := s.library.Info(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		mediaError(w, err)
		return
	}
	mediaJSON(w, file)
}

func (s *Server) mediaOriginal(w http.ResponseWriter, r *http.Request) {
	if !s.mediaReady(w) {
		return
	}
	file, entry, err := s.library.Open(mux.Vars(r)["id"])
	if err != nil {
		mediaError(w, err)
		return
	}
	defer file.Close()
	contentType := mime.TypeByExtension(filepath.Ext(entry.Path))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": entry.Name}))
	http.ServeContent(w, r, entry.Name, entry.ModifiedAt, file)
}

type mediaStreamWriter struct {
	w       http.ResponseWriter
	written bool
}

func (m *mediaStreamWriter) Write(p []byte) (int, error) {
	m.written = true
	n, err := m.w.Write(p)
	if f, ok := m.w.(http.Flusher); ok {
		f.Flush()
	}
	return n, err
}

func (s *Server) mediaPreview(w http.ResponseWriter, r *http.Request) {
	if !s.mediaReady(w) {
		return
	}
	start := 0.0
	if raw := r.URL.Query().Get("start"); raw != "" {
		var err error
		start, err = strconv.ParseFloat(raw, 64)
		if err != nil {
			mediaError(w, library.ErrInvalidPath)
			return
		}
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Accept-Ranges", "none")
	w.Header().Set("X-Accel-Buffering", "no")
	stream := &mediaStreamWriter{w: w}
	err := s.library.Preview(r.Context(), mux.Vars(r)["id"], start, stream)
	if r.Context().Err() != nil || errors.Is(err, context.Canceled) {
		return
	}
	if err != nil && !stream.written {
		mediaError(w, err)
	} else if err != nil && r.Context().Err() == nil {
		log.WithError(err).Debug("Video preview ended")
	}
}

func (s *Server) mediaJobAssets(w http.ResponseWriter, r *http.Request) {
	if !s.mediaReady(w) {
		return
	}
	assets, err := s.library.JobAssets(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		mediaError(w, err)
		return
	}
	mediaJSON(w, map[string]any{"files": assets})
}

func (s *Server) mediaStartScan(w http.ResponseWriter, r *http.Request) {
	if !s.mediaReady(w) || !s.dashboardMutationAllowed(w, r) {
		return
	}
	var filters model.MediaFilters
	if err := decodeLimitedJSON(w, r, &filters, 16384); err != nil {
		http.Error(w, "Filtros no válidos", 400)
		return
	}
	op, err := s.library.StartScan(filters)
	if err != nil {
		if !errors.Is(err, library.ErrUnavailable) && !errors.Is(err, library.ErrBusy) && !errors.Is(err, fs.ErrNotExist) {
			http.Error(w, "Revisa los filtros y la carpeta elegida", 400)
		} else {
			mediaError(w, err)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	mediaJSON(w, op)
}

func (s *Server) mediaStartBatch(w http.ResponseWriter, r *http.Request) {
	if !s.mediaReady(w) || !s.dashboardMutationAllowed(w, r) {
		return
	}
	var request model.MediaBatchRequest
	if err := decodeLimitedJSON(w, r, &request, 1<<20); err != nil {
		http.Error(w, "Selección no válida", 400)
		return
	}
	op, err := s.library.StartBatch(request)
	if err != nil {
		if errors.Is(err, library.ErrBusy) || errors.Is(err, library.ErrOperationNotFound) {
			mediaError(w, err)
		} else {
			http.Error(w, "Revisa la selección y el número de jobs; se necesita una vista previa completada", 400)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	mediaJSON(w, op)
}

func (s *Server) mediaOperation(w http.ResponseWriter, r *http.Request) {
	if !s.mediaReady(w) {
		return
	}
	op, err := s.library.Operation(mux.Vars(r)["id"])
	if err != nil {
		mediaError(w, err)
		return
	}
	mediaJSON(w, op)
}
func (s *Server) mediaCancel(w http.ResponseWriter, r *http.Request) {
	if !s.mediaReady(w) || !s.dashboardMutationAllowed(w, r) {
		return
	}
	if err := s.library.Cancel(mux.Vars(r)["id"]); err != nil {
		mediaError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
