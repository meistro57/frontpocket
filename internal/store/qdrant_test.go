package store

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
)

func TestParseVectorSizeUnnamed(t *testing.T) {
	raw := json.RawMessage(`{"size":768,"distance":"Cosine"}`)
	if got := parseVectorSize(raw, ""); got != 768 {
		t.Fatalf("expected size 768, got %d", got)
	}
}

func TestParseVectorSizeNamed(t *testing.T) {
	raw := json.RawMessage(`{"memory":{"size":1536,"distance":"Cosine"}}`)
	if got := parseVectorSize(raw, "memory"); got != 1536 {
		t.Fatalf("expected size 1536, got %d", got)
	}
}

func TestParseVectorSizeMissingNamedEntry(t *testing.T) {
	raw := json.RawMessage(`{"memory":{"size":1536,"distance":"Cosine"}}`)
	if got := parseVectorSize(raw, "other"); got != 0 {
		t.Fatalf("expected size 0 for missing vector name, got %d", got)
	}
}

func TestQdrantPointIDConvertsMemoryIDToUUID(t *testing.T) {
	memoryID := "chat_20260624_turn_001_chunk_001"
	pointID := qdrantPointID(memoryID)
	if !isUUID(pointID) {
		t.Fatalf("expected UUID point ID, got %q", pointID)
	}
	if pointID == memoryID {
		t.Fatalf("expected transformed UUID ID, got original memory ID %q", pointID)
	}
	if again := qdrantPointID(memoryID); again != pointID {
		t.Fatalf("expected deterministic UUID generation, got %q and %q", pointID, again)
	}
}

func TestUpsertUsesQdrantCompatiblePointID(t *testing.T) {
	var capturedID string
	var capturedMemoryID string

	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/frontpocket_test":
			_, _ = io.WriteString(w, `{"result":{"config":{"params":{"vectors":{"size":4,"distance":"Cosine"}}}}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/collections/frontpocket_test/points":
			var req struct {
				Points []struct {
					ID      string         `json:"id"`
					Payload map[string]any `json:"payload"`
				} `json:"points"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("failed decoding qdrant upsert payload: %v", err)
			}
			if len(req.Points) != 1 {
				t.Fatalf("expected one point, got %d", len(req.Points))
			}
			capturedID = req.Points[0].ID
			capturedMemoryID = req.Points[0].Payload["memory_id"].(string)
			_, _ = io.WriteString(w, `{"status":"ok","result":{"operation_id":1}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer qdrant.Close()

	memStore := NewQdrantMemoryStore(NewQdrantClient(qdrant.URL), nil, "frontpocket_test", "", "Cosine", nil)
	memoryID := "chat_20260624_turn_001_chunk_001"
	err := memStore.Upsert(context.Background(), []memory.MemoryPoint{{
		MemoryID:            memoryID,
		ConversationID:      "conv_1",
		SourceType:          "chat_export",
		SourceTitle:         "Test",
		Timestamp:           time.Now().UTC(),
		Speaker:             "user",
		MemoryKind:          memory.KindProjectContext,
		Text:                "hello",
		SourceQuote:         "hello",
		Summary:             "hello",
		EmbeddingProvider:   "openrouter",
		EmbeddingModel:      "openai/text-embedding-3-small",
		EmbeddingDimensions: 4,
		Vector:              []float32{0.1, 0.2, 0.3, 0.4},
	}})
	if err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	if !isUUID(capturedID) {
		t.Fatalf("expected qdrant point ID to be UUID, got %q", capturedID)
	}
	if capturedMemoryID != memoryID {
		t.Fatalf("expected payload memory_id %q, got %q", memoryID, capturedMemoryID)
	}
}
