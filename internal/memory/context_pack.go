package memory

import (
	"context"
	"sort"
	"strings"
)

const (
	ContextPackOrderRelevance     = "relevance"
	ContextPackOrderChronological = "chronological"
)

// ContextPackOptions controls how retrieved memory is assembled into a pack.
type ContextPackOptions struct {
	// MaxCharacters caps the rendered size of the pack. Zero means unlimited.
	MaxCharacters int
	// QuoteOnly drops full text, keeping only the source quote for each memory.
	QuoteOnly bool
	// Chronological orders the pack oldest-first instead of by score.
	Chronological bool
}

// ContextPackBudget reports how much of the requested budget was used.
type ContextPackBudget struct {
	CharacterLimit  int `json:"character_limit,omitempty"`
	CharactersUsed  int `json:"characters_used"`
	ResultsIncluded int `json:"results_included"`
	Dropped         int `json:"dropped,omitempty"`
}

type ContextPacker struct {
	Store MemoryStore
}

func (c ContextPacker) Build(ctx context.Context, req ContextPackRequest) (ContextPackResponse, error) {
	searchReq := SearchRequest{
		Query:              req.Query,
		Limit:              req.Limit,
		Filters:            req.Filters,
		IncludeProposed:    req.IncludeProposed,
		IncludeRejected:    req.IncludeRejected,
		CanonicalFirst:     req.CanonicalFirst,
		Hybrid:             req.Hybrid,
		Dedupe:             req.Dedupe,
		MinScore:           req.MinScore,
		MaxPerConversation: req.MaxPerConversation,
	}
	results, err := c.Store.Search(ctx, searchReq)
	if err != nil {
		return ContextPackResponse{}, err
	}
	return AssembleContextPack(req.Query, results, ContextPackOptions{
		MaxCharacters: req.MaxCharacters,
		QuoteOnly:     req.QuoteOnly,
		Chronological: strings.EqualFold(strings.TrimSpace(req.Order), ContextPackOrderChronological),
	}), nil
}

// packItemCharacters estimates the rendered size of one memory entry, including
// the metadata a caller needs in order to cite it.
func packItemCharacters(result SearchResult) int {
	size := len(result.SourceQuote) + len(result.Text) + len(result.SourceTitle) + len(result.MemoryID)
	size += len(result.Project) + len(result.Speaker) + len(result.SourceType)
	return size
}

// AssembleContextPack builds a bounded, de-duplicated pack from search results.
// It preserves relevance order by default, drops entries that would exceed the
// character budget, and reports what it kept so callers can trust the result.
func AssembleContextPack(query string, results []SearchResult, opts ContextPackOptions) ContextPackResponse {
	packed := make([]SearchResult, 0, len(results))
	for _, result := range results {
		if opts.QuoteOnly {
			result.Text = ""
			result.Summary = ""
		}
		packed = append(packed, result)
	}

	if opts.Chronological {
		sort.SliceStable(packed, func(i, j int) bool {
			return packed[i].Timestamp.Before(packed[j].Timestamp)
		})
	}

	budget := &ContextPackBudget{CharacterLimit: opts.MaxCharacters}
	kept := make([]SearchResult, 0, len(packed))
	usedIDs := make([]string, 0, len(packed))
	for _, result := range packed {
		cost := packItemCharacters(result)
		if opts.MaxCharacters > 0 && budget.CharactersUsed+cost > opts.MaxCharacters {
			budget.Dropped++
			continue
		}
		budget.CharactersUsed += cost
		kept = append(kept, result)
		usedIDs = append(usedIDs, result.MemoryID)
	}
	budget.ResultsIncluded = len(kept)

	return ContextPackResponse{
		Query:         query,
		MemoryPack:    kept,
		UsedMemoryIDs: usedIDs,
		Budget:        budget,
	}
}
