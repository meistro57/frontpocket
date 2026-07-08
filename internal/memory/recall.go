package memory

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type MemoryStore interface {
	Upsert(ctx context.Context, points []MemoryPoint) error
	Search(ctx context.Context, req SearchRequest) ([]SearchResult, error)
	Stats(ctx context.Context, filters SearchFilters) (MemoryStats, error)
}

type InMemoryStore struct {
	mu     sync.RWMutex
	points []MemoryPoint
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{points: make([]MemoryPoint, 0)}
}

func (s *InMemoryStore) Upsert(_ context.Context, points []MemoryPoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(points) == 0 {
		return nil
	}

	index := make(map[string]int, len(s.points))
	for i, p := range s.points {
		index[p.MemoryID] = i
	}

	for _, point := range points {
		if idx, ok := index[point.MemoryID]; ok {
			s.points[idx] = point
			continue
		}
		s.points = append(s.points, point)
	}

	return nil
}

func (s *InMemoryStore) Search(_ context.Context, req SearchRequest) ([]SearchResult, error) {
	s.mu.RLock()
	points := append([]MemoryPoint(nil), s.points...)
	s.mu.RUnlock()

	query := strings.TrimSpace(req.Query)
	if query == "" {
		return []SearchResult{}, nil
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 5
	}

	results := make([]SearchResult, 0, len(points))
	for _, p := range points {
		if !matchesFilters(p, req.Filters) {
			continue
		}
		if shouldExcludeByStatus(req.IncludeRejected, p.Status) {
			continue
		}

		score := scoreText(query, p.Text)
		if score <= 0 {
			continue
		}
		score = applyCanonicalBoost(score, p.Canonical, p.Status)

		results = append(results, SearchResult{
			MemoryID:            p.MemoryID,
			ConversationID:      p.ConversationID,
			SessionID:           p.SessionID,
			SourceTitle:         p.SourceTitle,
			SourceType:          p.SourceType,
			Timestamp:           p.Timestamp,
			Speaker:             p.Speaker,
			Project:             p.Project,
			MemoryKind:          p.MemoryKind,
			Tags:                append([]string(nil), p.Tags...),
			Summary:             p.Summary,
			SourceQuote:         p.SourceQuote,
			Text:                p.Text,
			UsedMemoryIDs:       append([]string(nil), p.UsedMemoryIDs...),
			Score:               score,
			EmbeddingProvider:   p.EmbeddingProvider,
			EmbeddingModel:      p.EmbeddingModel,
			EmbeddingDimensions: p.EmbeddingDimensions,
			Canonical:           p.Canonical,
			Confidence:          p.Confidence,
			Status:              p.Status,
			SourceMemoryIDs:     append([]string(nil), p.SourceMemoryIDs...),
			SourceQuotes:        append([]string(nil), p.SourceQuotes...),
			ReviewedAt:          p.ReviewedAt,
			ReviewedBy:          p.ReviewedBy,
			CreatedByLoop:       p.CreatedByLoop,
			Supersedes:          append([]string(nil), p.Supersedes...),
			MergedFrom:          append([]string(nil), p.MergedFrom...),
			ApproximateDate:     p.ApproximateDate,
			DateBasis:           p.DateBasis,
			AIProvider:          p.AIProvider,
			AIModel:             p.AIModel,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Timestamp.After(results[j].Timestamp)
		}
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func (s *InMemoryStore) Stats(_ context.Context, filters SearchFilters) (MemoryStats, error) {
	s.mu.RLock()
	points := append([]MemoryPoint(nil), s.points...)
	s.mu.RUnlock()

	stats := MemoryStats{
		ByKind:    make(map[string]int),
		BySpeaker: make(map[string]int),
		ByProject: make(map[string]int),
	}

	const maxDistinctTitles = 60
	titleSeen := make(map[string]struct{})
	titles := make([]string, 0, maxDistinctTitles)

	for _, point := range points {
		if !matchesFilters(point, filters) {
			continue
		}
		stats.Total++

		if kind := strings.TrimSpace(point.MemoryKind); kind != "" {
			stats.ByKind[kind]++
		}
		if speaker := strings.TrimSpace(point.Speaker); speaker != "" {
			stats.BySpeaker[speaker]++
		}
		if project := strings.TrimSpace(point.Project); project != "" {
			stats.ByProject[project]++
		}
		if title := strings.TrimSpace(point.SourceTitle); title != "" {
			if _, seen := titleSeen[title]; !seen && len(titles) < maxDistinctTitles {
				titleSeen[title] = struct{}{}
				titles = append(titles, title)
			}
		}
	}

	stats.TopTitles = titles

	return stats, nil
}

func (s *InMemoryStore) DeleteByFilters(filters SearchFilters) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.points) == 0 {
		return nil
	}

	filtered := make([]MemoryPoint, 0, len(s.points))
	for _, point := range s.points {
		if matchesFilters(point, filters) {
			continue
		}
		filtered = append(filtered, point)
	}
	s.points = filtered
	return nil
}

func (s *InMemoryStore) ScrollRaw(_ context.Context, limit int, offset string, filters SearchFilters, since, until time.Time, includeCanonical bool) ([]MemoryPoint, string, error) {
	s.mu.RLock()
	points := append([]MemoryPoint(nil), s.points...)
	s.mu.RUnlock()

	if limit <= 0 {
		limit = 128
	}
	start := 0
	if trimmed := strings.TrimSpace(offset); trimmed != "" {
		if parsed, err := strconv.Atoi(trimmed); err == nil && parsed >= 0 {
			start = parsed
		}
	}

	filtered := make([]MemoryPoint, 0, len(points))
	for _, point := range points {
		if !matchesFilters(point, filters) {
			continue
		}
		if !includeCanonical && point.Canonical {
			continue
		}
		if !since.IsZero() && point.Timestamp.Before(since) {
			continue
		}
		if !until.IsZero() && point.Timestamp.After(until) {
			continue
		}
		filtered = append(filtered, point)
	}
	if start >= len(filtered) {
		return []MemoryPoint{}, "", nil
	}
	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	next := ""
	if end < len(filtered) {
		next = strconv.Itoa(end)
	}
	return filtered[start:end], next, nil
}

func scoreText(query, text string) float64 {
	qTokens := tokens(query)
	if len(qTokens) == 0 {
		return 0
	}
	tTokens := tokens(text)
	if len(tTokens) == 0 {
		return 0
	}

	tSet := make(map[string]struct{}, len(tTokens))
	for _, token := range tTokens {
		tSet[token] = struct{}{}
	}

	matches := 0
	for _, token := range qTokens {
		if _, ok := tSet[token]; ok {
			matches++
		}
	}

	if matches == 0 {
		return 0
	}

	return float64(matches) / float64(len(qTokens))
}

func matchesFilters(point MemoryPoint, filters SearchFilters) bool {
	if filters.Project != "" && !strings.EqualFold(point.Project, filters.Project) {
		return false
	}
	if filters.MemoryKind != "" && !strings.EqualFold(point.MemoryKind, filters.MemoryKind) {
		return false
	}
	if filters.Speaker != "" && !strings.EqualFold(point.Speaker, filters.Speaker) {
		return false
	}
	if filters.SourceType != "" && !strings.EqualFold(point.SourceType, filters.SourceType) {
		return false
	}
	if filters.ConversationID != "" && !strings.EqualFold(point.ConversationID, filters.ConversationID) {
		return false
	}
	if filters.AIProvider != "" && !strings.EqualFold(point.AIProvider, filters.AIProvider) {
		return false
	}
	if filters.AIModel != "" && !strings.EqualFold(point.AIModel, filters.AIModel) {
		return false
	}
	if filters.UserStarred != nil && point.UserStarred != *filters.UserStarred {
		return false
	}
	if filters.UserShared != nil && point.UserShared != *filters.UserShared {
		return false
	}
	if filters.FeedbackRating != "" && !strings.EqualFold(point.FeedbackRating, filters.FeedbackRating) {
		return false
	}
	if filters.HasAttachment != nil {
		hasAttachment := strings.TrimSpace(point.AttachmentSourceSystem) != ""
		if hasAttachment != *filters.HasAttachment {
			return false
		}
	}
	if len(filters.Tags) == 0 {
		return true
	}

	available := make(map[string]struct{}, len(point.Tags))
	for _, tag := range point.Tags {
		available[strings.ToLower(strings.TrimSpace(tag))] = struct{}{}
	}
	for _, required := range filters.Tags {
		if _, ok := available[strings.ToLower(strings.TrimSpace(required))]; !ok {
			return false
		}
	}

	return true
}

func tokens(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, ",", " ")
	s = strings.ReplaceAll(s, ".", " ")
	s = strings.ReplaceAll(s, ":", " ")
	s = strings.ReplaceAll(s, ";", " ")
	s = strings.ReplaceAll(s, "\"", " ")
	s = strings.ReplaceAll(s, "'", " ")
	raw := strings.Fields(s)
	if len(raw) == 0 {
		return nil
	}

	out := make([]string, 0, len(raw))
	for _, token := range raw {
		if token != "" {
			out = append(out, token)
		}
	}
	return out
}

func applyCanonicalBoost(score float64, canonical bool, status string) float64 {
	boost := 1.0
	if canonical {
		boost *= 1.25
	}
	switch strings.TrimSpace(status) {
	case StatusApprovedByUser, StatusDirectUserStatement:
		boost *= 1.2
	case StatusInferredFromSources:
		boost *= 1.08
	case StatusNeedsReview:
		boost *= 0.98
	}
	return score * boost
}

func shouldExcludeByStatus(includeRejected bool, status string) bool {
	status = strings.TrimSpace(status)
	if status == "" {
		return false
	}
	if includeRejected {
		return false
	}
	switch status {
	case StatusRejected, StatusContradicted, StatusOutdated:
		return true
	default:
		return false
	}
}
