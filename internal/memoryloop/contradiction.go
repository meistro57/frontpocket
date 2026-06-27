package memoryloop

import (
	"strings"

	"github.com/meistro57/frontpocket/internal/memory"
)

type ContradictionScout struct{}

func (c ContradictionScout) Check(candidates []Candidate) ([]Candidate, int) {
	if len(candidates) == 0 {
		return nil, 0
	}
	byProjectKind := make(map[string]Candidate)
	out := make([]Candidate, 0, len(candidates))
	conflicts := 0
	for _, candidate := range candidates {
		key := strings.ToLower(strings.TrimSpace(candidate.Project + "|" + candidate.MemoryKind))
		existing, ok := byProjectKind[key]
		if !ok {
			byProjectKind[key] = candidate
			out = append(out, candidate)
			continue
		}
		if strings.EqualFold(existing.Summary, candidate.Summary) {
			continue
		}
		candidate.Status = memory.StatusNeedsReview
		if candidate.MemoryKind != memory.KindContradictionNote {
			candidate.MemoryKind = memory.KindContradictionNote
		}
		candidate.Confidence = memory.ConfidenceMedium
		candidate.SourceQuotes = unique(append(candidate.SourceQuotes, existing.SourceQuotes...))
		candidate.SourceMemoryIDs = unique(append(candidate.SourceMemoryIDs, existing.SourceMemoryIDs...))
		out = append(out, candidate)
		conflicts++
	}
	return out, conflicts
}
