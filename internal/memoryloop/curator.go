package memoryloop

import (
	"strings"
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
)

type CanonCurator struct{}

func (c CanonCurator) Curate(candidates []Candidate, existingCanonical []memory.SearchResult) (curated []Candidate, skipped int) {
	seen := make(map[string]struct{}, len(existingCanonical)+len(candidates))
	for _, item := range existingCanonical {
		key := dedupeKey(item.Summary, item.Project, item.MemoryKind)
		seen[key] = struct{}{}
	}

	curated = make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		candidate.SourceMemoryIDs = unique(candidate.SourceMemoryIDs)
		candidate.SourceQuotes = unique(candidate.SourceQuotes)
		if len(candidate.SourceMemoryIDs) == 0 || len(candidate.SourceQuotes) == 0 {
			skipped++
			continue
		}
		if strings.TrimSpace(candidate.Status) == "" {
			candidate.Status = memory.StatusNeedsReview
		}
		if strings.TrimSpace(candidate.Confidence) == "" {
			candidate.Confidence = memory.ConfidenceLow
		}
		if candidate.Status == memory.StatusDirectUserStatement && !strings.EqualFold(strings.TrimSpace(candidate.Speaker), "user") {
			candidate.Status = memory.StatusInferredFromSources
		}
		if candidate.CreatedAt.IsZero() {
			candidate.CreatedAt = time.Now().UTC()
		}
		candidate.UpdatedAt = time.Now().UTC()
		candidate.CreatedByLoop = true

		key := dedupeKey(candidate.Summary, candidate.Project, candidate.MemoryKind)
		if _, ok := seen[key]; ok {
			skipped++
			continue
		}
		seen[key] = struct{}{}
		curated = append(curated, candidate)
	}
	return curated, skipped
}

func dedupeKey(summary, project, kind string) string {
	return strings.ToLower(strings.TrimSpace(project + "|" + kind + "|" + summary))
}
