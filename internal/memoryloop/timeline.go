package memoryloop

import (
	"sort"
	"strings"
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
)

type TimelineBuilder struct{}

func (b TimelineBuilder) Build(points []memory.MemoryPoint) []Candidate {
	if len(points) == 0 {
		return nil
	}
	sorted := append([]memory.MemoryPoint(nil), points...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})
	first := sorted[0]
	last := sorted[len(sorted)-1]
	now := time.Now().UTC()
	return []Candidate{
		{
			ID:              candidateID("timeline_first", first.MemoryID+first.Timestamp.Format(time.RFC3339)),
			Summary:         "First known memory mention appears around " + first.Timestamp.Format("2006-01-02") + ", based on message metadata.",
			MemoryKind:      memory.KindCanonicalTimeline,
			Confidence:      memory.ConfidenceMedium,
			Status:          memory.StatusInferredFromSources,
			Canonical:       false,
			SourceMemoryIDs: []string{first.MemoryID},
			SourceQuotes:    []string{clampRunes(bestQuote(first), 180)},
			Project:         strings.TrimSpace(first.Project),
			Speaker:         strings.TrimSpace(first.Speaker),
			ApproximateDate: first.Timestamp.Format("2006-01-02"),
			DateBasis:       "message metadata",
			CreatedAt:       now,
			UpdatedAt:       now,
			CreatedByLoop:   true,
		},
		{
			ID:              candidateID("timeline_current", last.MemoryID+last.Timestamp.Format(time.RFC3339)),
			Summary:         "Current direction is reflected in later memory records near " + last.Timestamp.Format("2006-01-02") + ".",
			MemoryKind:      memory.KindCanonicalTimeline,
			Confidence:      memory.ConfidenceMedium,
			Status:          memory.StatusNeedsReview,
			Canonical:       false,
			SourceMemoryIDs: []string{last.MemoryID},
			SourceQuotes:    []string{clampRunes(bestQuote(last), 180)},
			Project:         strings.TrimSpace(last.Project),
			Speaker:         strings.TrimSpace(last.Speaker),
			ApproximateDate: last.Timestamp.Format("2006-01-02"),
			DateBasis:       "message metadata",
			CreatedAt:       now,
			UpdatedAt:       now,
			CreatedByLoop:   true,
		},
	}
}

func bestQuote(point memory.MemoryPoint) string {
	if quote := strings.TrimSpace(point.SourceQuote); quote != "" {
		return quote
	}
	if summary := strings.TrimSpace(point.Summary); summary != "" {
		return summary
	}
	return strings.TrimSpace(point.Text)
}
