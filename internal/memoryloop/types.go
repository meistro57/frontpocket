package memoryloop

import (
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
)

type Candidate struct {
	ID              string     `json:"id"`
	Summary         string     `json:"summary"`
	MemoryKind      string     `json:"memory_kind"`
	Confidence      string     `json:"confidence"`
	Status          string     `json:"status"`
	Canonical       bool       `json:"canonical"`
	SourceMemoryIDs []string   `json:"source_memory_ids"`
	SourceQuotes    []string   `json:"source_quotes"`
	Tags            []string   `json:"tags,omitempty"`
	Project         string     `json:"project,omitempty"`
	Speaker         string     `json:"speaker,omitempty"`
	ApproximateDate string     `json:"approximate_date,omitempty"`
	DateBasis       string     `json:"date_basis,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	ReviewedAt      *time.Time `json:"reviewed_at,omitempty"`
	ReviewedBy      string     `json:"reviewed_by,omitempty"`
	CreatedByLoop   bool       `json:"created_by_loop"`
	RejectionReason string     `json:"rejection_reason,omitempty"`
	MergeTargetID   string     `json:"merge_target_id,omitempty"`
	Supersedes      []string   `json:"supersedes,omitempty"`
	MergedFrom      []string   `json:"merged_from,omitempty"`
	Text            string     `json:"text,omitempty"`
}

type HarvestFilter struct {
	Project            string
	Speaker            string
	MemoryKind         string
	Since              time.Time
	Until              time.Time
	BatchSize          int
	Limit              int
	CanonicalOnly      bool
	IncludeExistingCanon bool
}

type Cluster struct {
	ClusterID string
	Label     string
	Points    []memory.MemoryPoint
}

type LoopResult struct {
	Candidates          []Candidate
	SkippedDuplicates   int
	ContradictionMarked int
}
