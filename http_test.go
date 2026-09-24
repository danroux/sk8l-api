package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danroux/sk8l/internal/k8s"
	"github.com/danroux/sk8l/internal/store"
	"k8s.io/client-go/kubernetes/fake"
)

// newHTTPTestServer builds a test Sk8lServer with a live in-memory Badger DB
// and returns it together with a teardown function.
func newHTTPTestServer(t *testing.T) (*Sk8lServer, func()) {
	t.Helper()
	db := setupBadger(t)
	fakeClientset := fake.NewClientset()
	k8sClient := k8s.NewClientWithInterface(fakeClientset)
	s := &Sk8lServer{
		CronJobDBStore: &store.CronJobDBStore{
			DB:        db,
			K8sClient: k8sClient,
		},
	}
	return s, func() { db.Close() }
}

func TestHealthzHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	healthzHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var body healthStatus
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("expected status %q, got %q", "ok", body.Status)
	}
}

func TestReadyzHandler_Healthy(t *testing.T) {
	s, teardown := newHTTPTestServer(t)
	defer teardown()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()

	readyzHandler(s)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var body healthStatus
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body.Status != "ready" {
		t.Errorf("expected status %q, got %q", "ready", body.Status)
	}
}

func TestReadyzHandler_Degraded(t *testing.T) {
	s, teardown := newHTTPTestServer(t)
	// Close the DB before the request to simulate a degraded state.
	teardown()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()

	readyzHandler(s)(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rr.Code)
	}

	var body healthStatus
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body.Status != "not ready" {
		t.Errorf("expected status %q, got %q", "not ready", body.Status)
	}
}

func TestSetupHTTPRoutes(t *testing.T) {
	s, teardown := newHTTPTestServer(t)
	defer teardown()

	mux := &http.ServeMux{}
	setupHTTPRoutes(mux, s)

	routes := []struct {
		path           string
		expectedStatus int
	}{
		{"/healthz", http.StatusOK},
		{"/readyz", http.StatusOK},
		{"/metrics", http.StatusOK},
	}

	for _, tc := range routes {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			if rr.Code != tc.expectedStatus {
				t.Errorf("GET %s: expected %d, got %d", tc.path, tc.expectedStatus, rr.Code)
			}
		})
	}
}
