package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
	"github.com/meistro57/frontpocket/internal/memoryloop"
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
	if !req.IncludeProposed {
		if cached, ok := s.getCachedSearchResults(r, req); ok {
			writeJSON(w, http.StatusOK, memory.SearchResponse{Query: req.Query, Results: cached})
			return
		}
	}

	results, err := s.searchWithCanonOptions(r, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, memory.ErrorBody{
			Code:    "SEARCH_FAILED",
			Message: "Search failed.",
			Detail:  err.Error(),
		})
		return
	}

	results = s.filterResults(results)
	if !req.IncludeProposed {
		s.setCachedSearchResults(r, req, results)
	}
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
	results, err := s.searchWithCanonOptions(r, memory.SearchRequest{
		Query:           req.Query,
		Limit:           req.Limit,
		Filters:         req.Filters,
		IncludeProposed: req.IncludeProposed,
		IncludeRejected: req.IncludeRejected,
		CanonicalFirst:  req.CanonicalFirst,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, memory.ErrorBody{
			Code:    "CONTEXT_PACK_FAILED",
			Message: "Context pack generation failed.",
			Detail:  err.Error(),
		})
		return
	}

	resp := memory.ContextPackResponse{Query: req.Query, MemoryPack: s.filterResults(results)}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) searchWithCanonOptions(r *http.Request, req memory.SearchRequest) ([]memory.SearchResult, error) {
	results, err := s.memoryStore.Search(r.Context(), req)
	if err != nil {
		return nil, err
	}
	if req.IncludeProposed && s.reviewQueue != nil {
		proposed, listErr := s.reviewQueue.List(memoryloop.CandidateFilter{})
		if listErr == nil {
			results = append(results, s.proposedCandidatesToResults(req, proposed)...)
		}
	}
	if req.CanonicalFirst {
		sort.Slice(results, func(i, j int) bool {
			if results[i].Canonical != results[j].Canonical {
				return results[i].Canonical
			}
			if results[i].Score == results[j].Score {
				return results[i].Timestamp.After(results[j].Timestamp)
			}
			return results[i].Score > results[j].Score
		})
	}
	if req.Limit > 0 && len(results) > req.Limit {
		results = results[:req.Limit]
	}
	return results, nil
}

func (s *Server) proposedCandidatesToResults(req memory.SearchRequest, candidates []memoryloop.Candidate) []memory.SearchResult {
	query := strings.ToLower(strings.TrimSpace(req.Query))
	if query == "" {
		return nil
	}
	out := make([]memory.SearchResult, 0, len(candidates))
	for _, candidate := range candidates {
		if !req.IncludeRejected && (candidate.Status == memory.StatusRejected || candidate.Status == memory.StatusContradicted || candidate.Status == memory.StatusOutdated) {
			continue
		}
		if req.Filters.Project != "" && !strings.EqualFold(candidate.Project, req.Filters.Project) {
			continue
		}
		if req.Filters.MemoryKind != "" && !strings.EqualFold(candidate.MemoryKind, req.Filters.MemoryKind) {
			continue
		}
		if req.Filters.Speaker != "" && !strings.EqualFold(candidate.Speaker, req.Filters.Speaker) {
			continue
		}
		searchText := strings.ToLower(candidate.Summary + " " + strings.Join(candidate.SourceQuotes, " "))
		score := simpleSearchScore(query, searchText)
		if score <= 0 {
			continue
		}
		statusBoost := 1.0
		switch candidate.Status {
		case memory.StatusDirectUserStatement, memory.StatusApprovedByUser:
			statusBoost = 1.2
		case memory.StatusInferredFromSources:
			statusBoost = 1.08
		case memory.StatusNeedsReview:
			statusBoost = 0.98
		}
		score = score * statusBoost
		out = append(out, memory.SearchResult{
			MemoryID:        candidate.ID,
			ConversationID:  "proposed_canon",
			SourceTitle:     "Proposed Canon",
			SourceType:      "proposed_canon",
			Timestamp:       candidate.CreatedAt,
			Speaker:         candidate.Speaker,
			Project:         candidate.Project,
			MemoryKind:      candidate.MemoryKind,
			Tags:            append([]string(nil), candidate.Tags...),
			Summary:         candidate.Summary,
			SourceQuote:     firstSourceQuote(candidate.SourceQuotes),
			Text:            candidate.Text,
			Score:           score,
			Canonical:       false,
			Confidence:      candidate.Confidence,
			Status:          candidate.Status,
			SourceMemoryIDs: append([]string(nil), candidate.SourceMemoryIDs...),
			SourceQuotes:    append([]string(nil), candidate.SourceQuotes...),
			ReviewedAt:      candidate.ReviewedAt,
			ReviewedBy:      candidate.ReviewedBy,
			CreatedByLoop:   candidate.CreatedByLoop,
			Supersedes:      append([]string(nil), candidate.Supersedes...),
			MergedFrom:      append([]string(nil), candidate.MergedFrom...),
			ApproximateDate: candidate.ApproximateDate,
			DateBasis:       candidate.DateBasis,
		})
	}
	return out
}

func simpleSearchScore(query, text string) float64 {
	queryTokens := strings.Fields(query)
	if len(queryTokens) == 0 {
		return 0
	}
	matches := 0
	for _, token := range queryTokens {
		if strings.Contains(text, token) {
			matches++
		}
	}
	if matches == 0 {
		return 0
	}
	return float64(matches) / float64(len(queryTokens))
}

