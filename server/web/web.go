package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"transcoder/model"
	"transcoder/server/scheduler"
	"transcoder/update"
)

type Server struct {
	config        *Config
	scheduler     scheduler.Scheduler
	srv           http.Server
	ctx           context.Context
	activeTracker *ActiveTracker
	updater       *update.Updater
}

func (s *Server) requestJob(writer http.ResponseWriter, request *http.Request) {
	workerName := request.Header.Get("workerName")
	if workerName == "" {
		webError(writer, fmt.Errorf("workerName is mandatory in the headers"), 403)
		return
	}
	job, err := s.scheduler.RequestJob(s.ctx, workerName)
	if errors.Is(err, scheduler.NoJobsAvailable) {
		webError(writer, err, 204)
		return
	}
	if webError(writer, err, 500) {
		return
	}
	b, err := json.MarshalIndent(job, "", "\t")
	if webError(writer, err, 500) {
		return
	}
	writer.WriteHeader(200)
	_, err = writer.Write(b)
	if err != nil {
		log.Errorf("Error writing response %v", err)
	}
}

func (s *Server) handleWorkerEvent(writer http.ResponseWriter, request *http.Request) {
	taskEvent := &model.TaskEvent{}
	err := json.NewDecoder(request.Body).Decode(taskEvent)
	if webError(writer, err, 500) {
		return
	}

	err = s.scheduler.HandleWorkerEvent(s.ctx, taskEvent)
	if webError(writer, err, 500) {
		return
	}

	writer.WriteHeader(200)
}

func (s *Server) addJobs(writer http.ResponseWriter, request *http.Request) {
	defer request.Body.Close()
	jobRequest := &model.JobRequest{}
	err := json.NewDecoder(request.Body).Decode(jobRequest)
	if err != nil {
		webError(writer, err, 500)
		return
	}
	if jobRequest.SourcePath == "" {
		webError(writer, fmt.Errorf("sourcePath is mandatory"), 400)
		return
	}

	scheduleJobResults, err := s.scheduler.ScheduleJobRequests(s.ctx, jobRequest)
	if webError(writer, err, 500) {
		return
	}

	if webError(writer, err, 500) {
		return
	}
	b, err := json.MarshalIndent(scheduleJobResults, "", "\t")
	if err != nil {
		if webError(writer, err, 500) {
			return
		}
	}
	writer.WriteHeader(200)
	_, err = writer.Write(b)
	if err != nil {
		log.Errorf("Error writing response %v", err)
	}
}

func (s *Server) cancelJobs(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	jobId := vars["jobid"]
	if jobId == "" {
		webError(writer, fmt.Errorf("jobId is mandatory"), 400)
		return
	}
	err := s.scheduler.CancelJob(s.ctx, jobId)
	if webError(writer, err, 500) {
		return
	}
}

