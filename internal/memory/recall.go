package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
)

type MemoryStore interface {
	Upsert(ctx context.Context, points []MemoryPoint) error
	Search(ctx context.Context, req SearchRequest) ([]SearchResult, error)
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

		score := scoreText(query, p.Text)
		if score <= 0 {
			continue
		}

		results = append(results, SearchResult{
			MemoryID:            p.MemoryID,
			ConversationID:      p.ConversationID,
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
			Score:               score,
			EmbeddingProvider:   p.EmbeddingProvider,
			EmbeddingModel:      p.EmbeddingModel,
			EmbeddingDimensions: p.EmbeddingDimensions,
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
