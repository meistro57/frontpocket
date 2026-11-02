package tools

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

func classifyEvidenceForHypotheses(hypotheses []DeepDrillHypothesisState, evidence []ResearchResult) (supportByID, counterByID, ambiguousByID map[string][]ResearchResult) {
	supportByID = make(map[string][]ResearchResult)
	counterByID = make(map[string][]ResearchResult)
	ambiguousByID = make(map[string][]ResearchResult)
	for _, item := range evidence {
		for _, hypothesis := range hypotheses {
			classification := classifyEvidenceForHypothesis(hypothesis, item)
			switch classification.Polarity {
			case deepDrillPolaritySupport:
				supportByID[hypothesis.ID] = append(supportByID[hypothesis.ID], item)
			case deepDrillPolarityCounter:
				counterByID[hypothesis.ID] = append(counterByID[hypothesis.ID], item)
			case deepDrillPolarityAmbiguous:
				ambiguousByID[hypothesis.ID] = append(ambiguousByID[hypothesis.ID], item)
			}
		}
	}
	return supportByID, counterByID, ambiguousByID
}

func buildEvidenceEnvelope(item ResearchResult) deepDrillEvidenceEnvelope {
	fields := make([]string, 0, 7)
	parts := make([]string, 0, 7)
	add := func(name, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		fields = append(fields, name)
		parts = append(parts, value)
	}
	add("text", item.Text)
	add("source_quote", item.SourceQuote)
	add("summary", item.Summary)
	add("source_title", item.SourceTitle)
	add("filename", item.Filename)
	add("path", item.Path)
	add("section_or_heading", item.SectionOrHeading)
	return deepDrillEvidenceEnvelope{fields: fields, body: strings.Join(parts, " ")}
}

func normalizeConcepts(text string) []string {
	tokens := tokenizeMeaningful(text)
	out := make([]string, 0, len(tokens))
	seen := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		concept := token
		if canonical, ok := deepDrillConceptCanonical[token]; ok {
			concept = canonical
		}
		if _, ok := seen[concept]; ok {
			continue
		}
		seen[concept] = struct{}{}
		out = append(out, concept)
	}
	return out
}

func rawLexicalOverlap(statement, evidenceText string) float64 {
	statementTokens := tokenizeMeaningful(statement)
	if len(statementTokens) == 0 {
		return 0
	}
	evidenceTokens := tokenizeMeaningful(evidenceText)
	if len(evidenceTokens) == 0 {
		return 0
	}
	evidenceSet := make(map[string]struct{}, len(evidenceTokens))
	for _, token := range evidenceTokens {
		evidenceSet[token] = struct{}{}
	}
	matches := 0
	for _, token := range statementTokens {
		if _, ok := evidenceSet[token]; ok {
			matches++
		}
	}
	return float64(matches) / float64(len(statementTokens))
}

