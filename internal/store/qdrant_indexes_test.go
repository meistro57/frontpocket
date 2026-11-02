package store

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/meistro57/frontpocket/internal/memory"
)

func TestEnsurePayloadIndexesUsesMemoryFieldSet(t *testing.T) {
	var mu sync.Mutex
	var created []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/collections/test_collection/index") {
			var body struct {
				FieldName   string `json:"field_name"`
				FieldSchema string `json:"field_schema"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("failed to decode index body: %v", err)
			}
			if body.FieldSchema == "" {
				t.Errorf("expected a field schema for %q", body.FieldName)
			}
			mu.Lock()
			created = append(created, body.FieldName)
			mu.Unlock()
			_, _ = io.WriteString(w, `{"status":"ok","result":{"operation_id":1}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := NewQdrantClient(srv.URL)
	if err := client.EnsurePayloadIndexes(context.Background(), "test_collection", MemoryPayloadIndexes()); err != nil {
		t.Fatalf("ensure payload indexes failed: %v", err)
	}

	expected := []string{
		"project", "memory_kind", "speaker", "source_type", "conversation_id",
		"ai_provider", "ai_model", "feedback_rating", "attachment_source_system",
		"tags", "status", "user_starred", "user_shared", "canonical", "timestamp",
	}
	if len(created) != len(expected) {
		t.Fatalf("expected %d indexes, got %d (%v)", len(expected), len(created), created)
	}
	for i, field := range expected {
		if created[i] != field {
			t.Fatalf("expected index %d to be %q, got %q", i, field, created[i])
		}
	}
}

func TestUpsertCreatesPayloadIndexesOncePerCollection(t *testing.T) {
	var mu sync.Mutex
	indexCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/frontpocket_test":
			_, _ = io.WriteString(w, `{"result":{"config":{"params":{"vectors":{"size":4,"distance":"Cosine"}}}}}`)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/index"):
			mu.Lock()
			indexCalls++
			mu.Unlock()
			_, _ = io.WriteString(w, `{"status":"ok"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/collections/frontpocket_test/points":
			_, _ = io.WriteString(w, `{"status":"ok","result":{"operation_id":1}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	memStore := NewQdrantMemoryStore(NewQdrantClient(srv.URL), nil, "frontpocket_test", "", "Cosine", nil)
	point := testMemoryPoint("mem_index_1")

	if err := memStore.Upsert(context.Background(), []memory.MemoryPoint{point}); err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}
	mu.Lock()
	afterFirst := indexCalls
	mu.Unlock()

	if err := memStore.Upsert(context.Background(), []memory.MemoryPoint{point}); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}
	mu.Lock()
	afterSecond := indexCalls
	mu.Unlock()

	if afterFirst != len(memoryPayloadIndexes) {
		t.Fatalf("expected %d index calls after first upsert, got %d", len(memoryPayloadIndexes), afterFirst)
	}
	if afterSecond != afterFirst {
		t.Fatalf("expected no extra index calls on second upsert, got %d then %d", afterFirst, afterSecond)
	}
}
