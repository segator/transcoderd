package web

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

//go:embed dashboard/*
var dashboardAssets embed.FS

const sessionCookie = "transcoderd_dashboard"

var ErrInvalidOrigin = errors.New("expected an HTTP(S) origin without credentials, path, query or fragment")

type Config struct {
	Listen    string `mapstructure:"listen"`
	APIURL    string `mapstructure:"apiURL"`
	PublicURL string `mapstructure:"publicURL"`
}

// Server depends exclusively on the remote HTTP API and its own embedded assets.
type Server struct {
	handler   http.Handler
	api       *url.URL
	public    *url.URL
	transport *http.Transport
	client    *http.Client
	proxy     *httputil.ReverseProxy
}

func parseOrigin(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || strings.Contains(raw, "#") || u.Opaque != "" {
		return nil, ErrInvalidOrigin
	}
	u.Path = ""
	return u, nil
}

func NewServer(config Config) (*Server, error) {
	api, err := parseOrigin(config.APIURL)
	if err != nil {
		return nil, fmt.Errorf("frontend.apiURL: %w", err)
	}
	s := &Server{api: api}
	if config.PublicURL != "" {
		s.public, err = parseOrigin(config.PublicURL)
		if err != nil {
			return nil, fmt.Errorf("frontend.publicURL: %w", err)
		}
	}
	s.transport = &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout: 90 * time.Second, MaxIdleConns: 100,
	}
	s.client = &http.Client{Transport: s.transport, Timeout: 5 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	s.proxy = &httputil.ReverseProxy{
		Transport: s.transport, FlushInterval: -1,
		Rewrite: func(p *httputil.ProxyRequest) {
			p.SetURL(s.api)
			p.Out.Header = make(http.Header)
			for _, name := range []string{"Authorization", "Content-Type", "Accept", "Accept-Encoding", "User-Agent", "Range", "If-Range", "If-Match", "If-None-Match", "If-Modified-Since", "If-Unmodified-Since", "X-Dashboard-Request"} {
				for _, value := range p.In.Header.Values(name) {
					p.Out.Header.Add(name, value)
				}
			}
			copySession(p.Out, p.In)
		},
		ModifyResponse: func(response *http.Response) error {
			if response.StatusCode >= 300 && response.StatusCode < 400 && response.StatusCode != http.StatusNotModified {
				return errors.New("unexpected API redirect")
			}
			cookies := response.Cookies()
			response.Header.Del("Set-Cookie")
			for _, cookie := range cookies {
				if cookie.Name != sessionCookie {
					continue
				}
				cookie.Domain, cookie.Path = "", "/dashboard/"
				cookie.HttpOnly, cookie.SameSite = true, http.SameSiteStrictMode
				cookie.Secure = cookie.Secure || s.secure(response.Request)
				response.Header.Add("Set-Cookie", cookie.String())
			}
			response.Header.Set("Cache-Control", "no-store")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "Cannot reach TranscoderD. Please try again.", http.StatusBadGateway)
		},
	}
	r := mux.NewRouter().SkipClean(true)
	r.Use(dashboardHeaders)
	for _, location := range []string{"/", "/dashboard"} {
		r.HandleFunc(location, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/dashboard/", http.StatusSeeOther)
		}).Methods(http.MethodGet)
	}
	r.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }).Methods(http.MethodGet)
	r.HandleFunc("/dashboard/", s.serveDashboard).Methods(http.MethodGet)
	r.HandleFunc("/dashboard/login", s.serveLogin).Methods(http.MethodGet)
	r.PathPrefix("/dashboard/api/").HandlerFunc(s.serveAPI)
	d := r.PathPrefix("/dashboard/").Subrouter()
	s.registerDashboardPWA(d)
	assets, err := fs.Sub(dashboardAssets, "dashboard")
	if err != nil {
		return nil, fmt.Errorf("frontend assets: %w", err)
	}
	for _, name := range dashboardPublicAssets {
		d.Handle("/"+name, http.StripPrefix("/dashboard/", dashboardPublicFileServer(http.FS(assets), name))).Methods(http.MethodGet, http.MethodHead)
	}
	s.handler = r
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }

