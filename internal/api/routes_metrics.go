package api

import (
	"net/http"

	"github.com/meistro57/frontpocket/internal/metrics"
)

// handleMetrics exposes Prometheus text exposition. It sits behind the same
// middleware chain as every other route, so API key auth still applies.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	metrics.Default().Render(w)
}
