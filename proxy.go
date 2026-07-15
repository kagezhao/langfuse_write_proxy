package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

var allowedWrites = map[string]struct{}{
	"/api/public/ingestion":       {},
	"/api/public/otel/v1/traces":  {},
	"/api/public/otel/v1/traces/": {},
	"/api/public/ingestion/":      {},
}

type Handler struct {
	cfg      Config
	projects []projectHandler
}

type projectHandler struct {
	project Project
	proxy   *httputil.ReverseProxy
}

func NewHandler(cfg Config) http.Handler {
	h := &Handler{cfg: cfg}
	transport := newUpstreamTransport(cfg)
	for _, project := range cfg.Projects {
		h.projects = append(h.projects, projectHandler{
			project: project,
			proxy:   newReverseProxy(project, transport),
		})
	}
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	projectName := "-"
	result := "ok"
	defer func() {
		log.Printf(
			"request remote=%q method=%s path=%q status=%d bytes=%d duration_ms=%d project=%q result=%q user_agent=%q",
			r.RemoteAddr,
			r.Method,
			r.URL.RequestURI(),
			rec.status,
			rec.bytes,
			time.Since(start).Milliseconds(),
			projectName,
			result,
			r.UserAgent(),
		)
	}()

	switch r.URL.Path {
	case "/healthz", "/readyz":
		writeJSON(rec, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	if _, ok := allowedWrites[r.URL.Path]; !ok {
		status := http.StatusNotFound
		if strings.HasPrefix(r.URL.Path, "/api/public/") {
			status = http.StatusForbidden
		}
		result = "endpoint is not allowed"
		writeJSON(rec, status, map[string]string{"error": result})
		return
	}

	if r.Method != http.MethodPost {
		rec.Header().Set("Allow", http.MethodPost)
		result = "method is not allowed"
		writeJSON(rec, http.StatusMethodNotAllowed, map[string]string{"error": result})
		return
	}

	project, err := h.authorize(r)
	if err != nil {
		result = err.Error()
		writeJSON(rec, http.StatusUnauthorized, map[string]string{"error": result})
		return
	}
	projectName = project.project.Name

	if h.cfg.MaxBodyBytes > 0 {
		r.Body = http.MaxBytesReader(rec, r.Body, h.cfg.MaxBodyBytes)
	}

	project.proxy.ServeHTTP(rec, r)
}

func (h *Handler) authorize(r *http.Request) (*projectHandler, error) {
	publicKey, clientSecret := clientCredentials(r)
	if publicKey == "" {
		return nil, errors.New("missing langfuse public key")
	}

	for i := range h.projects {
		project := &h.projects[i]
		if publicKey == project.project.LangfusePublicKey {
			if sameSecret(clientSecret, project.project.LangfuseSecretKey) {
				return nil, errors.New("client secret must not be the real langfuse secret key")
			}
			return &h.projects[i], nil
		}
	}
	return nil, errors.New("unknown langfuse public key")
}

func clientCredentials(r *http.Request) (string, string) {
	auth := r.Header.Get("Authorization")
	if username, password, ok := basicAuth(auth); ok {
		return username, password
	}

	return strings.TrimSpace(r.Header.Get("X-Langfuse-Public-Key")), strings.TrimSpace(r.Header.Get("X-Langfuse-Client-Secret"))
}

func sameSecret(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return a == b
}

func basicAuth(header string) (string, string, bool) {
	scheme, encoded, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Basic") {
		return "", "", false
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", "", false
	}

	username, password, ok := strings.Cut(string(decoded), ":")
	if !ok {
		return strings.TrimSpace(string(decoded)), "", true
	}
	return strings.TrimSpace(username), strings.TrimSpace(password), true
}

func newReverseProxy(project Project, transport http.RoundTripper) *httputil.ReverseProxy {
	upstream := *project.UpstreamURL
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(project.LangfusePublicKey+":"+project.LangfuseSecretKey))

	p := httputil.NewSingleHostReverseProxy(&upstream)
	p.Transport = transport
	originalDirector := p.Director
	p.Director = func(r *http.Request) {
		originalPath := r.URL.Path
		originalRawQuery := r.URL.RawQuery

		originalDirector(r)
		joinUpstreamPath(r.URL, upstream.Path, originalPath)
		r.URL.RawQuery = originalRawQuery
		r.Host = upstream.Host

		r.Header.Del("X-Proxy-Token")
		r.Header.Set("Authorization", auth)
		r.Header.Set("X-Langfuse-Write-Proxy", "1")
		r.Header.Set("X-Langfuse-Write-Proxy-Project", project.Name)
	}
	p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("proxy error: project=%s method=%s path=%s error=%v", project.Name, r.Method, r.URL.Path, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream request failed"})
	}

	return p
}

func newUpstreamTransport(cfg Config) http.RoundTripper {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          cfg.UpstreamMaxIdleConns,
		MaxIdleConnsPerHost:   cfg.UpstreamMaxIdleConnsPerHost,
		IdleConnTimeout:       cfg.UpstreamIdleConnTimeout,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

func joinUpstreamPath(dst *url.URL, upstreamBasePath string, requestPath string) {
	if upstreamBasePath == "" {
		dst.Path = requestPath
		return
	}
	dst.Path = strings.TrimRight(upstreamBasePath, "/") + "/" + strings.TrimLeft(requestPath, "/")
}

func writeJSON(w http.ResponseWriter, status int, payload map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
