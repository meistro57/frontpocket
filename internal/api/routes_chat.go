package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/meistro57/frontpocket/internal/chat"

	"github.com/meistro57/frontpocket/internal/config"
	"github.com/meistro57/frontpocket/internal/memory"
	"github.com/meistro57/frontpocket/internal/store"
)

// chatRequestTimeout bounds the whole chat request — including the up-to-two
// sequential LLM calls (query refinement + answer generation), each of which
// the chat client allows 60s. It is kept just under the frontend's abort
// deadline so the server returns a clean error before the browser gives up.
const chatRequestTimeout = 120 * time.Second

func (s *Server) handleMemoryChat(w http.ResponseWriter, r *http.Request) {
	var req memory.ChatMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{
			Code:    "INVALID_REQUEST",
			Message: "Request body must be valid JSON.",
			Detail:  err.Error(),
		})
		return
	}

	req.SessionID = strings.TrimSpace(req.SessionID)
	req.Message = strings.TrimSpace(req.Message)
	req.SystemPrompt = strings.TrimSpace(req.SystemPrompt)
	req.Project = strings.TrimSpace(req.Project)
	if req.SessionID == "" {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{
			Code:    "VALIDATION_ERROR",
			Message: "session_id is required.",
		})
		return
	}
	if req.Message == "" {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{
			Code:    "VALIDATION_ERROR",
			Message: "message is required.",
		})
		return
	}

	frontLimit := req.FrontPocketTopK
	if frontLimit <= 0 {
		frontLimit = s.defaultSearch
	}
	if frontLimit > s.maxSearch {
		frontLimit = s.maxSearch
	}

	mindLimit := req.MindDrillTopK
	if mindLimit <= 0 {
		mindLimit = s.cfg.MindDrillMemory.TopK
	}
	if mindLimit > s.maxSearch {
		mindLimit = s.maxSearch
	}

	ctx, cancel := context.WithTimeout(r.Context(), chatRequestTimeout)
	defer cancel()

	frontReq := memory.SearchRequest{
		Query: req.Message,
		Limit: frontLimit,
		Filters: memory.SearchFilters{
			Project: req.Project,
		},
	}
	frontResults, err := s.memoryStore.Search(ctx, frontReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, memory.ErrorBody{
			Code:    "CHAT_FRONTPOCKET_SEARCH_FAILED",
			Message: "FrontPocket memory search failed.",
			Detail:  err.Error(),
		})
		return
	}
	frontResults = s.filterResults(frontResults)

	mindResults := make([]memory.SearchResult, 0)
	if s.cfg.MindDrillMemory.Enabled {
		mindReq := memory.SearchRequest{
			Query: req.Message,
			Limit: mindLimit,
			Filters: memory.SearchFilters{
				Project: req.Project,
			},
		}
		mindResults, err = s.mindDrillStore.Search(ctx, mindReq)
		if err != nil {
			writeError(w, http.StatusInternalServerError, memory.ErrorBody{
				Code:    "CHAT_MINDDRILL_SEARCH_FAILED",
				Message: "MindDrill memory search failed.",
				Detail:  err.Error(),
			})
			return
		}
		mindResults = s.filterResults(mindResults)
	}

	// Second-pass retrieval: if the first search came back thin or seems to be matching
	// the *shape* of the question rather than real content (low scores, or mostly other
	// chat-turn questions rather than source material), ask the chat model for a sharper
	// search angle and re-search before answering. Skipped when retrieval already looks
	// solid, or when there's no chat client to ask.
	if s.chatClient != nil && retrievalLooksThin(frontResults, mindResults) {
		if refinedQuery := s.proposeRefinedQuery(ctx, req.Message, frontResults, mindResults); refinedQuery != "" {
			refinedFrontReq := memory.SearchRequest{
				Query: refinedQuery,
				Limit: frontLimit,
				Filters: memory.SearchFilters{
					Project: req.Project,
				},
			}
			if refinedFront, refErr := s.memoryStore.Search(ctx, refinedFrontReq); refErr == nil {
				frontResults = mergeSearchResults(frontResults, s.filterResults(refinedFront), frontLimit)
			} else {
				s.logger.Warn("minddrill refined search failed", "session_id", req.SessionID, "error", refErr)
			}
		}
	}

	contextPack := buildMindDrillPrompt(req.Message, req.SystemPrompt, mindResults, frontResults)
	answer, provider, model, err := s.generateChatAnswer(ctx, req.Message, req.SystemPrompt, contextPack, mindResults, frontResults)
	if err != nil {
		writeError(w, http.StatusBadGateway, memory.ErrorBody{
			Code:    "CHAT_COMPLETION_FAILED",
			Message: "Chat provider failed to generate a response.",
			Detail:  err.Error(),
		})
		return
	}

	answer, suggestions := extractSuggestions(answer)

	resp := memory.ChatMessageResponse{
		Answer:                  answer,
		Suggestions:             suggestions,
		UsedFrontPocketMemories: frontResults,
		UsedMindDrillMemories:   mindResults,
		ContextPack:             contextPack,
		Model:                   model,
		Provider:                provider,
	}

	if s.cfg.MindDrillMemory.Enabled {
		if err := s.writeMindDrillChatMemory(r, req, resp); err != nil {
			s.logger.Warn("minddrill memory write failed", "session_id", req.SessionID, "error", err)
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// extractSuggestions looks for a trailing 'SUGGESTIONS: a | b | c' line in the model's
// answer, strips it out, and returns the cleaned answer plus the parsed suggestion list.
// If no such line is present, the answer is returned unchanged with a nil slice.
func extractSuggestions(answer string) (string, []string) {
	lines := strings.Split(answer, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		const prefix = "SUGGESTIONS:"
		if !strings.HasPrefix(strings.ToUpper(trimmed), prefix) {
			break // only look at the very last non-blank line
		}

		raw := strings.TrimSpace(trimmed[len(prefix):])
		parts := strings.Split(raw, "|")
		suggestions := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.Trim(strings.TrimSpace(p), "\"'")
			if p != "" && len(p) <= 120 {
				suggestions = append(suggestions, p)
			}
		}
		if len(suggestions) == 0 {
			break
		}

		cleaned := strings.TrimSpace(strings.Join(lines[:i], "\n"))
		return cleaned, suggestions
	}
	return answer, nil
}

func retrievalLooksThin(front, mind []memory.SearchResult) bool {
	if len(front) == 0 {
		return true
	}

	// Average score across the front results: a low average suggests the query matched
	// loosely, not on real content.
	var sum float64
	for _, r := range front {
		sum += r.Score
	}
	avg := sum / float64(len(front))
	if avg < 0.55 {
		return true
	}

	// Self-referential retrieval: most of the top hits are themselves prior MindDrill
	// chat turns rather than source-backed memory, which is the "hall of mirrors"
	// pattern where a vague question keeps matching other instances of itself.
	selfReferential := 0
	for _, r := range front {
		if strings.EqualFold(strings.TrimSpace(r.SourceType), "minddrill_chat") {
			selfReferential++
		}
	}
	if len(front) > 0 && float64(selfReferential)/float64(len(front)) > 0.6 {
		return true
	}

	return false
}

// proposeRefinedQuery asks the chat model for a sharper search angle when the first-pass
// retrieval looked thin or self-referential. Returns an empty string if it can't produce
// one, in which case the caller just proceeds with the original results.
func (s *Server) proposeRefinedQuery(ctx context.Context, originalMessage string, front, mind []memory.SearchResult) string {
	if s.chatClient == nil {
		return ""
	}
	_ = mind // reserved for future use if MindDrill chat history should also inform the refined query

	var b strings.Builder
	b.WriteString("The user asked: \"")
	b.WriteString(originalMessage)
	b.WriteString("\"\n\n")
	b.WriteString("A first-pass vector search against their memory corpus came back thin or self-referential ")
	b.WriteString("(matching the shape of the question rather than real content). Here is what the weak search returned:\n\n")
	b.WriteString(formatMemoryList(front))
	b.WriteString("\nPropose ONE better search query — a concrete topic, theme, project name, or angle — that would surface ")
	b.WriteString("actual substantive content from this person's memory archive instead of more meta-questions. ")
	b.WriteString("Pick something specific and likely to exist in a personal chat/document archive (a recurring project, ")
	b.WriteString("a strong theme, a domain like work or a hobby, an emotional thread, anything concrete). ")
	b.WriteString("Reply with ONLY the search query text, 3-8 words, nothing else — no quotes, no explanation, no preamble.")

	result, err := s.chatClient.Complete(ctx, []chat.Message{
		{Role: "system", Content: "You generate concise, content-rich search queries. Reply with only the query, nothing else."},
		{Role: "user", Content: b.String()},
	})
	if err != nil {
		s.logger.Warn("minddrill query refinement failed", "error", err)
		return ""
	}

	refined := strings.Trim(strings.TrimSpace(result), "\"'")
	if refined == "" || len(refined) > 200 {
		return ""
	}
	return refined
}

// mergeSearchResults combines two result sets, de-duplicating by memory ID and preferring
// the higher score for any overlap, then returns the top `limit` by score.
func mergeSearchResults(a, b []memory.SearchResult, limit int) []memory.SearchResult {
	byID := make(map[string]memory.SearchResult, len(a)+len(b))
	for _, r := range a {
		byID[r.MemoryID] = r
	}
	for _, r := range b {
		existing, ok := byID[r.MemoryID]
		if !ok || r.Score > existing.Score {
			byID[r.MemoryID] = r
		}
	}

	merged := make([]memory.SearchResult, 0, len(byID))
	for _, r := range byID {
		merged = append(merged, r)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})

	if limit > 0 && len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}

