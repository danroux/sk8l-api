package main

import (
	"encoding/json"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

// healthStatus is the JSON body returned by the diagnostic endpoints.
type healthStatus struct {
	Status string `json:"status"`
}

// setupHTTPRoutes registers all HTTP handlers on mux. It is extracted from
// main() so the function stays within the revive function-length limit.
func setupHTTPRoutes(mux *http.ServeMux, s *Sk8lServer) {
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc("/readyz", readyzHandler(s))
}

// healthzHandler is a liveness probe: it reports that the process is running.
// Kubernetes restarts the pod if this endpoint stops responding.
func healthzHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthStatus{Status: "ok"})
}

// readyzHandler returns a readiness probe handler scoped to sk8lServer.
// It reports 200 when the backing Badger DB is healthy and 503 otherwise.
// Kubernetes stops routing traffic to the pod when this returns non-2xx.
func readyzHandler(s *Sk8lServer) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if s.IsHealthy() {
			writeJSON(w, http.StatusOK, healthStatus{Status: "ready"})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, healthStatus{Status: "not ready"})
	}
}

// writeJSON serializes v as JSON and writes it to w with the given status code.
// Errors are logged but not propagated — the connection will be closed by the
// HTTP server if the write fails.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Error().Err(err).Str("operation", "writeJSON").Send()
	}
}
