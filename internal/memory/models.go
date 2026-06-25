package memory

import "time"

const (
	KindFact              = "fact"
	KindPreference        = "preference"
	KindProjectContext    = "project_context"
	KindDecision          = "decision"
	KindTechnicalSolution = "technical_solution"
	KindCreativeArtifact  = "creative_artifact"
	KindRunningJoke       = "running_joke"
	KindPersonalContext   = "personal_context"
	KindRelationship      = "relationship_context"
	KindResearchNote      = "research_note"
	KindSystemNote        = "system_note"
	KindChatTurn          = "chat_turn"
	KindSessionSummary    = "session_summary"
	KindUserPreference    = "user_preference"
	KindProjectDecision   = "project_decision"
	KindToolResult        = "tool_result"
)

type MessageRecord struct {
	ConversationID string   `json:"conversation_id"`
	Timestamp      string   `json:"timestamp"`
	Speaker        string   `json:"speaker"`
	Text           string   `json:"text"`
	SourceType     string   `json:"source_type,omitempty"`
	SourceTitle    string   `json:"source_title,omitempty"`
	Project        string   `json:"project,omitempty"`
	MemoryKind     string   `json:"memory_kind,omitempty"`
	Tags           []string `json:"tags,omitempty"`
}

type MemoryPoint struct {
	MemoryID            string    `json:"memory_id"`
	ConversationID      string    `json:"conversation_id"`
	SessionID           string    `json:"session_id,omitempty"`
	SourceType          string    `json:"source_type"`
	SourceTitle         string    `json:"source_title"`
	Timestamp           time.Time `json:"timestamp"`
	Speaker             string    `json:"speaker"`
	Project             string    `json:"project,omitempty"`
	Tags                []string  `json:"tags,omitempty"`
	MemoryKind          string    `json:"memory_kind"`
	Importance          float64   `json:"importance,omitempty"`
	Text                string    `json:"text"`
	SourceQuote         string    `json:"source_quote"`
	Summary             string    `json:"summary,omitempty"`
	UsedMemoryIDs       []string  `json:"used_memory_ids,omitempty"`
	EmbeddingProvider   string    `json:"embedding_provider"`
	EmbeddingModel      string    `json:"embedding_model"`
	EmbeddingDimensions int       `json:"embedding_dimensions"`
	Vector              []float32 `json:"-"`
}

type SearchFilters struct {
	Project        string   `json:"project,omitempty"`
	MemoryKind     string   `json:"memory_kind,omitempty"`
	Speaker        string   `json:"speaker,omitempty"`
	SourceType     string   `json:"source_type,omitempty"`
	ConversationID string   `json:"conversation_id,omitempty"`
	Tags           []string `json:"tags,omitempty"`
}

type SearchRequest struct {
	Query   string        `json:"query"`
	Limit   int           `json:"limit,omitempty"`
	Filters SearchFilters `json:"filters,omitempty"`
}

type SearchResult struct {
	MemoryID            string    `json:"memory_id"`
	ConversationID      string    `json:"conversation_id"`
	SessionID           string    `json:"session_id,omitempty"`
	SourceTitle         string    `json:"source_title"`
	SourceType          string    `json:"source_type"`
	Timestamp           time.Time `json:"timestamp"`
	Speaker             string    `json:"speaker"`
	Project             string    `json:"project,omitempty"`
	MemoryKind          string    `json:"memory_kind"`
	Tags                []string  `json:"tags,omitempty"`
	Summary             string    `json:"summary"`
	SourceQuote         string    `json:"source_quote,omitempty"`
	Text                string    `json:"text,omitempty"`
	UsedMemoryIDs       []string  `json:"used_memory_ids,omitempty"`
	Score               float64   `json:"score"`
	EmbeddingProvider   string    `json:"embedding_provider"`
	EmbeddingModel      string    `json:"embedding_model"`
	EmbeddingDimensions int       `json:"embedding_dimensions"`
}

type SearchResponse struct {
	Query   string         `json:"query"`
	Results []SearchResult `json:"results"`
}

type ContextPackRequest struct {
	Query   string        `json:"query"`
	Limit   int           `json:"limit,omitempty"`
	Filters SearchFilters `json:"filters,omitempty"`
}

type ContextPackResponse struct {
	Query      string         `json:"query"`
	MemoryPack []SearchResult `json:"memory_pack"`
}

type IngestChatRequest struct {
	SourceTitle string          `json:"source_title,omitempty"`
	SourceType  string          `json:"source_type,omitempty"`
	Project     string          `json:"project,omitempty"`
	Records     []MessageRecord `json:"records,omitempty"`
	JSONL       string          `json:"jsonl,omitempty"`
}

type IngestChatResponse struct {
	InsertedCount int      `json:"inserted_count"`
	MemoryIDs     []string `json:"memory_ids"`
}

type MemoryStats struct {
	Total     int            `json:"total"`
	ByKind    map[string]int `json:"by_kind,omitempty"`
	BySpeaker map[string]int `json:"by_speaker,omitempty"`
	ByProject map[string]int `json:"by_project,omitempty"`
}

type SessionRequest struct {
	SessionID       string            `json:"session_id"`
	Project         string            `json:"project,omitempty"`
	ActiveSummary   string            `json:"active_summary,omitempty"`
	RecentMemoryIDs []string          `json:"recent_memory_ids,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	TTLSeconds      int               `json:"ttl_seconds,omitempty"`
	LoadOnly        bool              `json:"load_only,omitempty"`
}

type SessionState struct {
	SessionID       string            `json:"session_id"`
	Project         string            `json:"project,omitempty"`
	ActiveSummary   string            `json:"active_summary,omitempty"`
	RecentMemoryIDs []string          `json:"recent_memory_ids,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

type SessionResponse struct {
	Found bool          `json:"found"`
	State *SessionState `json:"state,omitempty"`
}

// ChatSessionDeleteResponse reports the outcome of clearing a MindDrill chat
// session from fast session state and durable MindDrill chat memory.
type ChatSessionDeleteResponse struct {
	SessionID      string `json:"session_id"`
	SessionCleared bool   `json:"session_cleared"`
	MemoryCleared  bool   `json:"memory_cleared"`
}

type ChatMessageRequest struct {
	SessionID       string `json:"session_id"`
	Message         string `json:"message"`
	Project         string `json:"project,omitempty"`
	FrontPocketTopK int    `json:"frontpocket_top_k,omitempty"`
	MindDrillTopK   int    `json:"minddrill_top_k,omitempty"`
	RememberThis    bool   `json:"remember_this,omitempty"`
}

type ChatMessageResponse struct {
	Answer                  string         `json:"answer"`
	UsedFrontPocketMemories []SearchResult `json:"used_frontpocket_memories"`
	UsedMindDrillMemories   []SearchResult `json:"used_minddrill_memories"`
	ContextPack             string         `json:"context_pack"`
	Model                   string         `json:"model"`
	Provider                string         `json:"provider"`
}

type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}