func (s *Server) generateChatAnswer(ctx context.Context, message, systemPrompt, contextPack string, mind, front []memory.SearchResult) (string, string, string, error) {
	provider, model := chatProviderModel(s.cfg.Chat)
	if s.chatClient == nil {
		return buildMindDrillAnswer(message, mind, front), provider, model, nil
	}

	baseSystemPrompt := "You are MindDrill's chat assistant, helping the user explore and reason over their FrontPocket memory archive. Answer with the retrieved memory context only when relevant. Be direct, source-aware, and explicit when memory is missing or uncertain. Do not claim remembered facts unless they appear in the supplied memory sections. " +
		"IMPORTANT: the retrieved memory sections are a SIMILARITY SAMPLE, not an exhaustive dataset — they are the small number of chunks closest in meaning to the user's message, drawn from a much larger corpus. If the user asks something that requires an exhaustive count, complete list, or 'how many X exist', say plainly that vector retrieval cannot answer that reliably (it samples by similarity, not completeness), and suggest they use the search or browse tools with a narrower query instead of guessing a number. " +
		"FOLLOW-UP SUGGESTIONS: when it's natural to offer the user a few directions to dig further (which is often, given this is an exploratory memory tool), end your reply with a final line starting with exactly 'SUGGESTIONS:' followed by 2-4 short follow-up prompts separated by ' | '. Each suggestion should be phrased as something the user could literally click and send as-is (first person, e.g. 'tell me more about the marriage thread', not 'the marriage thread'). Keep each suggestion under 8 words. Omit the SUGGESTIONS line entirely if there's nothing natural to suggest — do not force it on short factual answers."
	if trimmed := strings.TrimSpace(systemPrompt); trimmed != "" {
		baseSystemPrompt += "\n\nUser-provided persona/system prompt information:\n" + trimmed
	}

	answer, err := s.chatClient.Complete(ctx, []chat.Message{
		{
			Role:    "system",
			Content: baseSystemPrompt,
		},
		{
			Role:    "user",
			Content: contextPack,
		},
	})
	if err != nil {
		return "", provider, model, err
	}
	return answer, s.chatClient.ProviderName(), s.chatClient.ModelName(), nil
}

func (s *Server) handleMindDrillMemoryStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.mindDrillStore.Stats(r.Context(), statsFiltersFromQuery(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, memory.ErrorBody{
			Code:    "MINDDRILL_STATS_FAILED",
			Message: "MindDrill memory stats lookup failed.",
			Detail:  err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleMindDrillMemorySearch(w http.ResponseWriter, r *http.Request) {
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

	results, err := s.mindDrillStore.Search(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, memory.ErrorBody{
			Code:    "MINDDRILL_SEARCH_FAILED",
			Message: "MindDrill search failed.",
			Detail:  err.Error(),
		})
		return
	}

	results = s.filterResults(results)
	writeJSON(w, http.StatusOK, memory.SearchResponse{Query: req.Query, Results: results})
}

func (s *Server) handleMemoryChatSessionDelete(w http.ResponseWriter, r *http.Request) {
	resp, errBody, status := s.deleteMindDrillChatSession(r)
	if errBody != nil {
		writeError(w, status, *errBody)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleMindDrillMemorySessionDelete(w http.ResponseWriter, r *http.Request) {
	resp, errBody, status := s.deleteMindDrillChatSession(r)
	if errBody != nil {
		writeError(w, status, *errBody)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) deleteMindDrillChatSession(r *http.Request) (memory.ChatSessionDeleteResponse, *memory.ErrorBody, int) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		return memory.ChatSessionDeleteResponse{}, &memory.ErrorBody{
			Code:    "VALIDATION_ERROR",
			Message: "session_id query parameter is required.",
		}, http.StatusBadRequest
	}

	if err := s.deleteSessionState(r, sessionID); err != nil {
		return memory.ChatSessionDeleteResponse{}, &memory.ErrorBody{
			Code:    "SESSION_FAILED",
			Message: "Session delete failed.",
			Detail:  err.Error(),
		}, http.StatusInternalServerError
	}

	if qStore, ok := s.mindDrillStore.(*store.QdrantMemoryStore); ok {
		if err := qStore.DeleteByFilters(r.Context(), memory.SearchFilters{ConversationID: sessionID}); err != nil {
			return memory.ChatSessionDeleteResponse{}, &memory.ErrorBody{
				Code:    "MINDDRILL_SESSION_DELETE_FAILED",
				Message: "MindDrill session memory delete failed.",
				Detail:  err.Error(),
			}, http.StatusInternalServerError
		}
	}
	if memStore, ok := s.mindDrillStore.(*memory.InMemoryStore); ok {
		_ = memStore.DeleteByFilters(memory.SearchFilters{ConversationID: sessionID})
	}

	return memory.ChatSessionDeleteResponse{
		SessionID:      sessionID,
		SessionCleared: true,
		MemoryCleared:  true,
	}, nil, http.StatusOK
}

func (s *Server) writeMindDrillChatMemory(r *http.Request, req memory.ChatMessageRequest, resp memory.ChatMessageResponse) error {
	state, _, err := s.getSessionState(r, req.SessionID)
	if err != nil {
		return err
	}

	turnCount := 0
	metadata := map[string]string{}
	recentIDs := make([]string, 0)
	activeSummary := ""
	if state != nil {
		for k, v := range state.Metadata {
			metadata[k] = v
		}
		recentIDs = append(recentIDs, state.RecentMemoryIDs...)
		activeSummary = state.ActiveSummary
		if raw := strings.TrimSpace(metadata["minddrill_turn_count"]); raw != "" {
			if parsed, parseErr := strconv.Atoi(raw); parseErr == nil {
				turnCount = parsed
			}
		}
	}
	turnCount++

	safeSession := sanitizeID(req.SessionID)
	turnLabel := fmt.Sprintf("%06d", turnCount)
	usedIDs := collectUsedMemoryIDs(resp.UsedFrontPocketMemories, resp.UsedMindDrillMemories)
	now := time.Now().UTC()

	points := make([]memory.MemoryPoint, 0, 4)
	if s.cfg.MindDrillMemory.WriteMode == "raw" {
		points = append(points,
			s.newMindDrillPoint(req, now, "minddrill_"+safeSession+"_"+turnLabel+"_user", "user", memory.KindChatTurn, req.Message, summarizeText(req.Message), 0.12, nil, usedIDs),
			s.newMindDrillPoint(req, now, "minddrill_"+safeSession+"_"+turnLabel+"_assistant", "assistant", memory.KindChatTurn, resp.Answer, summarizeText(resp.Answer), 0.12, nil, usedIDs),
		)
	} else {
		summary := summarizeText("User: " + req.Message + "\nAssistant: " + resp.Answer)
		points = append(points, s.newMindDrillPoint(req, now, "minddrill_"+safeSession+"_"+turnLabel, "system", memory.KindChatTurn, "User: "+req.Message+"\nAssistant: "+resp.Answer, summary, 0.24, []string{"summary"}, usedIDs))
	}

	signalKind, signalImportance, signalTags := detectMindDrillSignal(req.Message, req.RememberThis)
	if signalKind != "" {
		signalSummary := summarizeText(req.Message)
		points = append(points, s.newMindDrillPoint(req, now, "minddrill_"+safeSession+"_"+turnLabel+"_signal", "user", signalKind, req.Message, signalSummary, signalImportance, signalTags, usedIDs))
	}

	if s.cfg.MindDrillMemory.SessionSummaryEvery > 0 && turnCount%s.cfg.MindDrillMemory.SessionSummaryEvery == 0 {
		summary := summarizeText(strings.TrimSpace(activeSummary + "\n" + req.Message + "\n" + resp.Answer))
		activeSummary = summary
		points = append(points, s.newMindDrillPoint(req, now, "minddrill_"+safeSession+"_session_summary", "system", memory.KindSessionSummary, summary, summary, 0.62, []string{"session-summary"}, usedIDs))
	}

	if len(points) > 0 {
		if err := s.mindDrillStore.Upsert(r.Context(), points); err != nil {
			return err
		}
	}

	newIDs := make([]string, 0, len(points))
	for _, point := range points {
		newIDs = append(newIDs, point.MemoryID)
	}
	recentIDs = append(recentIDs, newIDs...)
	if len(recentIDs) > 32 {
		recentIDs = recentIDs[len(recentIDs)-32:]
	}
	metadata["minddrill_turn_count"] = strconv.Itoa(turnCount)

	stateToSave := memory.SessionState{
		SessionID:       req.SessionID,
		Project:         req.Project,
		ActiveSummary:   activeSummary,
		RecentMemoryIDs: recentIDs,
		Metadata:        metadata,
		UpdatedAt:       now,
	}
	return s.setSessionState(r, stateToSave, int(s.defaultSessionTTL.Seconds()))
}

func (s *Server) newMindDrillPoint(req memory.ChatMessageRequest, now time.Time, memoryID, speaker, memoryKind, text, summary string, importance float64, tags []string, usedMemoryIDs []string) memory.MemoryPoint {
	allTags := append([]string{"minddrill", "chat"}, tags...)
	return memory.MemoryPoint{
		MemoryID:            memoryID,
		ConversationID:      req.SessionID,
		SessionID:           req.SessionID,
		SourceType:          "minddrill_chat",
		SourceTitle:         "MindDrill Chat",
		Timestamp:           now,
		Speaker:             strings.TrimSpace(speaker),
		Project:             req.Project,
		Tags:                uniqueStrings(allTags),
		MemoryKind:          strings.TrimSpace(memoryKind),
		Importance:          importance,
		Text:                strings.TrimSpace(text),
		SourceQuote:         clampRunes(text, 180),
		Summary:             summarizeText(summary),
		UsedMemoryIDs:       append([]string(nil), usedMemoryIDs...),
		EmbeddingProvider:   s.cfg.Embedding.Provider,
		EmbeddingModel:      embeddingModel(s.cfg),
		EmbeddingDimensions: s.cfg.Embedding.Dimensions,
	}
}

func buildMindDrillPrompt(message, systemPrompt string, mind, front []memory.SearchResult) string {
	var b strings.Builder
	b.WriteString("SYSTEM:\n")
	b.WriteString("You are Eli. Be direct, source-aware, and explicit about uncertainty.\n")
	if trimmed := strings.TrimSpace(systemPrompt); trimmed != "" {
		b.WriteString("User-provided persona/system prompt information:\n")
		b.WriteString(trimmed)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString("MINDDRILL CHAT MEMORY:\n")
	b.WriteString("These are memories from prior MindDrill chats. Use them as conversational continuity, not as absolute fact.\n")
	b.WriteString(formatMemoryList(mind))
	b.WriteString("\nFRONTPOCKET SOURCE MEMORY:\n")
	b.WriteString("These are source-backed memories from imported chats/documents. Treat these as the stronger evidence layer.\n")
	b.WriteString(formatMemoryList(front))
	b.WriteString("\nUSER MESSAGE:\n")
	b.WriteString(strings.TrimSpace(message))
	return b.String()
}

func buildMindDrillAnswer(message string, mind, front []memory.SearchResult) string {
	if len(front) == 0 && len(mind) == 0 {
		return "I did not retrieve relevant memory yet. Share more context and I will build continuity from this session."
	}
	parts := make([]string, 0, 3)
	parts = append(parts, fmt.Sprintf("I used %d FrontPocket source memories and %d MindDrill chat memories for this reply.", len(front), len(mind)))
	if len(front) > 0 {
		parts = append(parts, "Source-backed context: "+bestMemoryLine(front[0]))
	}
	if len(mind) > 0 {
		parts = append(parts, "Chat continuity: "+bestMemoryLine(mind[0]))
	}
	if len(parts) == 1 {
		parts = append(parts, "Your message is noted: "+summarizeText(message))
	}
	return strings.Join(parts, "\n")
}

func chatProviderModel(cfg config.ChatConfig) (string, string) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "ollama":
		return "ollama", strings.TrimSpace(cfg.OllamaModel)
	case "openai":
		return "openai", strings.TrimSpace(cfg.OpenAIModel)
	case "openrouter":
		return "openrouter", strings.TrimSpace(cfg.OpenRouterModel)
	default:
		return "none", "retrieval-only"
	}
}

func embeddingModel(cfg config.Config) string {
	switch strings.ToLower(strings.TrimSpace(cfg.Embedding.Provider)) {
	case "openai":
		return strings.TrimSpace(cfg.Embedding.OpenAIModel)
	case "openrouter":
		return strings.TrimSpace(cfg.Embedding.OpenRouterMod)
	default:
		return strings.TrimSpace(cfg.Embedding.OllamaModel)
	}
}

func detectMindDrillSignal(message string, rememberThis bool) (string, float64, []string) {
	normalized := strings.ToLower(strings.TrimSpace(message))
	if rememberThis || strings.Contains(normalized, "remember this") {
		return memory.KindUserPreference, 0.94, []string{"remember-this"}
	}
	if strings.Contains(normalized, "actually") || strings.Contains(normalized, "correction") || strings.Contains(normalized, "i meant") {
		return memory.KindUserPreference, 0.96, []string{"correction"}
	}
	if strings.Contains(normalized, "i prefer") || strings.Contains(normalized, "my preference") || strings.Contains(normalized, "for me") {
		return memory.KindUserPreference, 0.88, []string{"preference"}
	}
	if strings.Contains(normalized, "we decided") || strings.Contains(normalized, "decision") || strings.Contains(normalized, "let's do") {
		return memory.KindProjectDecision, 0.9, []string{"decision"}
	}
	if strings.Contains(normalized, "must") || strings.Contains(normalized, "constraint") || strings.Contains(normalized, "cannot") || strings.Contains(normalized, "can't") {
		return memory.KindTechnicalSolution, 0.9, []string{"constraint"}
	}
	if strings.Contains(normalized, "plan") || strings.Contains(normalized, "next step") || strings.Contains(normalized, "action item") {
		return memory.KindProjectDecision, 0.86, []string{"action-item"}
	}
	if strings.Contains(normalized, "persona") || strings.Contains(normalized, "identity") {
		return memory.KindUserPreference, 0.86, []string{"persona"}
	}
	return "", 0, nil
}

func collectUsedMemoryIDs(front []memory.SearchResult, mind []memory.SearchResult) []string {
	ids := make([]string, 0, len(front)+len(mind))
	for _, item := range front {
		if trimmed := strings.TrimSpace(item.MemoryID); trimmed != "" {
			ids = append(ids, trimmed)
		}
	}
	for _, item := range mind {
		if trimmed := strings.TrimSpace(item.MemoryID); trimmed != "" {
			ids = append(ids, trimmed)
		}
	}
	return uniqueStrings(ids)
}

func formatMemoryList(items []memory.SearchResult) string {
	if len(items) == 0 {
		return "- none retrieved\n"
	}
	var b strings.Builder
	for idx, item := range items {
		b.WriteString(fmt.Sprintf("- [%d] memory_id=%s kind=%s score=%.3f\n", idx+1, strings.TrimSpace(item.MemoryID), strings.TrimSpace(item.MemoryKind), item.Score))
		line := bestMemoryLine(item)
		if strings.TrimSpace(line) != "" {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func bestMemoryLine(item memory.SearchResult) string {
	if quote := strings.TrimSpace(item.SourceQuote); quote != "" {
		return quote
	}
	if summary := strings.TrimSpace(item.Summary); summary != "" {
		return summary
	}
	if text := strings.TrimSpace(item.Text); text != "" {
		return clampRunes(text, 180)
	}
	return ""
}

func summarizeText(text string) string {
	return clampRunes(strings.TrimSpace(text), 220)
}

func clampRunes(text string, size int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= size {
		return string(runes)
	}
	if size <= 3 {
		return string(runes[:size])
	}
	return string(runes[:size-3]) + "..."
}

func sanitizeID(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return "session"
	}
	var b strings.Builder
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}