func firstSourceQuote(quotes []string) string {
	for _, quote := range quotes {
		trimmed := strings.TrimSpace(quote)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (s *Server) handleMemoryStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.memoryStore.Stats(r.Context(), statsFiltersFromQuery(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, memory.ErrorBody{
			Code:    "STATS_FAILED",
			Message: "Memory stats lookup failed.",
			Detail:  err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleMemorySession(w http.ResponseWriter, r *http.Request) {
	var req memory.SessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{
			Code:    "INVALID_REQUEST",
			Message: "Request body must be valid JSON.",
			Detail:  err.Error(),
		})
		return
	}

	req.SessionID = strings.TrimSpace(req.SessionID)
	if req.SessionID == "" {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{
			Code:    "VALIDATION_ERROR",
			Message: "session_id is required.",
		})
		return
	}

	if req.LoadOnly {
		state, found, err := s.getSessionState(r, req.SessionID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, memory.ErrorBody{
				Code:    "SESSION_FAILED",
				Message: "Session lookup failed.",
				Detail:  err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, memory.SessionResponse{Found: found, State: state})
		return
	}

	state := memory.SessionState{
		SessionID:       req.SessionID,
		Project:         strings.TrimSpace(req.Project),
		ActiveSummary:   strings.TrimSpace(req.ActiveSummary),
		RecentMemoryIDs: cleanStrings(req.RecentMemoryIDs),
		Metadata:        req.Metadata,
		UpdatedAt:       time.Now().UTC(),
	}

	ttl := req.TTLSeconds
	if ttl <= 0 {
		ttl = int(s.defaultSessionTTL.Seconds())
	}
	if ttl <= 0 {
		ttl = 3600
	}

	if err := s.setSessionState(r, state, ttl); err != nil {
		writeError(w, http.StatusInternalServerError, memory.ErrorBody{
			Code:    "SESSION_FAILED",
			Message: "Session update failed.",
			Detail:  err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, memory.SessionResponse{Found: true, State: &state})
}

func (s *Server) handleMemorySessionDelete(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{
			Code:    "VALIDATION_ERROR",
			Message: "session_id query parameter is required.",
		})
		return
	}

	if err := s.deleteSessionState(r, sessionID); err != nil {
		writeError(w, http.StatusInternalServerError, memory.ErrorBody{
			Code:    "SESSION_FAILED",
			Message: "Session delete failed.",
			Detail:  err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, memory.SessionResponse{Found: false})
}

func statsFiltersFromQuery(r *http.Request) memory.SearchFilters {
	query := r.URL.Query()
	filters := memory.SearchFilters{
		Project:        strings.TrimSpace(query.Get("project")),
		MemoryKind:     strings.TrimSpace(query.Get("memory_kind")),
		Speaker:        strings.TrimSpace(query.Get("speaker")),
		SourceType:     strings.TrimSpace(query.Get("source_type")),
		ConversationID: strings.TrimSpace(query.Get("conversation_id")),
	}

	tags := make([]string, 0)
	for _, raw := range query["tag"] {
		for _, tag := range strings.Split(raw, ",") {
			trimmed := strings.TrimSpace(tag)
			if trimmed != "" {
				tags = append(tags, trimmed)
			}
		}
	}
	for _, tag := range strings.Split(query.Get("tags"), ",") {
		trimmed := strings.TrimSpace(tag)
		if trimmed != "" {
			tags = append(tags, trimmed)
		}
	}
	if len(tags) > 0 {
		filters.Tags = tags
	}

	return filters
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

func (s *Server) sessionStateKey(sessionID string) string {
	prefix := s.searchCacheKey
	if prefix == "" {
		prefix = "frontpocket"
	}
	return fmt.Sprintf("%s:session:%s", prefix, sessionID)
}

func (s *Server) getSessionState(r *http.Request, sessionID string) (*memory.SessionState, bool, error) {
	key := s.sessionStateKey(sessionID)
	if s.redis != nil {
		cached, found, err := s.redis.Get(r.Context(), key)
		if err == nil && found {
			var state memory.SessionState
			if unmarshalErr := json.Unmarshal([]byte(cached), &state); unmarshalErr == nil {
				s.sessionFallbackMu.Lock()
				s.sessionFallback[sessionID] = state
				s.sessionFallbackMu.Unlock()
				return &state, true, nil
			}
		}
	}

	s.sessionFallbackMu.RLock()
	fallbackState, ok := s.sessionFallback[sessionID]
	s.sessionFallbackMu.RUnlock()
	if !ok {
		return nil, false, nil
	}
	state := fallbackState
	return &state, true, nil
}

func (s *Server) setSessionState(r *http.Request, state memory.SessionState, ttlSeconds int) error {
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}

	s.sessionFallbackMu.Lock()
	s.sessionFallback[state.SessionID] = state
	s.sessionFallbackMu.Unlock()

	if s.redis == nil {
		return nil
	}
	_ = s.redis.SetEX(r.Context(), s.sessionStateKey(state.SessionID), string(payload), time.Duration(ttlSeconds)*time.Second)
	return nil
}

func (s *Server) deleteSessionState(r *http.Request, sessionID string) error {
	s.sessionFallbackMu.Lock()
	delete(s.sessionFallback, sessionID)
	s.sessionFallbackMu.Unlock()

	if s.redis == nil {
		return nil
	}
	_ = s.redis.Del(r.Context(), s.sessionStateKey(sessionID))
	return nil
}

func cleanStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, body memory.ErrorBody) {
	writeJSON(w, status, memory.ErrorEnvelope{Error: body})
}
