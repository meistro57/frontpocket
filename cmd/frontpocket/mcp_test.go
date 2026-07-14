package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMCPInitialize(t *testing.T) {
	server := &mcpServer{}
	resp := server.handleRequest(rpcRequest{JSONRPC: "2.0", ID: 1, Method: "initialize"})
	if resp == nil {
		t.Fatal("expected initialize response")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected initialize error: %+v", resp.Error)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected initialize result map, got %T", resp.Result)
	}
	if result["protocolVersion"] != "2024-11-05" {
		t.Fatalf("unexpected protocolVersion: %#v", result["protocolVersion"])
	}
}

func TestMCPToolCallSearchRequiresQuery(t *testing.T) {
	server := &mcpServer{}
	params, err := json.Marshal(toolCallParams{Name: "frontpocket_search", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	_, err = server.handleToolCall(params)
	if err == nil || !strings.Contains(err.Error(), "query is required") {
		t.Fatalf("expected query validation error, got: %v", err)
	}
}

func TestMCPToolCallSearchForwardsRequest(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/memory/search" {
			t.Fatalf("expected /memory/search path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query":"alpha","results":[]}`))
	}))
	defer api.Close()

	server := &mcpServer{apiBaseURL: api.URL, client: api.Client()}
	params, err := json.Marshal(toolCallParams{Name: "frontpocket_search", Arguments: map[string]any{"query": "alpha", "limit": 3}})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	result, err := server.handleToolCall(params)
	if err != nil {
		t.Fatalf("handleToolCall failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected successful tool result, got error content: %#v", result.Content)
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, `"query":"alpha"`) {
		t.Fatalf("expected response body in content, got %#v", result.Content)
	}
}

func TestMCPToolCallMarksAPIErrors(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"VALIDATION_ERROR","message":"query is required"}}`))
	}))
	defer api.Close()

	server := &mcpServer{apiBaseURL: api.URL, client: api.Client()}
	params, err := json.Marshal(toolCallParams{Name: "frontpocket_context_pack", Arguments: map[string]any{"query": "alpha"}})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	result, err := server.handleToolCall(params)
	if err != nil {
		t.Fatalf("handleToolCall failed: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected tool result to mark api failure")
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "400") {
		t.Fatalf("expected status code in error content, got %#v", result.Content)
	}
}