func splitDeepDrillWords(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

func detectNegationSignals(text string) []string {
	lower := strings.ToLower(text)
	words := splitDeepDrillWords(lower)
	wordSet := make(map[string]struct{}, len(words))
	for _, word := range words {
		wordSet[word] = struct{}{}
	}
	signals := make([]string, 0, 8)
	for _, phrase := range []string{"does not", "did not", "do not", "no evidence", "rather than", "separate from", "unrelated to", "independent of"} {
		if strings.Contains(lower, phrase) {
			signals = append(signals, phrase)
		}
	}
	for _, word := range []string{"never", "without", "instead", "contrary", "however", "not"} {
		if _, ok := wordSet[word]; ok {
			signals = append(signals, word)
		}
	}
	if strings.Contains(lower, "contradict") {
		signals = append(signals, "contradict*")
	}
	if strings.Contains(lower, "inconsisten") {
		signals = append(signals, "inconsisten*")
	}
	return uniqueTrimmed(signals)
}

func detectHedgeSignals(text string) []string {
	lower := strings.ToLower(text)
	words := splitDeepDrillWords(lower)
	wordSet := make(map[string]struct{}, len(words))
	for _, word := range words {
		wordSet[word] = struct{}{}
	}
	signals := make([]string, 0, 6)
	for _, word := range []string{"may", "might", "possibly", "perhaps", "unclear", "uncertain", "ambiguous", "whether", "maybe"} {
		if _, ok := wordSet[word]; ok {
			signals = append(signals, word)
		}
	}
	return uniqueTrimmed(signals)
}

func classifyEvidenceForHypothesis(hypothesis DeepDrillHypothesisState, item ResearchResult) deepDrillEvidenceClassification {
	result := deepDrillEvidenceClassification{EvidenceID: evidenceIDForResult(item)}
	envelope := buildEvidenceEnvelope(item)
	result.UsableTextFields = envelope.fields
	if len(envelope.fields) == 0 {
		result.Polarity = deepDrillPolarityNone
		result.Reason = "no classifiable source text"
		return result
	}

	hypothesisConcepts := normalizeConcepts(hypothesis.Statement)
	evidenceConcepts := normalizeConcepts(envelope.body)
	result.HypothesisConcepts = hypothesisConcepts
	result.EvidenceConcepts = evidenceConcepts

	if len(hypothesisConcepts) == 0 {
		result.Polarity = deepDrillPolarityNone
		result.Reason = "empty hypothesis statement"
		return result
	}

	result.LexicalOverlap = rawLexicalOverlap(hypothesis.Statement, envelope.body)

	evidenceSet := make(map[string]struct{}, len(evidenceConcepts))
	for _, concept := range evidenceConcepts {
		evidenceSet[concept] = struct{}{}
	}
	for _, concept := range hypothesisConcepts {
		if _, ok := evidenceSet[concept]; ok {
			result.ConceptMatches = append(result.ConceptMatches, concept)
		}
	}
	for _, concept := range result.ConceptMatches {
		if family, ok := deepDrillRelationConcepts[concept]; ok {
			result.RelationMatches = append(result.RelationMatches, family)
		}
	}
	result.RelationMatches = uniqueTrimmed(result.RelationMatches)

	result.NegationSignals = detectNegationSignals(envelope.body)
	result.HedgeSignals = detectHedgeSignals(envelope.body)

	coverage := 0.0
	if len(hypothesisConcepts) > 0 {
		coverage = float64(len(result.ConceptMatches)) / float64(len(hypothesisConcepts))
	}
	result.RelevanceScore = coverage

	if len(result.RelationMatches) > 0 || coverage >= deepDrillRelevanceThreshold {
		result.Relevant = true
	}

	if !result.Relevant {
		result.Polarity = deepDrillPolarityNone
		result.Reason = fmt.Sprintf("no shared relation or concept coverage (%.2f)", coverage)
		return result
	}

	switch {
	case len(result.NegationSignals) > 0:
		result.Polarity = deepDrillPolarityCounter
		result.Reason = fmt.Sprintf("relevant with negation signal: %s", strings.Join(result.NegationSignals, ", "))
	case len(result.HedgeSignals) > 0:
		result.Polarity = deepDrillPolarityAmbiguous
		result.Reason = fmt.Sprintf("relevant but polarity unresolved (%s)", strings.Join(result.HedgeSignals, ", "))
	default:
		result.Polarity = deepDrillPolaritySupport
		result.Reason = "relevant without negation or hedge signal"
	}
	return result
}

func tokenizeMeaningful(text string) []string {
	lower := strings.ToLower(strings.TrimSpace(text))
	fields := strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	out := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if !isMeaningfulDeepDrillToken(field) {
			continue
		}
		if _, ok := deepDrillStopwords[field]; ok {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out
}

func isMeaningfulDeepDrillToken(field string) bool {
	if len(field) >= 3 {
		return true
	}
	if len(field) == 2 {
		for _, r := range field {
			if r >= '0' && r <= '9' {
				return true
			}
		}
	}
	return false
}

// diagnoseEvidenceClassification reproduces classifyEvidenceForHypotheses
// decision-by-decision while surfacing the intermediate values that the
// production path discards. It is intended for audits and tests only; it does
// not alter stored state or hypothesis scoring.
func diagnoseEvidenceClassification(hypotheses []DeepDrillHypothesisState, evidence []ResearchResult) []deepDrillClassificationRow {
	rows := make([]deepDrillClassificationRow, 0, len(hypotheses)*len(evidence))
	for _, item := range evidence {
		for _, hypothesis := range hypotheses {
			c := classifyEvidenceForHypothesis(hypothesis, item)
			rows = append(rows, deepDrillClassificationRow{
				EvidenceID:       c.EvidenceID,
				HypothesisID:     hypothesis.ID,
				UsableTextFields: c.UsableTextFields,
				LexicalOverlap:   c.LexicalOverlap,
				ConceptMatches:   c.ConceptMatches,
				RelationMatches:  c.RelationMatches,
				NegationSignals:  c.NegationSignals,
				RelevanceScore:   c.RelevanceScore,
				Relevant:         c.Relevant,
				Polarity:         string(c.Polarity),
				Reason:           c.Reason,
			})
		}
	}
	return rows
}

func rerankHypotheses(hypotheses []DeepDrillHypothesisState) {
	if len(hypotheses) == 0 {
		return
	}
	oldRanks := make(map[string]int, len(hypotheses))
	for _, hypothesis := range hypotheses {
		oldRanks[hypothesis.ID] = hypothesis.Rank
	}
	sort.SliceStable(hypotheses, func(i, j int) bool {
		si := hypothesisRankScore(hypotheses[i])
		sj := hypothesisRankScore(hypotheses[j])
		if si == sj {
			return hypotheses[i].ID < hypotheses[j].ID
		}
		return si > sj
	})
	for idx := range hypotheses {
		hypotheses[idx].RankDelta = (idx + 1) - oldRanks[hypotheses[idx].ID]
		hypotheses[idx].Rank = idx + 1
	}
}

func hypothesisRankScore(hypothesis DeepDrillHypothesisState) float64 {
	return hypothesis.Confidence + hypothesis.EvidenceStrength*0.1
}

func cloneHypotheses(hypotheses []DeepDrillHypothesisState) []DeepDrillHypothesisState {
	clone := make([]DeepDrillHypothesisState, len(hypotheses))
	copy(clone, hypotheses)
	for idx := range clone {
		clone[idx].SupportingEvidenceIDs = append([]string(nil), hypotheses[idx].SupportingEvidenceIDs...)
		clone[idx].ContradictingEvidence = append([]string(nil), hypotheses[idx].ContradictingEvidence...)
		clone[idx].AmbiguousEvidenceIDs = append([]string(nil), hypotheses[idx].AmbiguousEvidenceIDs...)
	}
	return clone
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for idx := range a {
		if a[idx] != b[idx] {
			return false
		}
	}
	return true
}

func scoreInformationGain(modelBefore, modelAfter []DeepDrillHypothesisState, state *DeepDrillState, evidenceCount int, duplicate bool) DeepDrillInfoGain {
	if state != nil && state.BranchStatus == DeepDrillFrozen {
		return DeepDrillInfoHigh
	}
	if duplicate {
		return DeepDrillInfoLow
	}
	if evidenceCount == 0 {
		return DeepDrillInfoLow
	}
	if materialModelChange(modelBefore, modelAfter) {
		return DeepDrillInfoHigh
	}
	return DeepDrillInfoMedium
}

func exportHypothesisModel(hypotheses []DeepDrillHypothesisState) map[string]any {
	rows := make([]map[string]any, 0, len(hypotheses))
	for _, hypothesis := range hypotheses {
		rows = append(rows, map[string]any{
			"id":                           hypothesis.ID,
			"rank":                         hypothesis.Rank,
			"rank_delta":                   hypothesis.RankDelta,
			"confidence":                   hypothesis.Confidence,
			"prior_confidence":             hypothesis.PriorConfidence,
			"confidence_delta":             hypothesis.ConfidenceDelta,
			"evidence_strength":            hypothesis.EvidenceStrength,
			"support_weight":               hypothesis.SupportWeight,
			"counterevidence_weight":       hypothesis.CounterevidenceWeight,
			"provenance_weight":            hypothesis.ProvenanceWeight,
			"unique_support_count":         hypothesis.UniqueSupportCount,
			"unique_counterevidence_count": hypothesis.UniqueCounterevidenceCount,
			"unique_ambiguous_count":       hypothesis.UniqueAmbiguousCount,
			"scoring_reason":               hypothesis.ScoringReason,
			"status":                       hypothesis.Status,
			"supporting_evidence_ids":      hypothesis.SupportingEvidenceIDs,
			"contradicting_evidence_ids":   hypothesis.ContradictingEvidence,
			"ambiguous_evidence_ids":       hypothesis.AmbiguousEvidenceIDs,
		})
	}
	return map[string]any{"hypotheses": rows}
}

func normalizeHypothesisSeeds(input []DeepDrillHypothesisState) []DeepDrillHypothesisState {
	rows := make([]DeepDrillHypothesisState, 0, len(input))
	now := time.Now().UTC()
	for idx, item := range input {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = fmt.Sprintf("H%d", idx+1)
		}
		statement := strings.TrimSpace(item.Statement)
		if statement == "" {
			statement = id
		}
		row := DeepDrillHypothesisState{
			ID:                    id,
			Statement:             statement,
			Rank:                  idx + 1,
			Confidence:            defaultConfidence(item.Confidence),
			EvidenceStrength:      clamp01(item.EvidenceStrength),
			Status:                firstNonEmpty(item.Status, "active"),
			SupportingEvidenceIDs: uniqueTrimmed(item.SupportingEvidenceIDs),
			ContradictingEvidence: uniqueTrimmed(item.ContradictingEvidence),
			AmbiguousEvidenceIDs:  uniqueTrimmed(item.AmbiguousEvidenceIDs),
			LastUpdated:           now,
		}
		if item.Rank > 0 {
			row.Rank = item.Rank
		}
		if !item.LastUpdated.IsZero() {
			row.LastUpdated = item.LastUpdated
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Rank == rows[j].Rank {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].Rank < rows[j].Rank
	})
	for idx := range rows {
		rows[idx].Rank = idx + 1
	}
	return rows
}

func normalizeHypothesesFromStrings(items []string) []DeepDrillHypothesisState {
	if len(items) == 0 {
		return nil
	}
	states := make([]DeepDrillHypothesisState, 0, len(items))
	for idx, item := range items {
		statement := strings.TrimSpace(item)
		if statement == "" {
			continue
		}
		states = append(states, DeepDrillHypothesisState{ID: fmt.Sprintf("H%d", idx+1), Statement: statement, Rank: idx + 1, Confidence: 0.42, EvidenceStrength: 0.2, Status: "active"})
	}
	return normalizeHypothesisSeeds(states)
}

func normalizeUncertainties(items []DeepDrillUncertainty) []DeepDrillUncertainty {
	seen := map[DeepDrillUncertainty]struct{}{}
	rows := make([]DeepDrillUncertainty, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(string(item)) == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		rows = append(rows, item)
	}
	return rows
}

func stripGenericVocabulary(question string) string {
	generic := []string{"generic", "structure", "framework", "architecture", "generally", "broadly", "typically"}
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(question)))
	kept := make([]string, 0, len(fields))
	for _, field := range fields {
		skip := false
		for _, term := range generic {
			if field == term {
				skip = true
				break
			}
		}
		if !skip {
			kept = append(kept, field)
		}
	}
	if len(kept) == 0 {
		return strings.TrimSpace(question)
	}
	return strings.Join(kept, " ")
}

func deepDrillModelKey(state *DeepDrillState) string {
	parts := make([]string, 0, len(state.Hypotheses)+1)
	if state != nil {
		parts = append(parts, "tier:"+string(state.LastSeenProvenanceTier))
		for _, hypothesis := range state.Hypotheses {
			parts = append(parts, fmt.Sprintf("%s:%d:%s", hypothesis.ID, hypothesis.Rank, strings.TrimSpace(hypothesis.Status)))
		}
	}
	return strings.Join(parts, "|")
}
