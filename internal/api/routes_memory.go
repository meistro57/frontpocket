package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/meistro57/frontpocket/internal/memory"
)

func (s *Server) handleMemoryIngest(w http.ResponseWriter, r *http.Request) {
	var req memory.IngestChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{
			Code:    "INVALID_REQUEST",
			Message: "Request body must be valid JSON.",
			Detail:  err.Error(),
		})
		return
	}

	records := req.Records
	if len(records) == 0 && strings.TrimSpace(req.JSONL) != "" {
		parsed, err := memory.ParseJSONL(req.JSONL)
		if err != nil {
			writeError(w, http.StatusBadRequest, memory.ErrorBody{
				Code:    "INVALID_JSONL",
				Message: "JSONL payload could not be parsed.",
				Detail:  err.Error(),
			})
			return
		}
		records = parsed
	}

	if len(records) == 0 {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{
			Code:    "EMPTY_INPUT",
			Message: "At least one record is required.",
			Detail:  "Provide records or a JSONL payload.",
		})
		return
	}

	ingestor := s.ingestor
	if strings.TrimSpace(req.SourceTitle) != "" {
		ingestor.SourceTitle = req.SourceTitle
	}
	if strings.TrimSpace(req.SourceType) != "" {
		ingestor.SourceType = req.SourceType
	}
	if strings.TrimSpace(req.Project) != "" {
		ingestor.Project = req.Project
	}

	points, err := ingestor.Ingest(r.Context(), records)
	if err != nil {
		writeError(w, http.StatusInternalServerError, memory.ErrorBody{
			Code:    "INGEST_FAILED",
			Message: "Could not ingest memory records.",
			Detail:  err.Error(),
		})
		return
	}

	ids := make([]string, 0, len(points))
	for _, p := range points {
		ids = append(ids, p.MemoryID)
	}

	writeJSON(w, http.StatusOK, memory.IngestChatResponse{
		InsertedCount: len(points),
		MemoryIDs:     ids,
	})
}

func (s *Server) handleMemorySearch(w http.ResponseWriter, r *http.Request) {
	var req memory.SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{
			Code:    "INVALID_REQUEST",
			Message: "Request body must be valid JSON.",
			Detail:  err.Error(),
		})
		return
	}

	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{
			Code:    "VALIDATION_ERROR",
			Message: "query is required.",
		})
		return
	}

	req.Limit = s.clampLimit(req.Limit)
	if cached, ok := s.getCachedSearchResults(r, req); ok {
		writeJSON(w, http.StatusOK, memory.SearchResponse{Query: req.Query, Results: cached})
		return
	}

	results, err := s.memoryStore.Search(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, memory.ErrorBody{
			Code:    "SEARCH_FAILED",
			Message: "Search failed.",
			Detail:  err.Error(),
		})
		return
	}

	results = s.filterResults(results)
	s.setCachedSearchResults(r, req, results)
	writeJSON(w, http.StatusOK, memory.SearchResponse{Query: req.Query, Results: results})
}

func (s *Server) handleContextPack(w http.ResponseWriter, r *http.Request) {
	var req memory.ContextPackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{
			Code:    "INVALID_REQUEST",
			Message: "Request body must be valid JSON.",
			Detail:  err.Error(),
		})
		return
	}

	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{
			Code:    "VALIDATION_ERROR",
			Message: "query is required.",
		})
		return
	}

	req.Limit = s.clampContextLimit(req.Limit)
	resp, err := s.contextPacker.Build(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, memory.ErrorBody{
			Code:    "CONTEXT_PACK_FAILED",
			Message: "Context pack generation failed.",
			Detail:  err.Error(),
		})
		return
	}

	resp.MemoryPack = s.filterResults(resp.MemoryPack)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) clampLimit(limit int) int {
	if limit <= 0 {
		return s.defaultSearch
	}
	if limit > s.maxSearch {
		return s.maxSearch
	}
	return limit
}

func (s *Server) clampContextLimit(limit int) int {
	if limit <= 0 {
		return s.cfg.ContextPack.DefaultLimit
	}
	if limit > s.cfg.ContextPack.MaxLimit {
		return s.cfg.ContextPack.MaxLimit
	}
	return limit
}

func (s *Server) filterResults(results []memory.SearchResult) []memory.SearchResult {
	filtered := make([]memory.SearchResult, 0, len(results))
	for _, result := range results {
		if result.Score < s.minSearchScore {
			continue
		}
		if !s.cfg.Search.IncludeSourceQuote {
			result.SourceQuote = ""
		}
		if !s.cfg.Search.IncludeFullText {
			result.Text = ""
		}
		filtered = append(filtered, result)
	}
	return filtered
}

func (s *Server) getCachedSearchResults(r *http.Request, req memory.SearchRequest) ([]memory.SearchResult, bool) {
	if s.redis == nil || s.searchCacheTTL <= 0 {
		return nil, false
	}

	key := s.searchResultCacheKey(req)
	cached, found, err := s.redis.Get(r.Context(), key)
	if err != nil || !found {
		return nil, false
	}

	var results []memory.SearchResult
	if err := json.Unmarshal([]byte(cached), &results); err != nil {
		return nil, false
	}
	return results, true
}

func (s *Server) setCachedSearchResults(r *http.Request, req memory.SearchRequest, results []memory.SearchResult) {
	if s.redis == nil || s.searchCacheTTL <= 0 {
		return
	}

	payload, err := json.Marshal(results)
	if err != nil {
		return
	}
	_ = s.redis.SetEX(r.Context(), s.searchResultCacheKey(req), string(payload), s.searchCacheTTL)
}

func (s *Server) searchResultCacheKey(req memory.SearchRequest) string {
	prefix := s.searchCacheKey
	if prefix == "" {
		prefix = "frontpocket"
	}

	encoded, _ := json.Marshal(req)
	sum := sha256.Sum256(encoded)
	return fmt.Sprintf("%s:search:%s", prefix, hex.EncodeToString(sum[:]))
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, body memory.ErrorBody) {
	writeJSON(w, status, memory.ErrorEnvelope{Error: body})
}
