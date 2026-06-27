package memoryloop

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
)

type FactExtractor struct{}

func (e FactExtractor) Extract(cluster Cluster) []Candidate {
	if len(cluster.Points) == 0 {
		return nil
	}
	first := cluster.Points[0]
	summary := clusterSummary(cluster.Points)
	kind := classifyKind(cluster)
	status := inferStatus(cluster.Points)
	confidence := inferConfidence(cluster.Points)

	sourceIDs := make([]string, 0, len(cluster.Points))
	sourceQuotes := make([]string, 0, len(cluster.Points))
	tags := make([]string, 0)
	for _, point := range cluster.Points {
		sourceIDs = append(sourceIDs, strings.TrimSpace(point.MemoryID))
		quote := strings.TrimSpace(point.SourceQuote)
		if quote == "" {
			quote = strings.TrimSpace(point.Summary)
		}
		if quote == "" {
			quote = strings.TrimSpace(point.Text)
		}
		if quote != "" {
			sourceQuotes = append(sourceQuotes, clampRunes(quote, 180))
		}
		tags = append(tags, point.Tags...)
	}
	sourceIDs = unique(sourceIDs)
	sourceQuotes = unique(sourceQuotes)
	tags = unique(tags)

	now := time.Now().UTC()
	candidate := Candidate{
		ID:              candidateID(cluster.ClusterID, summary),
		Summary:         summary,
		MemoryKind:      kind,
		Confidence:      confidence,
		Status:          status,
		Canonical:       false,
		SourceMemoryIDs: sourceIDs,
		SourceQuotes:    sourceQuotes,
		Tags:            tags,
		Project:         strings.TrimSpace(first.Project),
		Speaker:         strings.TrimSpace(first.Speaker),
		CreatedAt:       now,
		UpdatedAt:       now,
		CreatedByLoop:   true,
		Text:            summary,
	}
	return []Candidate{candidate}
}

func clusterSummary(points []memory.MemoryPoint) string {
	sort.Slice(points, func(i, j int) bool {
		return points[i].Timestamp.Before(points[j].Timestamp)
	})
	parts := make([]string, 0, 3)
	for _, point := range points {
		line := strings.TrimSpace(point.Summary)
		if line == "" {
			line = strings.TrimSpace(point.SourceQuote)
		}
		if line == "" {
			line = strings.TrimSpace(point.Text)
		}
		if line == "" {
			continue
		}
		parts = append(parts, clampRunes(line, 160))
		if len(parts) == 3 {
			break
		}
	}
	if len(parts) == 0 {
		return "No reliable source-backed summary available."
	}
	return strings.Join(parts, " ")
}

func classifyKind(cluster Cluster) string {
	lowerLabel := strings.ToLower(cluster.Label)
	switch {
	case strings.Contains(lowerLabel, "project_decision") || strings.Contains(lowerLabel, "decision"):
		return memory.KindProjectDecision
	case strings.Contains(lowerLabel, "preference") || strings.Contains(lowerLabel, "user_preference"):
		return memory.KindPreference
	case strings.Contains(lowerLabel, "persona"):
		return memory.KindPersonaInstruction
	default:
		return memory.KindInferredPattern
	}
}

func inferStatus(points []memory.MemoryPoint) string {
	for _, point := range points {
		if strings.EqualFold(strings.TrimSpace(point.Speaker), "user") {
			return memory.StatusDirectUserStatement
		}
	}
	return memory.StatusInferredFromSources
}

func inferConfidence(points []memory.MemoryPoint) string {
	if len(points) >= 4 {
		return memory.ConfidenceHigh
	}
	if len(points) >= 2 {
		return memory.ConfidenceMedium
	}
	return memory.ConfidenceLow
}

func candidateID(clusterID, summary string) string {
	hash := sha1.Sum([]byte(clusterID + "|" + summary))
	return fmt.Sprintf("cand_%s", hex.EncodeToString(hash[:8]))
}

func unique(items []string) []string {
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
