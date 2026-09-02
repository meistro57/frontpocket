package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meistro57/frontpocket/internal/store"
)

type researchTestEmbedder struct{}

func (researchTestEmbedder) EmbedText(_ context.Context, _ string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3, 0.4}, nil
}

func (researchTestEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for range texts {
		out = append(out, []float32{0.1, 0.2, 0.3, 0.4})
	}
	return out, nil
}

func (researchTestEmbedder) ProviderName() string { return "test" }
func (researchTestEmbedder) ModelName() string    { return "google-embeddings-2" }

func TestInspectCollectionBuildsCorpusReport(t *testing.T) {
	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/corpus":
			_, _ = io.WriteString(w, `{"result":{"points_count":4,"config":{"params":{"vectors":{"size":4,"distance":"Cosine"}}}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/collections/corpus/points/scroll":
			_, _ = io.WriteString(w, `{"result":{"points":[
				{"payload":{"memory_id":"m1","source_document_id":"doc-A","source_title":"Doc Alpha","path":"/corpus/doc-alpha.md","filename":"doc-alpha.md","chunk_id":"c1","chunk_index":1,"created_at":"2024-01-02T00:00:00Z","embedding_provider":"google","embedding_model":"text-embedding-004","text":"alpha one"}},
				{"payload":{"memory_id":"m2","source_document_id":"doc-A","source_title":"Doc Alpha","path":"/corpus/doc-alpha.md","filename":"doc-alpha.md","chunk_id":"c2","chunk_index":2,"created_at":"2024-01-03T00:00:00Z","text":"alpha two"}},
				{"payload":{"memory_id":"m3","source_document_id":"doc-B","source_title":"Doc Alpha","path":"/corpus/doc-alpha.md","filename":"doc-alpha-copy.md","chunk_id":"c9","chunk_index":1,"created_at":"2024-01-04T00:00:00Z","text":"duplicate seed"}},
				{"payload":{"memory_id":"m4","source_document_id":"doc-C","source_title":"Doc Beta","path":"/corpus/doc-beta.md","filename":"doc-beta.md","chunk_id":"c3","chunk_index":1,"created_at":"2024-01-05T00:00:00Z","text":"beta one"}}
			],"next_page_offset":null}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer qdrant.Close()

	runtime := NewMindDrillResearchRuntime(store.NewQdrantClient(qdrant.URL), researchTestEmbedder{}, "", nil)
	report, err := InspectCollection(context.Background(), runtime, "corpus", 200)
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	if !report.CollectionExists {
		t.Fatal("expected collection to exist")
	}
	if report.VectorDimensions != 4 {
		t.Fatalf("expected vector size 4, got %d", report.VectorDimensions)
	}
	if report.ChunksOrVectors != 4 {
		t.Fatalf("expected chunk count 4, got %d", report.ChunksOrVectors)
	}
	if report.DocumentsDetected != 3 {
		t.Fatalf("expected 3 detected documents, got %d", report.DocumentsDetected)
	}
	if len(report.LargestDocuments) == 0 || report.LargestDocuments[0].DocumentID != "doc-A" || report.LargestDocuments[0].Chunks != 2 {
		t.Fatalf("unexpected largest document summary: %#v", report.LargestDocuments)
	}
	if len(report.DuplicateSourceDocuments) == 0 {
		t.Fatalf("expected duplicate source detection, got %#v", report.DuplicateSourceDocuments)
	}
	if report.EmbeddingModel != "text-embedding-004" {
		t.Fatalf("expected embedding model from payload, got %q", report.EmbeddingModel)
	}
}

func TestResearchToolsRequireBindingAndTrackLedger(t *testing.T) {
	sessionPath := filepath.Join(t.TempDir(), "research_sessions.json")
	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/corpus":
			_, _ = io.WriteString(w, `{"result":{"points_count":3,"config":{"params":{"vectors":{"size":4,"distance":"Cosine"}}}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/collections/corpus/points/search":
			_, _ = io.WriteString(w, `{"result":[
				{"score":0.95,"payload":{"memory_id":"m1","source_document_id":"doc-A","source_title":"Doc A","chunk_id":"a1","chunk_index":1,"text":"a1 text"}},
				{"score":0.92,"payload":{"memory_id":"m2","source_document_id":"doc-A","source_title":"Doc A","chunk_id":"a2","chunk_index":2,"text":"a2 text"}},
				{"score":0.91,"payload":{"memory_id":"m3","source_document_id":"doc-B","source_title":"Doc B","chunk_id":"b1","chunk_index":1,"text":"b1 text"}}
			]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/collections/corpus/points/scroll":
			_, _ = io.WriteString(w, `{"result":{"points":[
				{"payload":{"memory_id":"m1","source_document_id":"doc-A","source_title":"Doc A","chunk_id":"a1","chunk_index":1,"text":"first idea"}},
				{"payload":{"memory_id":"m2","source_document_id":"doc-A","source_title":"Doc A","chunk_id":"a2","chunk_index":2,"text":"second idea"}},
				{"payload":{"memory_id":"m3","source_document_id":"doc-A","source_title":"Doc A","chunk_id":"a3","chunk_index":3,"text":"third idea"}}
			],"next_page_offset":null}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer qdrant.Close()

	t.Setenv("MINDDRILL_RESEARCH_SESSION_FILE", sessionPath)
	runtime := NewMindDrillResearchRuntime(store.NewQdrantClient(qdrant.URL), researchTestEmbedder{}, "", nil)
	toolsList := NewMindDrillResearchTools(runtime)
	byName := make(map[string]*researchTool, len(toolsList))
	for _, tool := range toolsList {
		byName[tool.Definition().Name] = tool
	}

	semanticResp, err := byName["semantic_search"].Execute(context.Background(), `{"session_id":"s-unbound","query":"idea"}`)
	if err != nil {
		t.Fatalf("semantic tool returned error: %v", err)
	}
	if !strings.Contains(semanticResp, "not bound to a collection") {
		t.Fatalf("expected explicit binding error, got %s", semanticResp)
	}

	_, err = byName["bind_research_collection"].Execute(context.Background(), `{"session_id":"s1","collection":"corpus","research_question":"trace lineage"}`)
	if err != nil {
		t.Fatalf("bind tool returned error: %v", err)
	}

	semanticResp, err = byName["semantic_search"].Execute(context.Background(), `{"session_id":"s1","query":"idea","limit":3,"max_chunks_per_document":1,"reason":"find diverse evidence"}`)
	if err != nil {
		t.Fatalf("semantic tool returned error: %v", err)
	}
	var semanticBody map[string]any
	if err := json.Unmarshal([]byte(semanticResp), &semanticBody); err != nil {
		t.Fatalf("semantic response is not JSON: %v\n%s", err, semanticResp)
	}
	results, _ := semanticBody["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("expected document diversity cap to return 2 results, got %d", len(results))
	}
	sessionRaw, _ := semanticBody["session"].(map[string]any)
	searches, _ := sessionRaw["searches_performed"].([]any)
	if len(searches) == 0 {
		t.Fatalf("expected search ledger entries, got %#v", sessionRaw)
	}

	neighborResp, err := byName["get_neighbor_chunks"].Execute(context.Background(), `{"session_id":"s1","chunk_id":"a2","window":1}`)
	if err != nil {
		t.Fatalf("neighbor tool returned error: %v", err)
	}
	var neighborBody map[string]any
	if err := json.Unmarshal([]byte(neighborResp), &neighborBody); err != nil {
		t.Fatalf("neighbor response is not JSON: %v", err)
	}
	neighbors, _ := neighborBody["results"].([]any)
	if len(neighbors) != 3 {
		t.Fatalf("expected surrounding chunks for anchor, got %d", len(neighbors))
	}
}
