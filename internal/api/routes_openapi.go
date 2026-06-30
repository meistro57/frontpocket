package api

import (
	"net/http"
	"strings"

	"github.com/meistro57/frontpocket/internal/version"
)

func (s *Server) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.openAPISpec())
}

func (s *Server) openAPISpec() map[string]any {
	serverURL := strings.TrimSpace(s.cfg.App.PublicURL)
	if serverURL == "" {
		serverURL = "http://localhost:8088"
	}

	apiKeyHeader := strings.TrimSpace(s.cfg.Security.APIKeyHeader)
	if apiKeyHeader == "" {
		apiKeyHeader = "X-FrontPocket-Key"
	}

	spec := map[string]any{
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
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Service health.",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/HealthResponse"},
								},
							},
						},
					},
				},
			},
			"/openapi.json": map[string]any{
				"get": map[string]any{
					"operationId": "getOpenAPISchema",
					"summary":     "Fetch OpenAPI schema",
					"responses": map[string]any{
						"200": map[string]any{
							"description": "OpenAPI schema payload.",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"type": "object"},
								},
							},
						},
					},
				},
			},
			"/memory/stats": map[string]any{
				"get": map[string]any{
					"operationId": "getMemoryStats",
					"summary":     "Get aggregated memory stats",
					"parameters": []map[string]any{
						{"$ref": "#/components/parameters/projectFilter"},
						{"$ref": "#/components/parameters/memoryKindFilter"},
						{"$ref": "#/components/parameters/speakerFilter"},
						{"$ref": "#/components/parameters/sourceTypeFilter"},
						{"$ref": "#/components/parameters/conversationIDFilter"},
						{"$ref": "#/components/parameters/tagFilter"},
						{"$ref": "#/components/parameters/tagsFilter"},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Aggregated memory statistics.",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/MemoryStats"},
								},
							},
						},
						"401": map[string]any{"$ref": "#/components/responses/UnauthorizedError"},
						"500": map[string]any{"$ref": "#/components/responses/InternalServerError"},
					},
				},
			},
			"/memory/session": map[string]any{
				"post": map[string]any{
					"operationId": "upsertOrLoadSessionState",
					"summary":     "Create, update, or load session state",
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/SessionRequest"},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Session state response.",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/SessionResponse"},
								},
							},
						},
						"400": map[string]any{"$ref": "#/components/responses/ValidationError"},
						"401": map[string]any{"$ref": "#/components/responses/UnauthorizedError"},
						"500": map[string]any{"$ref": "#/components/responses/InternalServerError"},
					},
				},
				"delete": map[string]any{
					"operationId": "deleteSessionState",
					"summary":     "Delete a session state record",
					"parameters": []map[string]any{
						{"$ref": "#/components/parameters/sessionIDQuery"},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Session delete response.",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/SessionResponse"},
								},
							},
						},
						"400": map[string]any{"$ref": "#/components/responses/ValidationError"},
						"401": map[string]any{"$ref": "#/components/responses/UnauthorizedError"},
						"500": map[string]any{"$ref": "#/components/responses/InternalServerError"},
					},
				},
			},
			"/memory/search": map[string]any{
				"post": map[string]any{
					"operationId": "searchMemory",
					"summary":     "Search memory records",
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/SearchRequest"},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Search results.",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/SearchResponse"},
								},
							},
						},
						"400": map[string]any{"$ref": "#/components/responses/ValidationError"},
						"401": map[string]any{"$ref": "#/components/responses/UnauthorizedError"},
						"500": map[string]any{"$ref": "#/components/responses/InternalServerError"},
					},
				},
			},
			"/memory/context-pack": map[string]any{
				"post": map[string]any{
					"operationId": "buildContextPack",
					"summary":     "Build a context pack from query and filters",
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/ContextPackRequest"},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Context pack response.",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/ContextPackResponse"},
								},
							},
						},
						"400": map[string]any{"$ref": "#/components/responses/ValidationError"},
						"401": map[string]any{"$ref": "#/components/responses/UnauthorizedError"},
						"500": map[string]any{"$ref": "#/components/responses/InternalServerError"},
					},
				},
			},
			"/memory/chat": map[string]any{
				"post": map[string]any{
					"operationId": "chatWithMemory",
					"summary":     "Generate MindDrill chat response with split memory context",
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/ChatMessageRequest"},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Chat response with memory usage details.",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/ChatMessageResponse"},
								},
							},
						},
						"400": map[string]any{"$ref": "#/components/responses/ValidationError"},
						"401": map[string]any{"$ref": "#/components/responses/UnauthorizedError"},
						"500": map[string]any{"$ref": "#/components/responses/InternalServerError"},
					},
				},
			},
			"/memory/canon/proposed": map[string]any{
				"get": map[string]any{
					"operationId": "listProposedCanon",
					"summary":     "List proposed canon candidates",
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Proposed canon candidates.",
							"content":     map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object"}}},
						},
						"401": map[string]any{"$ref": "#/components/responses/UnauthorizedError"},
						"500": map[string]any{"$ref": "#/components/responses/InternalServerError"},
					},
				},
			},
			"/memory/canon/proposed/{id}": map[string]any{
				"get": map[string]any{
					"operationId": "getProposedCanon",
					"summary":     "Get proposed canon candidate",
					"responses": map[string]any{
						"200": map[string]any{"description": "Candidate payload."},
						"401": map[string]any{"$ref": "#/components/responses/UnauthorizedError"},
						"404": map[string]any{"$ref": "#/components/responses/ValidationError"},
					},
				},
			},
			"/memory/canon/proposed/{id}/approve": map[string]any{
				"post": map[string]any{
					"operationId": "approveProposedCanon",
					"summary":     "Approve proposed canon candidate",
					"responses": map[string]any{
						"200": map[string]any{"description": "Approved candidate."},
						"401": map[string]any{"$ref": "#/components/responses/UnauthorizedError"},
						"500": map[string]any{"$ref": "#/components/responses/InternalServerError"},
					},
				},
			},
			"/memory/canon/proposed/{id}/reject": map[string]any{
				"post": map[string]any{
					"operationId": "rejectProposedCanon",
					"summary":     "Reject proposed canon candidate",
					"responses": map[string]any{
						"200": map[string]any{"description": "Rejected candidate."},
						"401": map[string]any{"$ref": "#/components/responses/UnauthorizedError"},
						"500": map[string]any{"$ref": "#/components/responses/InternalServerError"},
					},
				},
			},
			"/memory/canon/proposed/{id}/merge": map[string]any{
				"post": map[string]any{
					"operationId": "mergeProposedCanon",
					"summary":     "Merge proposed canon candidate",
					"responses": map[string]any{
						"200": map[string]any{"description": "Merged candidate."},
						"401": map[string]any{"$ref": "#/components/responses/UnauthorizedError"},
						"500": map[string]any{"$ref": "#/components/responses/InternalServerError"},
					},
				},
			},
			"/memory/ingest/chat": map[string]any{
				"post": map[string]any{
					"operationId": "ingestChatMemory",
					"summary":     "Ingest normalized chat records",
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/IngestChatRequest"},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Ingestion response.",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/IngestChatResponse"},
								},
							},
						},
						"400": map[string]any{"$ref": "#/components/responses/ValidationError"},
						"401": map[string]any{"$ref": "#/components/responses/UnauthorizedError"},
						"500": map[string]any{"$ref": "#/components/responses/InternalServerError"},
					},
				},
			},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"ApiKeyAuth": map[string]any{
					"type": "apiKey",
					"in":   "header",
					"name": apiKeyHeader,
				},
			},
			"parameters": map[string]any{
				"projectFilter": map[string]any{
					"in":          "query",
					"name":        "project",
					"required":    false,
					"description": "Filter by project.",
					"schema":      map[string]any{"type": "string"},
				},
				"memoryKindFilter": map[string]any{
					"in":          "query",
					"name":        "memory_kind",
					"required":    false,
					"description": "Filter by memory kind.",
					"schema":      map[string]any{"type": "string"},
				},
				"speakerFilter": map[string]any{
					"in":          "query",
					"name":        "speaker",
					"required":    false,
					"description": "Filter by speaker.",
					"schema":      map[string]any{"type": "string"},
				},
				"sourceTypeFilter": map[string]any{
					"in":          "query",
					"name":        "source_type",
					"required":    false,
					"description": "Filter by source type.",
					"schema":      map[string]any{"type": "string"},
				},
				"conversationIDFilter": map[string]any{
					"in":          "query",
					"name":        "conversation_id",
					"required":    false,
					"description": "Filter by conversation id.",
					"schema":      map[string]any{"type": "string"},
				},
				"tagFilter": map[string]any{
					"in":          "query",
					"name":        "tag",
					"required":    false,
					"description": "Filter by tag. Can be repeated.",
					"schema":      map[string]any{"type": "string"},
				},
				"tagsFilter": map[string]any{
					"in":          "query",
					"name":        "tags",
					"required":    false,
					"description": "Comma-separated tag list.",
					"schema":      map[string]any{"type": "string"},
				},
				"sessionIDQuery": map[string]any{
					"in":          "query",
					"name":        "session_id",
					"required":    true,
					"description": "Session identifier.",
					"schema":      map[string]any{"type": "string", "minLength": 1},
				},
			},
			"responses": map[string]any{
				"ValidationError": map[string]any{
					"description": "Invalid request payload.",
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/ErrorEnvelope"},
						},
					},
				},
				"UnauthorizedError": map[string]any{
					"description": "Missing or invalid API key.",
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/ErrorEnvelope"},
						},
					},
				},
				"InternalServerError": map[string]any{
					"description": "Internal server error.",
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/ErrorEnvelope"},
						},
					},
				},
			},
			"schemas": map[string]any{
				"HealthResponse": map[string]any{
					"type":     "object",
					"required": []string{"status", "qdrant", "redis", "version"},
					"properties": map[string]any{
						"status":  map[string]any{"type": "string"},
						"qdrant":  map[string]any{"type": "string"},
						"redis":   map[string]any{"type": "string"},
						"version": map[string]any{"type": "string"},
					},
				},
				"ErrorBody": map[string]any{
					"type":     "object",
					"required": []string{"code", "message"},
					"properties": map[string]any{
						"code":    map[string]any{"type": "string"},
						"message": map[string]any{"type": "string"},
						"detail":  map[string]any{"type": "string"},
					},
				},
				"ErrorEnvelope": map[string]any{
					"type":     "object",
					"required": []string{"error"},
					"properties": map[string]any{
						"error": map[string]any{"$ref": "#/components/schemas/ErrorBody"},
					},
				},
				"SearchFilters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project":         map[string]any{"type": "string"},
						"memory_kind":     map[string]any{"type": "string"},
						"speaker":         map[string]any{"type": "string"},
						"source_type":     map[string]any{"type": "string"},
						"conversation_id": map[string]any{"type": "string"},
						"tags": map[string]any{
							"type":  "array",
							"items": map[string]any{"type": "string"},
						},
					},
				},
				"SearchRequest": map[string]any{
					"type":     "object",
					"required": []string{"query"},
					"properties": map[string]any{
						"query": map[string]any{
							"type":        "string",
							"minLength":   1,
							"description": "Search query.",
						},
						"limit": map[string]any{
							"type":        "integer",
							"minimum":     1,
							"maximum":     s.cfg.Search.MaxLimit,
							"description": "Result limit.",
						},
						"filters": map[string]any{"$ref": "#/components/schemas/SearchFilters"},
					},
				},
				"SearchResult": map[string]any{
					"type":     "object",
					"required": []string{"memory_id", "conversation_id", "source_title", "source_type", "timestamp", "speaker", "memory_kind", "summary", "score", "embedding_provider", "embedding_model", "embedding_dimensions"},
					"properties": map[string]any{
						"memory_id":       map[string]any{"type": "string"},
						"conversation_id": map[string]any{"type": "string"},
						"session_id":      map[string]any{"type": "string"},
						"source_title":    map[string]any{"type": "string"},
						"source_type":     map[string]any{"type": "string"},
						"timestamp":       map[string]any{"type": "string", "format": "date-time"},
						"speaker":         map[string]any{"type": "string"},
						"project":         map[string]any{"type": "string"},
						"memory_kind":     map[string]any{"type": "string"},
						"tags": map[string]any{
							"type":  "array",
							"items": map[string]any{"type": "string"},
						},
						"summary": map[string]any{"type": "string"},
						"source_quote": map[string]any{
							"type": "string",
						},
						"text": map[string]any{"type": "string"},
						"used_memory_ids": map[string]any{
							"type":  "array",
							"items": map[string]any{"type": "string"},
						},
						"score": map[string]any{
							"type":   "number",
							"format": "double",
						},
						"embedding_provider":   map[string]any{"type": "string"},
						"embedding_model":      map[string]any{"type": "string"},
						"embedding_dimensions": map[string]any{"type": "integer"},
					},
				},
				"SearchResponse": map[string]any{
					"type":     "object",
					"required": []string{"query", "results"},
					"properties": map[string]any{
						"query": map[string]any{"type": "string"},
						"results": map[string]any{
							"type":  "array",
							"items": map[string]any{"$ref": "#/components/schemas/SearchResult"},
						},
					},
				},
				"ContextPackRequest": map[string]any{
					"type":     "object",
					"required": []string{"query"},
					"properties": map[string]any{
						"query":            map[string]any{"type": "string", "minLength": 1},
						"limit":            map[string]any{"type": "integer", "minimum": 1, "maximum": s.cfg.ContextPack.MaxLimit},
						"filters":          map[string]any{"$ref": "#/components/schemas/SearchFilters"},
						"include_proposed": map[string]any{"type": "boolean"},
						"include_rejected": map[string]any{"type": "boolean"},
						"canonical_first":  map[string]any{"type": "boolean"},
					},
				},
				"ContextPackResponse": map[string]any{
					"type":     "object",
					"required": []string{"query", "memory_pack"},
					"properties": map[string]any{
						"query": map[string]any{"type": "string"},
						"memory_pack": map[string]any{
							"type":  "array",
							"items": map[string]any{"$ref": "#/components/schemas/SearchResult"},
						},
					},
				},
				"MessageRecord": map[string]any{
					"type":     "object",
					"required": []string{"conversation_id", "speaker", "text"},
					"properties": map[string]any{
						"conversation_id": map[string]any{"type": "string"},
						"timestamp":       map[string]any{"type": "string", "format": "date-time"},
						"speaker":         map[string]any{"type": "string"},
						"text":            map[string]any{"type": "string"},
						"source_type":     map[string]any{"type": "string"},
						"source_title":    map[string]any{"type": "string"},
						"project":         map[string]any{"type": "string"},
						"memory_kind":     map[string]any{"type": "string"},
						"tags": map[string]any{
							"type":  "array",
							"items": map[string]any{"type": "string"},
						},
					},
				},
				"IngestChatRequest": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"source_title": map[string]any{"type": "string"},
						"source_type":  map[string]any{"type": "string"},
						"project":      map[string]any{"type": "string"},
						"records": map[string]any{
							"type":  "array",
							"items": map[string]any{"$ref": "#/components/schemas/MessageRecord"},
						},
						"jsonl": map[string]any{"type": "string"},
					},
				},
				"IngestChatResponse": map[string]any{
					"type":     "object",
					"required": []string{"inserted_count", "memory_ids"},
					"properties": map[string]any{
						"inserted_count": map[string]any{"type": "integer"},
						"memory_ids": map[string]any{
							"type":  "array",
							"items": map[string]any{"type": "string"},
						},
					},
				},
				"MemoryStats": map[string]any{
					"type":     "object",
					"required": []string{"total"},
					"properties": map[string]any{
						"total": map[string]any{"type": "integer"},
						"by_kind": map[string]any{
							"type":                 "object",
							"additionalProperties": map[string]any{"type": "integer"},
						},
						"by_speaker": map[string]any{
							"type":                 "object",
							"additionalProperties": map[string]any{"type": "integer"},
						},
						"by_project": map[string]any{
							"type":                 "object",
							"additionalProperties": map[string]any{"type": "integer"},
						},
					},
				},
				"SessionRequest": map[string]any{
					"type":     "object",
					"required": []string{"session_id"},
					"properties": map[string]any{
						"session_id":        map[string]any{"type": "string", "minLength": 1},
						"project":           map[string]any{"type": "string"},
						"active_summary":    map[string]any{"type": "string"},
						"recent_memory_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"metadata": map[string]any{
							"type":                 "object",
							"additionalProperties": map[string]any{"type": "string"},
						},
						"ttl_seconds": map[string]any{"type": "integer", "minimum": 1},
						"load_only":   map[string]any{"type": "boolean"},
					},
				},
				"SessionState": map[string]any{
					"type":     "object",
					"required": []string{"session_id", "updated_at"},
					"properties": map[string]any{
						"session_id":        map[string]any{"type": "string"},
						"project":           map[string]any{"type": "string"},
						"active_summary":    map[string]any{"type": "string"},
						"recent_memory_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"metadata": map[string]any{
							"type":                 "object",
							"additionalProperties": map[string]any{"type": "string"},
						},
						"updated_at": map[string]any{"type": "string", "format": "date-time"},
					},
				},
				"SessionResponse": map[string]any{
					"type":     "object",
					"required": []string{"found"},
					"properties": map[string]any{
						"found": map[string]any{"type": "boolean"},
						"state": map[string]any{"$ref": "#/components/schemas/SessionState"},
					},
				},
				"ChatMessageRequest": map[string]any{
					"type":     "object",
					"required": []string{"session_id", "message"},
					"properties": map[string]any{
						"session_id":        map[string]any{"type": "string", "minLength": 1},
						"message":           map[string]any{"type": "string", "minLength": 1},
						"system_prompt":     map[string]any{"type": "string", "description": "Optional user-provided persona or system prompt guidance for this chat turn."},
						"project":           map[string]any{"type": "string"},
						"frontpocket_top_k": map[string]any{"type": "integer", "minimum": 1, "maximum": s.cfg.Search.MaxLimit},
						"minddrill_top_k":   map[string]any{"type": "integer", "minimum": 1, "maximum": s.cfg.Search.MaxLimit},
						"remember_this":     map[string]any{"type": "boolean"},
					},
				},
				"ChatMessageResponse": map[string]any{
					"type":     "object",
					"required": []string{"answer", "used_frontpocket_memories", "used_minddrill_memories", "context_pack", "model", "provider"},
					"properties": map[string]any{
						"answer":                    map[string]any{"type": "string"},
						"used_frontpocket_memories": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/SearchResult"}},
						"used_minddrill_memories":   map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/SearchResult"}},
						"context_pack":              map[string]any{"type": "string"},
						"model":                     map[string]any{"type": "string"},
						"provider":                  map[string]any{"type": "string"},
					},
				},
			},
		},
	}

	if s.cfg.Security.RequireAPIKey {
		spec["security"] = []map[string][]string{{"ApiKeyAuth": {}}}
	}

	return spec
}
