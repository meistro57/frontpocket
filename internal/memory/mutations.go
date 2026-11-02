package memory

// PinRequest marks or unmarks memories as pinned. Pinned memories are intended
// to survive cleanup or forgetting passes by default.
type PinRequest struct {
	MemoryIDs []string `json:"memory_ids"`
	Pinned    *bool    `json:"pinned,omitempty"`
	Reason    string   `json:"reason,omitempty"`
	Actor     string   `json:"actor,omitempty"`
}

// CorrectRequest rewrites user-visible fields of an existing memory while
// leaving the original source material in place.
type CorrectRequest struct {
	MemoryID    string   `json:"memory_id"`
	Text        *string  `json:"text,omitempty"`
	Summary     *string  `json:"summary,omitempty"`
	SourceQuote *string  `json:"source_quote,omitempty"`
	MemoryKind  *string  `json:"memory_kind,omitempty"`
	Project     *string  `json:"project,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Reason      string   `json:"reason,omitempty"`
	Actor       string   `json:"actor,omitempty"`
}

// ForgetRequest soft-deletes memories (status=deleted) unless Hard is set, in
// which case the points are removed from the store.
type ForgetRequest struct {
	MemoryIDs []string `json:"memory_ids"`
	Hard      bool     `json:"hard,omitempty"`
	Reason    string   `json:"reason,omitempty"`
	Actor     string   `json:"actor,omitempty"`
}

// MutationRecord is appended to the audit log for every accepted mutation.
type MutationRecord struct {
	Timestamp string   `json:"timestamp"`
	Action    string   `json:"action"`
	Actor     string   `json:"actor"`
	MemoryIDs []string `json:"memory_ids"`
	Fields    []string `json:"fields,omitempty"`
	Reason    string   `json:"reason,omitempty"`
	Hard      bool     `json:"hard,omitempty"`
}