func (s *Server) upload(writer http.ResponseWriter, request *http.Request) {
	workerName := request.Header.Get("workerName")
	if workerName == "" {
		webError(writer, fmt.Errorf("workerName is mandatory in the headers"), 403)
		return
	}
	values := request.URL.Query()
	uuid := values.Get("uuid")
	if uuid == "" {
		webError(writer, fmt.Errorf("UUID get parameter not found"), 404)
	}
	uploadStream, err := s.scheduler.GetUploadJobWriter(request.Context(), uuid, workerName)
	switch {
	case errors.Is(err, scheduler.ErrorStreamNotAllowed):
		webError(writer, err, 403)
		return
	case errors.Is(err, scheduler.ErrorJobNotFound):
		webError(writer, err, 404)
		return
	case webError(writer, err, 500):
		return
	}
	defer func(uploadStream *scheduler.UploadJobStream, pushChecksum bool) {
		_ = uploadStream.Close(pushChecksum)
	}(uploadStream, false)

	size, err := strconv.ParseUint(request.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		webError(writer, fmt.Errorf("invalid size %v", err), 400)
		return
	}
	checksum := request.Header.Get("checksum")
	if checksum == "" {
		webError(writer, fmt.Errorf("checksum is mandatory in the headers"), 403)
		return
	}

	b := make([]byte, 131072)
	reader := request.Body
	var readed uint64
loop:
	for {
		select {
		case <-request.Context().Done():
			return
		default:
			readedBytes, readErr := reader.Read(b)
			readed += uint64(readedBytes)
			_, writeErr := uploadStream.Write(b[0:readedBytes])
			if writeErr != nil {
				break loop
			}
			if errors.Is(readErr, io.EOF) {
				break loop
			}
			if readErr != nil {
				log.Errorf("Error reading from download stream: %v", readErr)
				return
			}
		}
	}
	if size != readed {
		defer func(uploadStream *scheduler.UploadJobStream) {
			_ = uploadStream.Clean()
		}(uploadStream)
		webError(writer, fmt.Errorf("invalid size, expected %d, received %d", size, readed), 400)
		return
	}
	checksumUpload := uploadStream.GetHash()
	if checksumUpload != checksum {
		defer func(uploadStream *scheduler.UploadJobStream) {
			_ = uploadStream.Clean()
		}(uploadStream)
		webError(writer, fmt.Errorf("invalid checksum, received %s, calculated %s", checksum, checksumUpload), 400)
		return
	}

	writer.WriteHeader(201)
}

func (s *Server) download(writer http.ResponseWriter, request *http.Request) {
	workerName := request.Header.Get("workerName")
	if workerName == "" {
		webError(writer, fmt.Errorf("workerName is mandatory in the headers"), 403)
		return
	}
	values := request.URL.Query()
	uuid := values.Get("uuid")
	if uuid == "" {
		webError(writer, fmt.Errorf("UUID get parameter not found"), 404)
	}
	downloadStream, err := s.scheduler.GetDownloadJobWriter(request.Context(), uuid, workerName)
	switch {
	case errors.Is(err, scheduler.ErrorStreamNotAllowed):
		webError(writer, err, 403)
		return
	case errors.Is(err, scheduler.ErrorJobNotFound):
		webError(writer, err, 404)
		return
	case webError(writer, err, 500):
		return
	}
	defer downloadStream.Close(true)

	writer.Header().Set("Content-Length", strconv.FormatInt(downloadStream.Size(), 10))
	writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", url.QueryEscape(downloadStream.Name())))
	writer.WriteHeader(200)
	b := make([]byte, 131072)
loop:
	for {
		select {
		case <-request.Context().Done():
			return
		default:
			readedBytes, readErr := downloadStream.Read(b)
			_, writeErr := writer.Write(b[0:readedBytes])
			if writeErr != nil {
				break loop
			}
			if errors.Is(readErr, io.EOF) {
				break loop
			}
			if readErr != nil {
				log.Errorf("Error reading from download stream: %v", readErr)
				return
			}

		}
	}
}

func (s *Server) checksum(writer http.ResponseWriter, request *http.Request) {
	values := request.URL.Query()
	uuid := values.Get("uuid")
	if uuid == "" {
		webError(writer, fmt.Errorf("UUID get parameter not found"), 404)
		return
	}
	checksum, err := s.scheduler.GetChecksum(request.Context(), uuid)
	if webError(writer, err, 404) {
		return
	}
	writer.Header().Set("Content-Length", strconv.Itoa(len(checksum)))
	writer.Header().Set("Content-Type", "text/plain")
	writer.WriteHeader(200)
	_, err = writer.Write([]byte(checksum))
	if err != nil {
		log.Errorf("Error writing response %v", err)
	}
}

type Config struct {
	Port   int    `mapstructure:"port" envconfig:"WEB_PORT"`
	Token  string `mapstructure:"token" envconfig:"WEB_TOKEN"`
	Domain string `mapstructure:"domain" envconfig:"WEB_DOMAIN"`
}

type ActiveTracker struct {
	activeRequests int64
}

func (a *ActiveTracker) ActiveRequests() bool {
	return atomic.LoadInt64(&a.activeRequests) > 0
}

