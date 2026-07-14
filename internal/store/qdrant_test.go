package store

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestDoJSONUsesDefaultTimeoutWhenContextHasNoDeadline(t *testing.T) {
	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(40 * time.Millisecond)
		_, _ = io.WriteString(w, `{"result":[]}`)
	}))
	defer qdrant.Close()

	client := NewQdrantClient(qdrant.URL)
	client.requestTimeout = 10 * time.Millisecond

	_, err := client.doJSON(context.Background(), http.MethodGet, "/collections", nil, nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error message, got %v", err)
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

func TestUpsertEmbedsPointsWithoutVector(t *testing.T) {
	var embedInput string
	var capturedVector []float64

	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/minddrill_chat_memory":
			_, _ = io.WriteString(w, `{"result":{"config":{"params":{"vectors":{"size":4,"distance":"Cosine"}}}}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/collections/minddrill_chat_memory/points":
			var req struct {
				Points []struct {
					Vector []float64 `json:"vector"`
				} `json:"points"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("failed decoding qdrant upsert payload: %v", err)
			}
			if len(req.Points) != 1 {
				t.Fatalf("expected one point, got %d", len(req.Points))
			}
			capturedVector = req.Points[0].Vector
			_, _ = io.WriteString(w, `{"status":"ok","result":{"operation_id":1}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer qdrant.Close()

	embedder := &testEmbedder{embedText: func(_ context.Context, text string) ([]float32, error) {
		embedInput = text
		return []float32{0.11, 0.22, 0.33, 0.44}, nil
	}}

	memStore := NewQdrantMemoryStore(NewQdrantClient(qdrant.URL), embedder, "minddrill_chat_memory", "", "Cosine", nil)
	err := memStore.Upsert(context.Background(), []memory.MemoryPoint{{
		MemoryID:       "minddrill_session_1_0001",
		ConversationID: "session_1",
		SourceType:     "minddrill_chat",
		SourceTitle:    "MindDrill Chat",
		Timestamp:      time.Now().UTC(),
		Speaker:        "user",
		MemoryKind:     memory.KindChatTurn,
		Text:           "remember this detail",
		SourceQuote:    "remember this detail",
		Summary:        "remember this detail",
	}})
	if err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	if embedInput != "remember this detail" {
		t.Fatalf("expected embed input to come from point text, got %q", embedInput)
	}
	if len(capturedVector) != 4 {
		t.Fatalf("expected embedded vector in qdrant payload, got %#v", capturedVector)
	}
}

func TestToQdrantPayloadIncludesMindDrillMetadata(t *testing.T) {
	now := time.Now().UTC()
	payload := toQdrantPayload(memory.MemoryPoint{
		MemoryID:            "minddrill_session_1_0001",
		ConversationID:      "session_1",
		SessionID:           "session_1",
		SourceType:          "minddrill_chat",
		SourceTitle:         "MindDrill Chat",
		Timestamp:           now,
		Speaker:             "user",
		UserStarred:         true,
		UserShared:          true,
		ShareID:             "share-123",
		FeedbackRating:      "thumbs_down",
		FeedbackNote:        "totally lost personality",
		FeedbackAt:          "2025-12-12T03:14:43Z",
		MemoryKind:          memory.KindUserPreference,
		Summary:             "Keep responses concise.",
		Text:                "remember this: keep responses concise",
		SourceQuote:         "remember this: keep responses concise",
		UsedMemoryIDs:       []string{"front_1", "mind_2"},
		EmbeddingProvider:   "ollama",
		EmbeddingModel:      "nomic-embed-text",
		EmbeddingDimensions: 4,
	})

	if payload["session_id"] != "session_1" {
		t.Fatalf("expected payload session_id session_1, got %v", payload["session_id"])
	}
	ids, ok := payload["used_memory_ids"].([]string)
	if !ok {
		t.Fatalf("expected used_memory_ids []string, got %#v", payload["used_memory_ids"])
	}
	if len(ids) != 2 || ids[0] != "front_1" || ids[1] != "mind_2" {
		t.Fatalf("unexpected used_memory_ids: %#v", ids)
	}
	if payload["user_starred"] != true {
		t.Fatalf("expected payload user_starred true, got %v", payload["user_starred"])
	}
	if payload["user_shared"] != true {
		t.Fatalf("expected payload user_shared true, got %v", payload["user_shared"])
	}
	if payload["share_id"] != "share-123" {
		t.Fatalf("expected payload share_id share-123, got %v", payload["share_id"])
	}
	if payload["feedback_rating"] != "thumbs_down" {
		t.Fatalf("expected payload feedback_rating thumbs_down, got %v", payload["feedback_rating"])
	}
	if payload["feedback_note"] != "totally lost personality" {
		t.Fatalf("expected payload feedback_note value, got %v", payload["feedback_note"])
	}
	if payload["feedback_at"] != "2025-12-12T03:14:43Z" {
		t.Fatalf("expected payload feedback_at value, got %v", payload["feedback_at"])
	}
}

func TestUpsertRetriesTransientConnectionError(t *testing.T) {
	originalDelays := qdrantWriteRetryDelays
	qdrantWriteRetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	defer func() { qdrantWriteRetryDelays = originalDelays }()

	attempts := 0
	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/frontpocket_test":
			_, _ = io.WriteString(w, `{"result":{"config":{"params":{"vectors":{"size":4,"distance":"Cosine"}}}}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/collections/frontpocket_test/points":
			attempts++
			if attempts == 1 {
				hijacker, ok := w.(http.Hijacker)
				if !ok {
					t.Fatal("response writer does not support hijacking")
				}
				conn, _, err := hijacker.Hijack()
				if err != nil {
					t.Fatalf("hijack failed: %v", err)
				}
				_ = conn.Close()
				return
			}
			_, _ = io.WriteString(w, `{"status":"ok","result":{"operation_id":1}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer qdrant.Close()

	memStore := NewQdrantMemoryStore(NewQdrantClient(qdrant.URL), nil, "frontpocket_test", "", "Cosine", nil)
	err := memStore.Upsert(context.Background(), []memory.MemoryPoint{{
		MemoryID:            "retry_test_memory",
		ConversationID:      "conv_1",
		SourceType:          "chat_export",
		SourceTitle:         "Retry Test",
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
		t.Fatalf("expected retry to recover transient error, got %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 upsert attempts, got %d", attempts)
	}
}

func TestUpsertFailsAfterTransientRetriesExhausted(t *testing.T) {
	originalDelays := qdrantWriteRetryDelays
	qdrantWriteRetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	defer func() { qdrantWriteRetryDelays = originalDelays }()

	attempts := 0
	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/frontpocket_test":
			_, _ = io.WriteString(w, `{"result":{"config":{"params":{"vectors":{"size":4,"distance":"Cosine"}}}}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/collections/frontpocket_test/points":
			attempts++
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("response writer does not support hijacking")
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatalf("hijack failed: %v", err)
			}
			_ = conn.Close()
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer qdrant.Close()

	memStore := NewQdrantMemoryStore(NewQdrantClient(qdrant.URL), nil, "frontpocket_test", "", "Cosine", nil)
	err := memStore.Upsert(context.Background(), []memory.MemoryPoint{{
		MemoryID:            "retry_failure_test_memory",
		ConversationID:      "conv_1",
		SourceType:          "chat_export",
		SourceTitle:         "Retry Failure Test",
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
	if err == nil {
		t.Fatal("expected upsert error after retries were exhausted")
	}
	if attempts != 3 {
		t.Fatalf("expected 3 upsert attempts, got %d", attempts)
	}
}

func TestUpsertDoesNotRetryStatusErrors(t *testing.T) {
	originalDelays := qdrantWriteRetryDelays
	qdrantWriteRetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	defer func() { qdrantWriteRetryDelays = originalDelays }()

	attempts := 0
	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/frontpocket_test":
			_, _ = io.WriteString(w, `{"result":{"config":{"params":{"vectors":{"size":4,"distance":"Cosine"}}}}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/collections/frontpocket_test/points":
			attempts++
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"status":"error","result":null}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer qdrant.Close()

	memStore := NewQdrantMemoryStore(NewQdrantClient(qdrant.URL), nil, "frontpocket_test", "", "Cosine", nil)
	err := memStore.Upsert(context.Background(), []memory.MemoryPoint{{
		MemoryID:            "status_failure_test_memory",
		ConversationID:      "conv_1",
		SourceType:          "chat_export",
		SourceTitle:         "Status Failure Test",
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
	if err == nil {
		t.Fatal("expected upsert error on qdrant status failure")
	}
	if attempts != 1 {
		t.Fatalf("expected no retries for status error, got %d attempts", attempts)
	}
}

func TestUpsertSplitsLargePointCountIntoMultipleRequests(t *testing.T) {
	requests := 0
	totalPoints := 0
	maxPointsInRequest := 0

	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/frontpocket_test":
			_, _ = io.WriteString(w, `{"result":{"config":{"params":{"vectors":{"size":4,"distance":"Cosine"}}}}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/collections/frontpocket_test/points":
			requests++
			var req struct {
				Points []struct{} `json:"points"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("failed decoding qdrant upsert payload: %v", err)
			}
			if len(req.Points) > qdrantMaxUpsertPoints {
				t.Fatalf("request exceeded point cap: got %d > %d", len(req.Points), qdrantMaxUpsertPoints)
			}
			totalPoints += len(req.Points)
			if len(req.Points) > maxPointsInRequest {
				maxPointsInRequest = len(req.Points)
			}
			_, _ = io.WriteString(w, `{"status":"ok","result":{"operation_id":1}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer qdrant.Close()

	points := make([]memory.MemoryPoint, 0, 300)
	for idx := 0; idx < 300; idx++ {
		points = append(points, memory.MemoryPoint{
			MemoryID:            fmt.Sprintf("count_split_%03d", idx),
			ConversationID:      "conv_1",
			SourceType:          "chat_export",
			SourceTitle:         "Count Split Test",
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
		})
	}

	memStore := NewQdrantMemoryStore(NewQdrantClient(qdrant.URL), nil, "frontpocket_test", "", "Cosine", nil)
	if err := memStore.Upsert(context.Background(), points); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	if requests < 3 {
		t.Fatalf("expected multiple requests for 300 points, got %d", requests)
	}
	if totalPoints != 300 {
		t.Fatalf("expected 300 points upserted, got %d", totalPoints)
	}
	if maxPointsInRequest > qdrantMaxUpsertPoints {
		t.Fatalf("max points per request exceeded cap: %d", maxPointsInRequest)
	}
}

func TestUpsertSplitsByPayloadBytes(t *testing.T) {
	requests := 0
	requestSizes := make([]int, 0, 3)
	pointsPerRequest := make([]int, 0, 3)

	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/frontpocket_test":
			_, _ = io.WriteString(w, `{"result":{"config":{"params":{"vectors":{"size":4,"distance":"Cosine"}}}}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/collections/frontpocket_test/points":
			requests++
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("failed reading body: %v", err)
			}
			requestSizes = append(requestSizes, len(body))
			var req struct {
				Points []struct{} `json:"points"`
			}
			if err := json.Unmarshal(body, &req); err != nil {
				t.Fatalf("failed decoding payload: %v", err)
			}
			pointsPerRequest = append(pointsPerRequest, len(req.Points))
			_, _ = io.WriteString(w, `{"status":"ok","result":{"operation_id":1}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer qdrant.Close()

	largeText := strings.Repeat("x", 11*1024*1024)
	points := []memory.MemoryPoint{
		{
			MemoryID:            "bytes_split_1",
			ConversationID:      "conv_1",
			SourceType:          "chat_export",
			SourceTitle:         "Bytes Split Test",
			Timestamp:           time.Now().UTC(),
			Speaker:             "user",
			MemoryKind:          memory.KindProjectContext,
			Text:                largeText,
			SourceQuote:         "quote",
			Summary:             "summary",
			EmbeddingProvider:   "openrouter",
			EmbeddingModel:      "openai/text-embedding-3-small",
			EmbeddingDimensions: 4,
			Vector:              []float32{0.1, 0.2, 0.3, 0.4},
		},
		{
			MemoryID:            "bytes_split_2",
			ConversationID:      "conv_1",
			SourceType:          "chat_export",
			SourceTitle:         "Bytes Split Test",
			Timestamp:           time.Now().UTC(),
			Speaker:             "assistant",
			MemoryKind:          memory.KindProjectContext,
			Text:                largeText,
			SourceQuote:         "quote",
			Summary:             "summary",
			EmbeddingProvider:   "openrouter",
			EmbeddingModel:      "openai/text-embedding-3-small",
			EmbeddingDimensions: 4,
			Vector:              []float32{0.1, 0.2, 0.3, 0.4},
		},
		{
			MemoryID:            "bytes_split_3",
			ConversationID:      "conv_1",
			SourceType:          "chat_export",
			SourceTitle:         "Bytes Split Test",
			Timestamp:           time.Now().UTC(),
			Speaker:             "user",
			MemoryKind:          memory.KindProjectContext,
			Text:                largeText,
			SourceQuote:         "quote",
			Summary:             "summary",
			EmbeddingProvider:   "openrouter",
			EmbeddingModel:      "openai/text-embedding-3-small",
			EmbeddingDimensions: 4,
			Vector:              []float32{0.1, 0.2, 0.3, 0.4},
		},
	}

	memStore := NewQdrantMemoryStore(NewQdrantClient(qdrant.URL), nil, "frontpocket_test", "", "Cosine", nil)
	if err := memStore.Upsert(context.Background(), points); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	if requests < 2 {
		t.Fatalf("expected payload byte split into multiple requests, got %d", requests)
	}
	for idx, size := range requestSizes {
		if size > qdrantMaxUpsertPayloadBytes {
			t.Fatalf("request %d payload size %d exceeds cap %d", idx, size, qdrantMaxUpsertPayloadBytes)
		}
	}
	if len(pointsPerRequest) > 0 && pointsPerRequest[0] == len(points) {
		t.Fatalf("expected first request to be split by bytes, got points per request=%v", pointsPerRequest)
	}
}

func TestToQdrantFilterIncludesNewMetadataFilters(t *testing.T) {
	starred := true
	shared := false
	hasAttachment := true

	filter := toQdrantFilter(memory.SearchFilters{
		AIProvider:     "claude",
		AIModel:        "claude-3-5-sonnet",
		UserStarred:    &starred,
		UserShared:     &shared,
		FeedbackRating: "thumbs_down",
		HasAttachment:  &hasAttachment,
	})
	if filter == nil {
		t.Fatal("expected filter")
	}

	must, _ := filter["must"].([]map[string]any)
	mustNot, _ := filter["must_not"].([]map[string]any)
	if len(must) != 5 {
		t.Fatalf("expected 5 must clauses, got %d", len(must))
	}
	if len(mustNot) != 1 {
		t.Fatalf("expected 1 must_not clause, got %d", len(mustNot))
	}
}

func TestToQdrantFilterHasAttachmentFalseMatchesEmptySourceSystem(t *testing.T) {
	hasAttachment := false
	filter := toQdrantFilter(memory.SearchFilters{HasAttachment: &hasAttachment})
	if filter == nil {
		t.Fatal("expected filter")
	}
	if _, ok := filter["must_not"]; ok {
		t.Fatalf("expected no must_not clause for has_attachment=false, got %#v", filter["must_not"])
	}
	must, ok := filter["must"].([]map[string]any)
	if !ok || len(must) != 1 {
		t.Fatalf("expected exactly one must clause for has_attachment=false, got %#v", filter["must"])
	}
	if must[0]["key"] != "attachment_source_system" {
		t.Fatalf("expected must clause on attachment_source_system, got %#v", must[0])
	}
}

func TestMemoryPointFromPayloadIncludesAIFields(t *testing.T) {
	point := memoryPointFromPayload(map[string]any{
		"memory_id":       "m1",
		"conversation_id": "c1",
		"source_type":     "chat_export",
		"source_title":    "Title",
		"speaker":         "assistant",
		"memory_kind":     memory.KindProjectContext,
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
		"text":            "hello",
		"source_quote":    "hello",
		"ai_provider":     "chatgpt",
		"ai_model":        "gpt-4o",
	})

	if point.AIProvider != "chatgpt" {
		t.Fatalf("expected ai_provider chatgpt, got %q", point.AIProvider)
	}
	if point.AIModel != "gpt-4o" {
		t.Fatalf("expected ai_model gpt-4o, got %q", point.AIModel)
	}
}

func TestStatsAvoidsFullScrollAndCachesDistinctValues(t *testing.T) {
	scrollCalls := 0
	collectionInfoCalls := 0

	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/frontpocket_test":
			collectionInfoCalls++
			_, _ = io.WriteString(w, `{"result":{"points_count":322000}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/collections/frontpocket_test/facet":
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = io.WriteString(w, `{"status":{"error":"facet unavailable"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/collections/frontpocket_test/points/scroll":
			scrollCalls++
			_, _ = io.WriteString(w, `{"result":{"points":[{"payload":{"memory_kind":"project_context","speaker":"user","project":"FrontPocket","source_title":"Title A"}},{"payload":{"memory_kind":"research_note","speaker":"assistant","project":"Notebook","source_title":"Title B"}}],"next_page_offset":null}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/collections/frontpocket_test/points/count":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("failed reading count request: %v", err)
			}
			raw := string(body)
			count := 0
			switch {
			case strings.Contains(raw, `"key":"memory_kind"`) && strings.Contains(raw, `"value":"project_context"`):
				count = 210000
			case strings.Contains(raw, `"key":"memory_kind"`) && strings.Contains(raw, `"value":"research_note"`):
				count = 112000
			case strings.Contains(raw, `"key":"speaker"`) && strings.Contains(raw, `"value":"user"`):
				count = 200000
			case strings.Contains(raw, `"key":"speaker"`) && strings.Contains(raw, `"value":"assistant"`):
				count = 122000
			case strings.Contains(raw, `"key":"project"`) && strings.Contains(raw, `"value":"FrontPocket"`):
				count = 220000
			case strings.Contains(raw, `"key":"project"`) && strings.Contains(raw, `"value":"Notebook"`):
				count = 102000
			case strings.Contains(raw, `"key":"source_title"`) && strings.Contains(raw, `"value":"Title A"`):
				count = 123000
			case strings.Contains(raw, `"key":"source_title"`) && strings.Contains(raw, `"value":"Title B"`):
				count = 99000
			default:
				count = 0
			}
			_, _ = io.WriteString(w, fmt.Sprintf(`{"result":{"count":%d}}`, count))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qdrant.Close()

	memStore := NewQdrantMemoryStore(NewQdrantClient(qdrant.URL), nil, "frontpocket_test", "", "Cosine", nil)
	memStore.distinctCacheTTL = time.Minute

	first, err := memStore.Stats(context.Background(), memory.SearchFilters{})
	if err != nil {
		t.Fatalf("first stats call failed: %v", err)
	}
	second, err := memStore.Stats(context.Background(), memory.SearchFilters{})
	if err != nil {
		t.Fatalf("second stats call failed: %v", err)
	}

	if first.Total != 322000 || second.Total != 322000 {
		t.Fatalf("expected total from collection info, got first=%d second=%d", first.Total, second.Total)
	}
	if scrollCalls != 1 {
		t.Fatalf("expected one sampled aggregation scroll on first call only, got %d", scrollCalls)
	}
	if collectionInfoCalls != 2 {
		t.Fatalf("expected collection info on both calls, got %d", collectionInfoCalls)
	}
	if first.ByKind["project_context"] != 1 || first.ByKind["research_note"] != 1 {
		t.Fatalf("unexpected by_kind counts: %#v", first.ByKind)
	}
	if len(first.TopTitles) != 2 || first.TopTitles[0] != "Title A" {
		t.Fatalf("unexpected top titles: %#v", first.TopTitles)
	}
}

type testEmbedder struct {
	embedText func(ctx context.Context, text string) ([]float32, error)
}

func (t *testEmbedder) EmbedText(ctx context.Context, text string) ([]float32, error) {
	if t.embedText == nil {
		return []float32{}, nil
	}
	return t.embedText(ctx, text)
}

func (t *testEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vector, err := t.EmbedText(ctx, text)
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, vector)
	}
	return vectors, nil
}

func (t *testEmbedder) ProviderName() string {
	return "test"
}

func (t *testEmbedder) ModelName() string {
	return "test-model"
}
