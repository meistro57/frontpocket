package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfigEndpointReturnsAPIBaseURL(t *testing.T) {
	testServer := httptest.NewServer(newHandler("http://localhost:8181"))
	defer testServer.Close()

	resp, err := http.Get(testServer.URL + "/config.json")
	if err != nil {
		t.Fatalf("config request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var cfg struct {
		APIBaseURL string `json:"api_base_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		t.Fatalf("failed decoding config response: %v", err)
	}

	if cfg.APIBaseURL != "http://localhost:8181" {
		t.Fatalf("expected api_base_url to be %q, got %q", "http://localhost:8181", cfg.APIBaseURL)
	}
}

func TestRootServesRuntimeConfigAwareUI(t *testing.T) {
	testServer := httptest.NewServer(newHandler("http://localhost:8088"))
	defer testServer.Close()

	resp, err := http.Get(testServer.URL + "/")
	if err != nil {
		t.Fatalf("root request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed reading root response body: %v", err)
	}
	content := string(body)

	if !strings.Contains(content, "fetch('/config.json'") {
		t.Fatalf("expected UI to fetch runtime config.json, got:\n%s", content)
	}
	if strings.Contains(content, "const API = 'http://localhost:8088';") {
		t.Fatalf("expected hardcoded API constant to be removed")
	}
}

func TestMindDrillUINeverEmbedsServerSecrets(t *testing.T) {
	testServer := httptest.NewServer(newHandler("http://localhost:8088"))
	defer testServer.Close()

	resp, err := http.Get(testServer.URL + "/")
	if err != nil {
		t.Fatalf("root request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed reading root response body: %v", err)
	}
	content := string(body)

	if strings.Contains(content, "FRONTPOCKET_API_KEY") {
		t.Fatal("expected UI to avoid exposing FRONTPOCKET_API_KEY")
	}
	if strings.Contains(content, "OPENAI_API_KEY") {
		t.Fatal("expected UI to avoid exposing OPENAI_API_KEY")
	}
	if strings.Contains(content, "OPENROUTER_API_KEY") {
		t.Fatal("expected UI to avoid exposing OPENROUTER_API_KEY")
	}
}

func TestMindDrillUIUsesNonDebugSessionDeleteRoute(t *testing.T) {
	testServer := httptest.NewServer(newHandler("http://localhost:8088"))
	defer testServer.Close()

	resp, err := http.Get(testServer.URL + "/")
	if err != nil {
		t.Fatalf("root request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed reading root response body: %v", err)
	}
	content := string(body)

	if !strings.Contains(content, "/memory/chat/session") {
		t.Fatal("expected UI to use the non-debug chat session delete route")
	}
	if strings.Contains(content, "/minddrill/memory/session") {
		t.Fatal("expected UI to avoid debug-only MindDrill session route")
	}
	if !strings.Contains(content, "fetchJSON") {
		t.Fatal("expected UI to use shared JSON fetch helper")
	}
}
