package api

import (
	"net/http"

	"github.com/meistro57/frontpocket/internal/version"
)

func (s *Server) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.openAPISpec())
}

func (s *Server) openAPISpec() map[string]any {
	serverURL := s.cfg.App.PublicURL
	if serverURL == "" {
		serverURL = "http://localhost:8088"
	}

	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "FrontPocket API",
			"version":     version.Current,
			"description": "Source-backed memory retrieval API for FrontPocket.",
		},
		"servers": []map[string]any{{"url": serverURL}},
		"paths": map[string]any{
			"/health": map[string]any{
				"get": map[string]any{
					"operationId": "getHealth",
					"summary":     "Health status",
				},
			},
			"/memory/search": map[string]any{
				"post": map[string]any{
					"operationId": "searchMemory",
					"summary":     "Search memory records",
				},
			},
			"/memory/context-pack": map[string]any{
				"post": map[string]any{
					"operationId": "buildContextPack",
					"summary":     "Build a context pack from query and filters",
				},
			},
			"/memory/ingest/chat": map[string]any{
				"post": map[string]any{
					"operationId": "ingestChatMemory",
					"summary":     "Ingest normalized chat records",
				},
			},
		},
	}
}
