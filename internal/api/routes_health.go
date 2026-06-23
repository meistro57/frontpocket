package api

import (
	"context"
	"net/http"
	"time"

	"github.com/meistro57/frontpocket/internal/version"
)

type healthResponse struct {
	Status  string `json:"status"`
	Qdrant  string `json:"qdrant"`
	Redis   string `json:"redis"`
	Version string `json:"version"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	resp := healthResponse{Status: "ok", Qdrant: "connected", Redis: "connected", Version: version.Current}
	if err := s.qdrant.Health(ctx); err != nil {
		resp.Status = "degraded"
		resp.Qdrant = "unavailable"
	}
	if err := s.redis.Health(ctx); err != nil {
		resp.Status = "degraded"
		resp.Redis = "unavailable"
	}

	writeJSON(w, http.StatusOK, resp)
}
