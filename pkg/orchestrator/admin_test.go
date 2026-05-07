package orchestrator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHealthCheckRestate_okWhen200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/restate/health" {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := HealthCheckRestate(srv.URL); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestHealthCheckRestate_failsOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if err := HealthCheckRestate(srv.URL); err == nil {
		t.Fatal("expected error on 503")
	}
}

func TestHealthCheckRestate_failsWhenUnreachable(t *testing.T) {
	if err := HealthCheckRestate("http://127.0.0.1:1"); err == nil {
		t.Fatal("expected unreachable error")
	}
}

func TestRegisterDeployment_postsExpectedBody(t *testing.T) {
	var seen struct {
		path        string
		body        []byte
		contentType string
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.path = r.URL.Path
		seen.contentType = r.Header.Get("Content-Type")
		buf := make([]byte, 256)
		n, _ := r.Body.Read(buf)
		seen.body = buf[:n]
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	if err := RegisterDeployment(context.Background(), srv.URL, "http://flow:9080"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if seen.path != "/deployments" {
		t.Errorf("path: %q", seen.path)
	}
	if seen.contentType != "application/json" {
		t.Errorf("content-type: %q", seen.contentType)
	}
	var body map[string]any
	if err := json.Unmarshal(seen.body, &body); err != nil {
		t.Fatalf("body json: %v", err)
	}
	if body["uri"] != "http://flow:9080" || body["force"] != true {
		t.Errorf("body: %+v", body)
	}
}

func TestRegisterDeployment_returnsErrorOn5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("oops"))
	}))
	defer srv.Close()

	if err := RegisterDeployment(context.Background(), srv.URL, "http://flow:9080"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRegisterDeploymentWithRetry_succeedsAfterFailures(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	if err := RegisterDeploymentWithRetry(context.Background(), srv.URL, "http://flow:9080", 5, 5*time.Millisecond); err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestRegisterDeploymentWithRetry_givesUpAfterMaxAttempts(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := RegisterDeploymentWithRetry(context.Background(), srv.URL, "http://flow:9080", 3, 1*time.Millisecond)
	if err == nil {
		t.Fatal("expected failure")
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("attempts: got %d, want 3", got)
	}
}

func TestRegisterDeploymentWithRetry_respectsContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	if err := RegisterDeploymentWithRetry(ctx, srv.URL, "http://flow:9080", 100, 50*time.Millisecond); err == nil {
		t.Fatal("expected error from cancelled context")
	}
}