func NewWebServer(config *Config, scheduler scheduler.Scheduler, updater *update.Updater) *Server {
	rtr := mux.NewRouter()
	at := &ActiveTracker{}
	rtr.Use(at.ActiveRequestsMiddleware())
	rtr.Use(LoggingMiddleware())

	webServer := &Server{
		config:        config,
		activeTracker: at,
		updater:       updater,
		scheduler:     scheduler,
		srv: http.Server{
			Addr:    ":" + strconv.Itoa(config.Port),
			Handler: rtr,
		},
	}
	rtr.Handle("/api/v1/job/", webServer.AuthFunc(webServer.addJobs)).Methods("POST")
	rtr.Handle("/api/v1/job/{jobid}", webServer.AuthFunc(webServer.cancelJobs)).Methods("DELETE")
	rtr.Handle("/api/v1/job/request", webServer.AuthFunc(webServer.requestJob)).Methods("GET")
	rtr.Handle("/api/v1/event", webServer.AuthFunc(webServer.handleWorkerEvent)).Methods("POST")
	rtr.HandleFunc("/api/v1/download", webServer.download).Methods("GET")
	rtr.HandleFunc("/api/v1/checksum", webServer.checksum).Methods("GET")
	rtr.HandleFunc("/api/v1/upload", webServer.upload).Methods("POST", "PUT")
	return webServer
}

func (a *ActiveTracker) ActiveRequestsMiddleware() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			atomic.AddInt64(&a.activeRequests, 1)
			defer atomic.AddInt64(&a.activeRequests, -1)
			next.ServeHTTP(w, req)
		})
	}

}

func LoggingMiddleware() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, req) // Call the next handler
			duration := time.Since(start)

			log.WithFields(log.Fields{
				"method":      req.Method,
				"url":         req.URL.String(),
				"remote_addr": req.RemoteAddr,
				"user_agent":  req.UserAgent(),
				"duration_ms": duration.Milliseconds(),
			}).Debug("HTTP request")
		})
	}
}

func (s *Server) Run(wg *sync.WaitGroup, ctx context.Context) {
	s.ctx = ctx
	log.Info("Starting WebServer...")
	s.start()
	log.Info("Started WebServer...")
	wg.Add(1)
	go func() {
		<-ctx.Done()
		log.Info("Stopping WebServer...")
		s.stop(ctx)
		wg.Done()
	}()
}

func (s *Server) start() {
	// Web server
	go func() {
		err := s.srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Panic(err)
		}
	}()

	// updater
	go func() {
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-time.After(time.Minute * 5):
				log.Debug("Checking for updates")
				if !s.activeTracker.ActiveRequests() {
					release, updateRequired, err := s.updater.CheckForUpdate()
					if err != nil {
						log.Error(err)
						continue
					}
					if updateRequired {
						log.Warnf("New version available %s,exiting ...", release.TagName)
						os.Exit(update.ExitCode)
					}
				}

			}
		}
	}()
}

func (s *Server) stop(ctx context.Context) {
	if err := s.srv.Shutdown(ctx); err != nil {
		log.Panic(err)
	}
}

func (s *Server) AuthFunc(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeUnauthorized(w)
			return
		}
		const bearerPrefix = "Bearer "
		if !strings.HasPrefix(authHeader, bearerPrefix) {
			writeUnauthorized(w)
			return
		}

		t := strings.TrimPrefix(authHeader, bearerPrefix)

		if t != s.config.Token {
			writeUnauthorized(w)
			return
		}
		handler(w, r)
	}
}
func writeUnauthorized(w http.ResponseWriter) {
	w.WriteHeader(401)
	_, err := w.Write([]byte("Unauthorised.\n"))
	if err != nil {
		log.Errorf("Error writing response %v", err)
	}
}

func webError(writer http.ResponseWriter, err error, code int) bool {
	if err != nil {
		http.Error(writer, err.Error(), code)
		return true
	}
	return false
}
