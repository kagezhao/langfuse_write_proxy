package main

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestAllowedIngestionIsForwardedWithProjectLangfuseAuth(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotProxyToken string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotProxyToken = r.Header.Get("X-Proxy-Token")
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()

	handler := NewHandler(testConfig(t, testProject{
		upstream:  upstream.URL,
		publicKey: "pk-lf-test",
		secretKey: "sk-lf-test",
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/public/ingestion", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("pk-lf-test:sk-lf-none")))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if gotPath != "/api/public/ingestion" {
		t.Fatalf("upstream path = %q", gotPath)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("pk-lf-test:sk-lf-test"))
	if gotAuth != wantAuth {
		t.Fatalf("Authorization = %q, want %q", gotAuth, wantAuth)
	}
	if gotProxyToken != "" {
		t.Fatalf("X-Proxy-Token leaked upstream: %q", gotProxyToken)
	}
}

func TestPublicKeySelectsDifferentUpstreams(t *testing.T) {
	var upstreamAHit bool
	var upstreamBAuth string

	upstreamA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamAHit = true
		w.WriteHeader(http.StatusAccepted)
	}))
	defer upstreamA.Close()

	upstreamB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamBAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer upstreamB.Close()

	handler := NewHandler(testConfig(t,
		testProject{upstream: upstreamA.URL, publicKey: "pk-a", secretKey: "sk-a"},
		testProject{upstream: upstreamB.URL, publicKey: "pk-b", secretKey: "sk-b"},
	))
	req := httptest.NewRequest(http.MethodPost, "/api/public/otel/v1/traces", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("pk-b:sk-lf-none")))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if upstreamAHit {
		t.Fatal("request was sent to upstream A")
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("pk-b:sk-b"))
	if upstreamBAuth != wantAuth {
		t.Fatalf("upstream B Authorization = %q, want %q", upstreamBAuth, wantAuth)
	}
}

func TestLangfuseSDKStyleBasicAuthCanSelectProjectByPublicKey(t *testing.T) {
	var gotAuth string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer upstream.Close()

	handler := NewHandler(testConfig(t, testProject{
		upstream:  upstream.URL,
		publicKey: "pk-lf-test",
		secretKey: "sk-lf-test",
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/public/ingestion", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("pk-lf-test:sk-lf-none")))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("pk-lf-test:sk-lf-test"))
	if gotAuth != wantAuth {
		t.Fatalf("Authorization = %q, want %q", gotAuth, wantAuth)
	}
}

func TestPublicReadEndpointIsBlocked(t *testing.T) {
	handler := NewHandler(testConfig(t, testProject{
		upstream:  "https://langfuse.example.com",
		publicKey: "pk-lf-test",
		secretKey: "sk-lf-test",
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/public/traces", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("pk-lf-test:sk-lf-none")))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestPublicKeyIsRequired(t *testing.T) {
	handler := NewHandler(testConfig(t, testProject{
		upstream:  "https://langfuse.example.com",
		publicKey: "pk-lf-test",
		secretKey: "sk-lf-test",
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/public/ingestion", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestClientSecretCanBeAnyValue(t *testing.T) {
	var gotAuth string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer upstream.Close()

	handler := NewHandler(testConfig(t, testProject{
		upstream:  upstream.URL,
		publicKey: "pk-lf-test",
		secretKey: "sk-lf-test",
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/public/ingestion", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("pk-lf-test:any-client-value")))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("pk-lf-test:sk-lf-test"))
	if gotAuth != wantAuth {
		t.Fatalf("Authorization = %q, want %q", gotAuth, wantAuth)
	}
}

func TestClientSecretCanBeEmpty(t *testing.T) {
	var gotAuth string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer upstream.Close()

	handler := NewHandler(testConfig(t, testProject{
		upstream:  upstream.URL,
		publicKey: "pk-lf-test",
		secretKey: "sk-lf-test",
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/public/ingestion", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("pk-lf-test:")))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("pk-lf-test:sk-lf-test"))
	if gotAuth != wantAuth {
		t.Fatalf("Authorization = %q, want %q", gotAuth, wantAuth)
	}
}

func TestRealLangfuseSecretFromClientIsRejected(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("request should not reach upstream")
	}))
	defer upstream.Close()

	handler := NewHandler(testConfig(t, testProject{
		upstream:  upstream.URL,
		publicKey: "pk-lf-test",
		secretKey: "sk-lf-test",
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/public/ingestion", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("pk-lf-test:sk-lf-test")))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

type testProject struct {
	upstream  string
	publicKey string
	secretKey string
}

func testConfig(t *testing.T, projects ...testProject) Config {
	t.Helper()

	cfg := Config{
		ListenAddr:   ":0",
		MaxBodyBytes: 20 << 20,
	}

	for i, item := range projects {
		u, err := url.Parse(item.upstream)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Projects = append(cfg.Projects, Project{
			Name:              string(rune('a' + i)),
			UpstreamURL:       u,
			LangfusePublicKey: item.publicKey,
			LangfuseSecretKey: item.secretKey,
		})
	}

	return cfg
}
