package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
)

func TestStatsFiltersFromQueryParsesNewFields(t *testing.T) {
	req := httptest.NewRequest("GET", "/memory/stats?ai_provider=claude&ai_model=claude-3-5-sonnet&starred=true&shared=false&feedback_rating=thumbs_down&has_attachment=true", nil)
	filters := statsFiltersFromQuery(req)

	if filters.AIProvider != "claude" {
		t.Fatalf("expected ai_provider=claude, got %q", filters.AIProvider)
	}
	if filters.AIModel != "claude-3-5-sonnet" {
		t.Fatalf("expected ai_model=claude-3-5-sonnet, got %q", filters.AIModel)
	}
	if filters.UserStarred == nil || !*filters.UserStarred {
		t.Fatalf("expected user_starred=true, got %#v", filters.UserStarred)
	}
	if filters.UserShared == nil || *filters.UserShared {
		t.Fatalf("expected user_shared=false, got %#v", filters.UserShared)
	}
	if filters.FeedbackRating != "thumbs_down" {
		t.Fatalf("expected feedback_rating=thumbs_down, got %q", filters.FeedbackRating)
	}
	if filters.HasAttachment == nil || !*filters.HasAttachment {
		t.Fatalf("expected has_attachment=true, got %#v", filters.HasAttachment)
	}
}

func TestParseOptionalBoolSupportsCommonValues(t *testing.T) {
	truthy := []string{"true", "1", "yes", "y"}
	for _, raw := range truthy {
		value := parseOptionalBool(raw)
		if value == nil || !*value {
			t.Fatalf("expected %q to parse true", raw)
		}
	}

	falsey := []string{"false", "0", "no", "n"}
	for _, raw := range falsey {
		value := parseOptionalBool(raw)
		if value == nil || *value {
			t.Fatalf("expected %q to parse false", raw)
		}
	}

	if value := parseOptionalBool("maybe"); value != nil {
		t.Fatalf("expected maybe to parse nil, got %#v", value)
	}
}

func TestHandleMemoryStatsCachesResponses(t *testing.T) {
	store := &stubMemoryStore{stats: memory.MemoryStats{Total: 12}}
	s := &Server{
		memoryStore:       store,
		searchCacheKey:    "frontpocket-test",
		statsCacheTTL:     time.Minute,
		statsQueryTimeout: 100 * time.Millisecond,
		statsCache:        make(map[string]memoryStatsCacheEntry),
	}

	req1 := httptest.NewRequest(http.MethodGet, "/memory/stats", nil)
	rr1 := httptest.NewRecorder()
	s.handleMemoryStats(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("expected first stats call status 200, got %d", rr1.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/memory/stats", nil)
	rr2 := httptest.NewRecorder()
	s.handleMemoryStats(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected second stats call status 200, got %d", rr2.Code)
	}
	if store.calls != 1 {
		t.Fatalf("expected one store stats call with cache hit on second request, got %d", store.calls)
	}
}

func TestHandleMemoryStatsTimeoutReturnsGatewayTimeout(t *testing.T) {
	store := &stubMemoryStore{delayUntilCtxDone: true}
	s := &Server{
		memoryStore:       store,
		searchCacheKey:    "frontpocket-test",
		statsCacheTTL:     time.Minute,
		statsQueryTimeout: 20 * time.Millisecond,
		statsCache:        make(map[string]memoryStatsCacheEntry),
	}

	req := httptest.NewRequest(http.MethodGet, "/memory/stats", nil)
	rr := httptest.NewRecorder()
	s.handleMemoryStats(rr, req)
	if rr.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected timeout status 504, got %d", rr.Code)
	}

	var body memory.ErrorEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode timeout body: %v", err)
	}
	if body.Error.Code != "STATS_TIMEOUT" {
		t.Fatalf("expected STATS_TIMEOUT code, got %q", body.Error.Code)
	}
}

type stubMemoryStore struct {
	stats             memory.MemoryStats
	calls             int
	delayUntilCtxDone bool
}

func (s *stubMemoryStore) Upsert(context.Context, []memory.MemoryPoint) error {
	return nil
}

func (s *stubMemoryStore) Search(context.Context, memory.SearchRequest) ([]memory.SearchResult, error) {
	return nil, nil
}

func (s *stubMemoryStore) Stats(ctx context.Context, _ memory.SearchFilters) (memory.MemoryStats, error) {
	s.calls++
	if s.delayUntilCtxDone {
		<-ctx.Done()
		return memory.MemoryStats{}, ctx.Err()
	}
	return s.stats, nil
}