func (s *Server) Close() { s.transport.CloseIdleConnections() }

func copySession(out, in *http.Request) {
	if cookie, err := in.Cookie(sessionCookie); err == nil {
		out.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie.Value})
	}
}

func (s *Server) secure(r *http.Request) bool {
	if s.public != nil && s.public.Scheme == "https" {
		return true
	}
	if r == nil {
		return false
	}
	forwarded, _ := r.Context().Value(secureContextKey{}).(bool)
	return r.TLS != nil || forwarded
}

func (s *Server) mutationAllowed(r *http.Request) bool {
	if r.Header.Get("X-Dashboard-Request") != "1" || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	if raw := r.Header.Get("Origin"); raw != "" {
		origin, err := parseOrigin(raw)
		if err != nil {
			return false
		}
		want := s.public
		if want == nil {
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			want = &url.URL{Scheme: scheme, Host: r.Host}
		}
		return origin.Scheme == want.Scheme && strings.EqualFold(origin.Hostname(), want.Hostname()) && originPort(origin) == originPort(want)
	}
	return true
}

func originPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

var workerSettingsPath = regexp.MustCompile(`^/dashboard/api/workers/[^/]+/settings$`)
var mediaObjectPath = regexp.MustCompile(`^/dashboard/api/media/(info|file|preview|jobs|operations)/[^/]+$`)

func allowedAPI(method, p string) bool {
	if path.Clean(p) != p || strings.ContainsAny(p, "\x00\r\n") {
		return false
	}
	switch method {
	case http.MethodGet:
		return p == "/dashboard/api/session" || p == "/dashboard/api/stats" || p == "/dashboard/api/media/roots" || p == "/dashboard/api/media/browse" || mediaObjectPath.MatchString(p)
	case http.MethodHead:
		return strings.HasPrefix(p, "/dashboard/api/media/file/") && mediaObjectPath.MatchString(p)
	case http.MethodPost:
		return p == "/dashboard/api/login" || p == "/dashboard/api/logout" || p == "/dashboard/api/media/scans" || p == "/dashboard/api/media/batches"
	case http.MethodPut:
		return workerSettingsPath.MatchString(p)
	case http.MethodDelete:
		return strings.HasPrefix(p, "/dashboard/api/media/operations/") && mediaObjectPath.MatchString(p)
	}
	return false
}

func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
	if !allowedAPI(r.Method, r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && !s.mutationAllowed(r) {
		http.Error(w, "Same-origin request required", http.StatusForbidden)
		return
	}
	// Carry frontend TLS state across the private API hop without trusting headers.
	if s.secure(r) {
		r = r.WithContext(context.WithValue(r.Context(), secureContextKey{}, true))
	}
	s.proxy.ServeHTTP(w, r)
}

type secureContextKey struct{}

func (s *Server) sessionStatus(r *http.Request) int {
	u := *s.api
	u.Path = "/dashboard/api/session"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u.String(), nil)
	if err != nil {
		return http.StatusServiceUnavailable
	}
	copySession(req, r)
	req.Header.Set("Authorization", r.Header.Get("Authorization"))
	response, err := s.client.Do(req)
	if err != nil {
		return http.StatusServiceUnavailable
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode == http.StatusNoContent || response.StatusCode == http.StatusUnauthorized {
		return response.StatusCode
	}
	return http.StatusServiceUnavailable
}

func (s *Server) serveDashboard(w http.ResponseWriter, r *http.Request) {
	switch s.sessionStatus(r) {
	case http.StatusNoContent:
		serveDashboardHTML(w, "index.html")
	case http.StatusUnauthorized:
		http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
	default:
		http.Error(w, "Cannot reach TranscoderD. Please try again.", http.StatusServiceUnavailable)
	}
}

func (s *Server) serveLogin(w http.ResponseWriter, r *http.Request) {
	if s.sessionStatus(r) == http.StatusNoContent {
		http.Redirect(w, r, "/dashboard/", http.StatusSeeOther)
		return
	}
	serveDashboardHTML(w, "login.html")
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
