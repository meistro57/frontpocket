package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPostJSONSupportsLargeEmbeddingResponses(t *testing.T) {
	largePayload := strings.Repeat("x", 2<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"data": largePayload})
	}))
	defer srv.Close()

	var out struct {
		Data string `json:"data"`
	}
	err := postJSON(context.Background(), newHTTPClient(), srv.URL, nil, map[string]string{"hello": "world"}, &out)
	if err != nil {
		t.Fatalf("expected large response to decode successfully, got error: %v", err)
	}
	if len(out.Data) != len(largePayload) {
		t.Fatalf("expected decoded payload length %d, got %d", len(largePayload), len(out.Data))
	}
}
