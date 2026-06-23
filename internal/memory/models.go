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

type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}
