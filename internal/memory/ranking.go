package memory

import (
	"sort"
	"strings"
)

// hybridRankConstant dampens the contribution of very high ranks, following the
// reciprocal-rank-fusion convention so dense and lexical orders fuse stably.
const hybridRankConstant = 60.0

// LexicalScore reports how much of the query appears in a result's own text.
// It complements dense similarity, which tends to miss exact names, quotes, and
// code symbols.
func LexicalScore(query string, result SearchResult) float64 {
	qTokens := lexicalTokens(query)
	if len(qTokens) == 0 {
		return 0
	}

	fields := []string{result.Text, result.SourceQuote, result.Summary, result.SourceTitle}
	seen := make(map[string]struct{}, len(qTokens))
	for _, field := range fields {
		for _, token := range lexicalTokens(field) {
			seen[token] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return 0
	}

	matches := 0
	for _, token := range qTokens {
		if _, ok := seen[token]; ok {
			matches++
		}
	}
	if matches == 0 {
		return 0
	}
	return float64(matches) / float64(len(qTokens))
}

// lexicalTokens trims wrapping punctuation so markup like `code` or (aside)
// still matches the bare word, while keeping inner punctuation for symbols.
func lexicalTokens(s string) []string {
	raw := tokens(s)
	out := make([]string, 0, len(raw))
	for _, token := range raw {
		trimmed := strings.Trim(token, "`()[]{}<>\"*#>|/\\")
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// FuseHybridFusion re-ranks dense candidates by blending dense order with
// lexical order. Candidates with no lexical overlap keep their dense order but
// rank below any candidate that does overlap, so exact-term matches surface
// without discarding semantically relevant results.
func FuseHybridResults(query string, candidates []SearchResult, limit int) []SearchResult {
	if len(candidates) == 0 {
		return candidates
	}

	scored := make([]SearchResult, len(candidates))
	copy(scored, candidates)

	denseOrder := make([]int, len(scored))
	for i := range denseOrder {
		denseOrder[i] = i
	}
	sort.SliceStable(denseOrder, func(a, b int) bool {
		return scored[denseOrder[a]].Score > scored[denseOrder[b]].Score
	})

	denseRank := make([]int, len(scored))
	for rank, idx := range denseOrder {
		denseRank[idx] = rank
	}

	lexOrder := make([]int, len(scored))
	for i := range lexOrder {
		lexOrder[i] = i
	}
	for i := range scored {
		scored[i].LexicalScore = LexicalScore(query, scored[i])
	}
	sort.SliceStable(lexOrder, func(a, b int) bool {
		return scored[lexOrder[a]].LexicalScore > scored[lexOrder[b]].LexicalScore
	})

	lexRank := make([]int, len(scored))
	for rank, idx := range lexOrder {
		lexRank[idx] = rank
	}

	raw := make([]float64, len(scored))
	maxRaw := 0.0
	for i := range scored {
		fused := 1.0 / (hybridRankConstant + float64(denseRank[i]) + 1)
		if scored[i].LexicalScore > 0 {
			fused += 1.0 / (hybridRankConstant + float64(lexRank[i]) + 1)
		}
		raw[i] = fused
		if fused > maxRaw {
			maxRaw = fused
		}
	}

	for i := range scored {
		if maxRaw > 0 {
			scored[i].Score = raw[i] / maxRaw
		}
	}

	sort.SliceStable(scored, func(a, b int) bool {
		if scored[a].Score == scored[b].Score {
			return scored[a].Timestamp.After(scored[b].Timestamp)
		}
		return scored[a].Score > scored[b].Score
	})

	if limit > 0 && len(scored) > limit {
		return scored[:limit]
	}
	return scored
}

// normaliseDedupeText produces the comparison key used to collapse duplicate
// memory points, which usually come from overlapping chunks or repeated imports.
func normaliseDedupeText(text string) string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(text)))
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}

// DedupeResults collapses results that carry the same text inside the same
// conversation, keeping the highest-scoring representative and recording the
// collapsed IDs. maxPerConversation limits how much of one conversation can
// occupy the result set (0 disables the cap).
func DedupeResults(results []SearchResult, maxPerConversation int) []SearchResult {
	if len(results) == 0 {
		return results
	}

	out := make([]SearchResult, 0, len(results))
	seenIndex := make(map[string]int, len(results))
	for _, result := range results {
		key := normaliseDedupeText(result.Text)
		if key == "" {
			key = normaliseDedupeText(result.SourceQuote)
		}
		if key != "" {
			groupKey := result.ConversationID + "\x00" + key
			if idx, ok := seenIndex[groupKey]; ok {
				out[idx].DuplicateIDs = append(out[idx].DuplicateIDs, result.MemoryID)
				out[idx].DuplicateCount = len(out[idx].DuplicateIDs)
				continue
			}
			seenIndex[groupKey] = len(out)
		}
		out = append(out, result)
	}

	if maxPerConversation > 0 {
		capped := make([]SearchResult, 0, len(out))
		perConversation := make(map[string]int, len(out))
		for _, result := range out {
			conversation := result.ConversationID
			if conversation != "" {
				if perConversation[conversation] >= maxPerConversation {
					continue
				}
				perConversation[conversation]++
			}
			capped = append(capped, result)
		}
		out = capped
	}

	return out
}

// ApplyMinScore drops results below a score floor.
func ApplyMinScore(results []SearchResult, minScore float64) []SearchResult {
	if minScore <= 0 || len(results) == 0 {
		return results
	}
	filtered := make([]SearchResult, 0, len(results))
	for _, result := range results {
		if result.Score < minScore {
			continue
		}
		filtered = append(filtered, result)
	}
	return filtered
}
