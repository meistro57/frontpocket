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

func TestMindDrillInspectCommandRequiresCollection(t *testing.T) {
	err := run([]string{"inspect"})
	if err == nil || !strings.Contains(err.Error(), "exactly one collection") {
		t.Fatalf("expected inspect argument validation error, got %v", err)
	}
}

func TestMindDrillInspectListCommand(t *testing.T) {
	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/collections" {
			_, _ = io.WriteString(w, `{"result":{"collections":[{"name":"alpha"},{"name":"beta"}]}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer qdrant.Close()

	t.Setenv("QDRANT_URL", qdrant.URL)
	if err := run([]string{"inspect", "--list"}); err != nil {
		t.Fatalf("inspect --list failed: %v", err)
	}
}

func TestMindDrillInspectCommand(t *testing.T) {
	embeddingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"embeddings":[[0.1,0.2,0.3,0.4]]}`)
	}))
	defer embeddingServer.Close()

	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/research_collection":
			_, _ = io.WriteString(w, `{"result":{"points_count":2,"config":{"params":{"vectors":{"size":4,"distance":"Cosine"}}}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/collections/research_collection/points/scroll":
			_, _ = io.WriteString(w, `{"result":{"points":[{"payload":{"memory_id":"m1","source_document_id":"doc-a","source_title":"Doc A","chunk_id":"c1","chunk_index":1,"text":"alpha","embedding_model":"text-embedding-004"}}],"next_page_offset":null}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer qdrant.Close()

	t.Setenv("EMBEDDING_PROVIDER", "ollama")
	t.Setenv("OLLAMA_BASE_URL", embeddingServer.URL)
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "nomic-embed-text")
	t.Setenv("QDRANT_URL", qdrant.URL)

	if err := run([]string{"inspect", "--sample-limit", "50", "research_collection"}); err != nil {
		t.Fatalf("inspect command failed: %v", err)
	}
}

func TestMindDrillUIRendersAssistantMarkdown(t *testing.T) {
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

	if !strings.Contains(content, "function markdownToHTML(text)") {
		t.Fatal("expected UI to include markdown parser for assistant responses")
	}
	if !strings.Contains(content, "body.innerHTML = markdownToHTML(text)") {
		t.Fatal("expected assistant turns to render markdown HTML")
	}
	if !strings.Contains(content, "chat-turn-body") {
		t.Fatal("expected chat turn body element for markdown rendering")
	}
}

func TestMindDrillDeepDrillPlanCommand(t *testing.T) {
	embeddingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"embeddings":[[0.1,0.2,0.3,0.4]]}`)
	}))
	defer embeddingServer.Close()

	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/corpus":
			_, _ = io.WriteString(w, `{"result":{"points_count":2,"config":{"params":{"vectors":{"size":4,"distance":"Cosine"}}}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer qdrant.Close()

	t.Setenv("EMBEDDING_PROVIDER", "ollama")
	t.Setenv("OLLAMA_BASE_URL", embeddingServer.URL)
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "nomic-embed-text")
	t.Setenv("QDRANT_URL", qdrant.URL)

	err := run([]string{"deepdrill", "plan", "--session", "centerstone", "--collection", "corpus", "--research-question", "trace earliest bridge", "--blockers", "PROVENANCE_GAP,CHRONOLOGY_GAP"})
	if err != nil {
		t.Fatalf("deepdrill plan command failed: %v", err)
	}
}

func TestMindDrillDeepDrillRunCommand(t *testing.T) {
	embeddingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"embeddings":[[0.1,0.2,0.3,0.4]]}`)
	}))
	defer embeddingServer.Close()

	thoughtCollectionReady := false
	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/corpus":
			_, _ = io.WriteString(w, `{"result":{"points_count":2,"config":{"params":{"vectors":{"size":4,"distance":"Cosine"}}}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/collections/corpus/points/scroll":
			_, _ = io.WriteString(w, `{"result":{"points":[],"next_page_offset":null}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/collections/corpus/points/search":
			_, _ = io.WriteString(w, `{"result":[]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/collections/minddrill_research_thoughts":
			if !thoughtCollectionReady {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"status":"error"}`)
				return
			}
			_, _ = io.WriteString(w, `{"result":{"points_count":1,"config":{"params":{"vectors":{"size":4,"distance":"Cosine"}}}}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/collections/minddrill_research_thoughts":
			thoughtCollectionReady = true
			_, _ = io.WriteString(w, `{"status":"ok"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/collections/minddrill_research_thoughts/points/count":
			_, _ = io.WriteString(w, `{"result":{"count":0}}`)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/collections/minddrill_research_thoughts/points"):
			_, _ = io.WriteString(w, `{"status":"ok"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer qdrant.Close()

	t.Setenv("EMBEDDING_PROVIDER", "ollama")
	t.Setenv("OLLAMA_BASE_URL", embeddingServer.URL)
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "nomic-embed-text")
	t.Setenv("QDRANT_URL", qdrant.URL)

	err := run([]string{"deepdrill", "run", "--session", "centerstone", "--collection", "corpus", "--steps", "1", "--blockers", "PROVENANCE_GAP"})
	if err != nil {
		t.Fatalf("deepdrill run command failed: %v", err)
	}
}

func TestMindDrillDeepDrillThoughtsCommand(t *testing.T) {
	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/collections/minddrill_research_thoughts/points/scroll":
			_, _ = io.WriteString(w, `{"result":{"points":[{"payload":{"thought_id":"t1","memory_id":"t1","session_id":"s1","timestamp":"2026-01-01T00:00:00Z","type":"QUESTION","status":"open","source_type":"deepdrill_thought","evidence_origin":"RESEARCH_THOUGHTS"}},{"payload":{"thought_id":"t2","memory_id":"t2","session_id":"s1","timestamp":"2026-01-02T00:00:00Z","type":"STOP_DECISION","status":"frozen","source_type":"deepdrill_thought","evidence_origin":"SOURCE_CORPUS"}}],"next_page_offset":null}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer qdrant.Close()

	t.Setenv("EMBEDDING_PROVIDER", "ollama")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "nomic-embed-text")
	t.Setenv("QDRANT_URL", qdrant.URL)

	err := run([]string{"deepdrill", "thoughts", "--session", "s1", "--summary"})
	if err != nil {
		t.Fatalf("deepdrill thoughts command failed: %v", err)
	}
}

func TestMindDrillDeepDrillShowCommand(t *testing.T) {
	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/collections/minddrill_research_thoughts/points/scroll":
			_, _ = io.WriteString(w, `{"result":{"points":[{"payload":{"thought_id":"target-thought","memory_id":"target-thought","session_id":"s1","timestamp":"2026-01-01T00:00:00Z","type":"EVIDENCE","status":"rediscovery","duplicate_of_thought":"base-thought","source_type":"deepdrill_thought","evidence_origin":"SOURCE_CORPUS","sources":["doc-A"]}}],"next_page_offset":null}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer qdrant.Close()

	t.Setenv("EMBEDDING_PROVIDER", "ollama")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "nomic-embed-text")
	t.Setenv("QDRANT_URL", qdrant.URL)

	err := run([]string{"deepdrill", "show", "target-thought"})
	if err != nil {
		t.Fatalf("deepdrill show command failed: %v", err)
	}
}

func TestMindDrillDeepDrillProvenanceCommandByThoughtID(t *testing.T) {
	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/collections/minddrill_research_thoughts/points/scroll":
			_, _ = io.WriteString(w, `{"result":{"points":[{"payload":{"thought_id":"t-provenance","memory_id":"t-provenance","session_id":"s1","timestamp":"2026-01-01T00:00:00Z","type":"EVIDENCE","source_type":"deepdrill_thought","evidence_origin":"SOURCE_CORPUS","sources":["doc-A"],"query_spec":{"collection":"corpus"}}}],"next_page_offset":null}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/collections/corpus/points/scroll":
			_, _ = io.WriteString(w, `{"result":{"points":[{"payload":{"memory_id":"doc-A","source_document_id":"doc-A","source_title":"Doc A","source_type":"document","timestamp":"2026-01-01T00:00:00Z","derived_from":["raw-A"]}},{"payload":{"memory_id":"summary-A","source_document_id":"summary-A","source_title":"Doc A","source_type":"audio_overview","timestamp":"2026-01-02T00:00:00Z","cited_source_ids":["doc-A"]}}],"next_page_offset":null}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer qdrant.Close()

	t.Setenv("EMBEDDING_PROVIDER", "ollama")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "nomic-embed-text")
	t.Setenv("QDRANT_URL", qdrant.URL)

	err := run([]string{"deepdrill", "provenance", "t-provenance", "--collection", "corpus"})
	if err != nil {
		t.Fatalf("deepdrill provenance command by thought_id failed: %v", err)
	}
}

func TestMindDrillDeepDrillProvenanceCommandBySource(t *testing.T) {
	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/collections/corpus/points/scroll":
			_, _ = io.WriteString(w, `{"result":{"points":[{"payload":{"memory_id":"doc-A","source_document_id":"doc-A","source_title":"Doc A","source_type":"document","timestamp":"2026-01-01T00:00:00Z"}},{"payload":{"memory_id":"doc-B","source_document_id":"doc-B","source_title":"Doc A","source_type":"audio_overview","timestamp":"2026-01-02T00:00:00Z","derived_from":["doc-A"]}}],"next_page_offset":null}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer qdrant.Close()

	t.Setenv("EMBEDDING_PROVIDER", "ollama")
	t.Setenv("OLLAMA_EMBEDDING_MODEL", "nomic-embed-text")
	t.Setenv("QDRANT_URL", qdrant.URL)

	err := run([]string{"deepdrill", "provenance", "--collection", "corpus", "--source", "doc-A", "--json"})
	if err != nil {
		t.Fatalf("deepdrill provenance command by source failed: %v", err)
	}
}
