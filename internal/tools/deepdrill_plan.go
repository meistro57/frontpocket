package tools

import (
	"crypto/sha1"
	"encoding/hex"
	"sort"
	"strings"
	"time"
)

func identifyHighestValueUncertainty(session ResearchSession, state *DeepDrillState) DeepDrillUncertainty {
	if state == nil {
		return DeepDrillEvidenceGap
	}
	if state.BranchStatus == DeepDrillFrozen {
		return DeepDrillDiminishingReturns
	}
	if strings.TrimSpace(string(state.ReclassifiedUncertainty)) != "" && state.ReclassifiedUncertainty != DeepDrillDiminishingReturns {
		return state.ReclassifiedUncertainty
	}
	if state.ConsecutiveLowGain >= 1 {
		return DeepDrillDiminishingReturns
	}
	if len(state.KnownBlockers) > 0 {
		return state.KnownBlockers[0]
	}
	q := strings.ToLower(strings.TrimSpace(state.HighestValueQuestion + " " + session.ResearchQuestion))
	switch {
	case strings.Contains(q, "earliest") || strings.Contains(q, "first explicit") || strings.Contains(q, "draft") || strings.Contains(q, "lineage") || strings.Contains(q, "chronolog"):
		return DeepDrillChronologyGap
	case strings.Contains(q, "provenance") || strings.Contains(q, "transcript") || strings.Contains(q, "citation"):
		return DeepDrillProvenanceGap
	case len(session.SearchesPerformed) == 0:
		return DeepDrillEvidenceGap
	default:
		return DeepDrillAmbiguousClassification
	}
}

func generateCompetingQuestions(session ResearchSession, state *DeepDrillState, uncertainty DeepDrillUncertainty) []string {
	if state != nil && strings.TrimSpace(state.HighestValueQuestion) != "" {
		return uniqueTrimmed([]string{
			state.HighestValueQuestion,
			"What evidence could falsify the currently top-ranked hypothesis first?",
			"Which unresolved source relationship would most change hypothesis rank if resolved?",
		})
	}
	switch uncertainty {
	case DeepDrillChronologyGap:
		return []string{
			"In the earliest recoverable material, what is the first explicit bridge sentence between sacred-symbolic terms and E8-specific technical constructs?",
			"Do source-local sequence traces place bridge language before or after E8 terminology?",
			"Is there any upstream citation proving bridge terminology existed before E8 adoption?",
		}
	case DeepDrillProvenanceGap:
		return []string{
			"What is the highest provenance tier currently supporting each hypothesis?",
			"Can any derived-summary claim be traced to a raw source or versioned draft?",
			"Which hypothesis depends most on weak provenance and should have confidence reduced?",
		}
	case DeepDrillDiminishingReturns:
		return []string{
			"Has this branch produced repeated low-gain duplicate evidence and should be frozen?",
			"What new provenance tier or contradiction would justify reopening this branch?",
		}
	default:
		base := strings.TrimSpace(session.ResearchQuestion)
		if base == "" {
			base = "What evidence most strongly discriminates between the current hypotheses?"
		}
		return []string{base, "What counterevidence would most likely overturn the current ranking?"}
	}
}

func selectMostDiscriminatingQuestion(candidates []string, state *DeepDrillState) string {
	best := ""
	bestScore := -1
	for _, candidate := range candidates {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "" {
			continue
		}
		score := 1
		lower := strings.ToLower(trimmed)
		if state != nil && strings.TrimSpace(state.HighestValueQuestion) != "" && trimmed == strings.TrimSpace(state.HighestValueQuestion) {
			score += 5
		}
		if strings.Contains(lower, "first") || strings.Contains(lower, "earliest") || strings.Contains(lower, "upstream") {
			score += 2
		}
		if strings.Contains(lower, "falsif") || strings.Contains(lower, "counter") {
			score += 2
		}
		if state != nil && trimmed != state.CurrentQuestion {
			score += 1
		}
		if score > bestScore {
			best = trimmed
			bestScore = score
		}
	}
	if best == "" {
		return "What is the next most discriminating research question?"
	}
	return best
}

func selectStrategy(state *DeepDrillState, uncertainty DeepDrillUncertainty) (DeepDrillStrategy, string, bool) {
	candidates := strategyCandidates(uncertainty)
	if len(candidates) == 0 {
		return DeepDrillSemanticBroad, "default fallback", false
	}
	// Exclude exhausted strategies before ranking so a failed strategy never
	// wins selection while unused materially distinct strategies remain.
	available := make([]DeepDrillStrategy, 0, len(candidates))
	if state == nil {
		available = append(available, candidates...)
	} else {
		for _, strategy := range candidates {
			if !isStrategyExhausted(state, uncertainty, strategy) {
				available = append(available, strategy)
			}
		}
	}
	if len(available) == 0 {
		return DeepDrillFreezeBranch, "all strategies for uncertainty are exhausted", true
	}
	if state == nil || len(state.StrategyHistory) == 0 {
		return available[0], "no prior runs, using primary strategy for uncertainty", false
	}
	usage := make(map[DeepDrillStrategy]int)
	success := make(map[DeepDrillStrategy]int)
	for _, entry := range state.StrategyHistory {
		if entry.Uncertainty != uncertainty {
			continue
		}
		usage[entry.Strategy]++
		if entry.InfoGain == DeepDrillInfoMedium || entry.InfoGain == DeepDrillInfoHigh {
			success[entry.Strategy]++
		}
	}
	best := available[0]
	bestScore := -1
	reused := false
	for _, strategy := range available {
		score := success[strategy]*3 - usage[strategy]
		if score > bestScore {
			best = strategy
			bestScore = score
			reused = usage[strategy] > 0
		}
	}
	return best, "selected by uncertainty class and historical strategy yield", reused
}

func strategyCandidates(uncertainty DeepDrillUncertainty) []DeepDrillStrategy {
	switch uncertainty {
	case DeepDrillEvidenceGap:
		return []DeepDrillStrategy{DeepDrillSemanticBroad, DeepDrillKeywordTargeted, DeepDrillDocumentExpansion, DeepDrillCrossDocument}
	case DeepDrillRetrievalLimit:
		return []DeepDrillStrategy{DeepDrillBroadScroll, DeepDrillCollectionInspection, DeepDrillInspectCollection, DeepDrillCrossDocument}
	case DeepDrillAmbiguousClassification:
		return []DeepDrillStrategy{DeepDrillMarkerPartition, DeepDrillNegativeControl, DeepDrillPartitionTest, DeepDrillCrossDocument}
	case DeepDrillGenericSimilarity:
		return []DeepDrillStrategy{DeepDrillVocabularyStrip, DeepDrillNegativeControl, DeepDrillKeywordTargeted, DeepDrillCrossDocument}
	case DeepDrillChronologyGap:
		return []DeepDrillStrategy{DeepDrillProvenanceTrace, DeepDrillSourceSequence, DeepDrillChronologyTrace, DeepDrillTimestampTrace, DeepDrillDocumentExpansion}
	case DeepDrillProvenanceGap:
		return []DeepDrillStrategy{DeepDrillProvenanceTrace, DeepDrillProvenanceProfile, DeepDrillRawSourceLookup, DeepDrillConfidenceDowngrade}
	case DeepDrillContradiction:
		return []DeepDrillStrategy{DeepDrillCounterevidenceSearch, DeepDrillSourceComparison, DeepDrillCounterEvidenceScan, DeepDrillCrossDocument}
	case DeepDrillSourceQuality:
		return []DeepDrillStrategy{DeepDrillProvenanceTrace, DeepDrillSourceTypeProfile, DeepDrillProvenanceProfile, DeepDrillConfidenceDowngrade}
	case DeepDrillDiminishingReturns:
		return []DeepDrillStrategy{DeepDrillFreezeBranch}
	default:
		return []DeepDrillStrategy{DeepDrillSemanticBroad, DeepDrillKeywordTargeted}
	}
}

func shouldReopenFrozenBranch(state *DeepDrillState, question, _ string) bool {
	if state == nil || state.BranchStatus != DeepDrillFrozen {
		return false
	}
	if strings.TrimSpace(question) != "" && strings.TrimSpace(question) != strings.TrimSpace(state.CurrentQuestion) {
		return true
	}
	if state.LastSeenProvenanceTier == DeepDrillUnknownSourceTier {
		return true
	}
	return false
}

func nextActionFromState(state *DeepDrillState) string {
	if state == nil {
		return "continue"
	}
	if state.BranchStatus == DeepDrillFrozen {
		return "freeze"
	}
	if state.ConsecutiveLowGain > 0 {
		return "switch strategy class"
	}
	return "continue"
}

func hasChronologyQuestion(question string) bool {
	q := strings.ToLower(strings.TrimSpace(question))
	return strings.Contains(q, "earliest") || strings.Contains(q, "first") || strings.Contains(q, "chronolog") ||
		strings.Contains(q, "timestamp") || strings.Contains(q, "sequence") || strings.Contains(q, "draft") ||
		strings.Contains(q, "lineage") || strings.Contains(q, "before") || strings.Contains(q, "after")
}

func isStrategyExhausted(state *DeepDrillState, uncertainty DeepDrillUncertainty, strategy DeepDrillStrategy) bool {
	if state == nil {
		return false
	}
	modelKey := deepDrillModelKey(state)
	for _, record := range state.StrategyExhaustion {
		if record.Uncertainty != uncertainty || record.Strategy != strategy || record.ModelKey != modelKey {
			continue
		}
		// A higher provenance tier invalidates prior exhaustion: the strategy
		// has new, stronger evidence to work with.
		if compareSourceTier(state.LastSeenProvenanceTier, record.ProvenanceTier) > 0 {
			continue
		}
		return true
	}
	return false
}

func markStrategyExhausted(state *DeepDrillState, uncertainty DeepDrillUncertainty, strategy DeepDrillStrategy) {
	if state == nil {
		return
	}
	modelKey := deepDrillModelKey(state)
	for idx := range state.StrategyExhaustion {
		record := &state.StrategyExhaustion[idx]
		if record.Uncertainty == uncertainty && record.Strategy == strategy && record.ModelKey == modelKey {
			record.Timestamp = time.Now().UTC()
			return
		}
	}
	state.StrategyExhaustion = append(state.StrategyExhaustion, DeepDrillStrategyExhaustion{
		Uncertainty:    uncertainty,
		Strategy:       strategy,
		ModelKey:       modelKey,
		ProvenanceTier: state.LastSeenProvenanceTier,
		Timestamp:      time.Now().UTC(),
	})
}

func strategyAvailability(state *DeepDrillState, uncertainty DeepDrillUncertainty) (remaining, exhausted []DeepDrillStrategy) {
	remaining = []DeepDrillStrategy{}
	exhausted = []DeepDrillStrategy{}
	if state == nil {
		return remaining, exhausted
	}
	for _, strategy := range strategyCandidates(uncertainty) {
		if isStrategyExhausted(state, uncertainty, strategy) {
			exhausted = append(exhausted, strategy)
		} else {
			remaining = append(remaining, strategy)
		}
	}
	return remaining, exhausted
}

func strategyOutcomeSignature(entry DeepDrillStrategyOutcome) string {
	return strings.Join([]string{string(entry.Uncertainty), string(entry.Strategy), strings.TrimSpace(entry.EvidenceFingerprint)}, "|")
}

func strategyOutcomeBlockEqual(a, b []DeepDrillStrategyOutcome) bool {
	if len(a) != len(b) {
		return false
	}
	for idx := range a {
		if strategyOutcomeSignature(a[idx]) != strategyOutcomeSignature(b[idx]) {
			return false
		}
	}
	return true
}

func detectStrategyCycle(history []DeepDrillStrategyOutcome) bool {
	n := len(history)
	for length := 2; length <= n/2; length++ {
		if strategyOutcomeBlockEqual(history[n-length:], history[n-2*length:n-length]) {
			return true
		}
	}
	return false
}

func reclassifyUncertainty(state *DeepDrillState, evidence []ResearchResult, provenanceScore float64, topTier DeepDrillSourceTier, hasContradiction bool, question string) (DeepDrillUncertainty, string, bool) {
	current := DeepDrillEvidenceGap
	if state != nil && strings.TrimSpace(string(state.CurrentUncertainty)) != "" {
		current = state.CurrentUncertainty
	}

	weakProvenance := provenanceScore > 0 && provenanceScore < 0.6
	weakTier := isWeakProvenanceTier(topTier)
	noCoverage := len(evidence) == 0
	chronologyQuestion := hasChronologyQuestion(question)

	type signal struct {
		uncertainty DeepDrillUncertainty
		reason      string
		applicable  bool
	}
	signals := []signal{
		{DeepDrillContradiction, "retrieved evidence contains contradiction signals", hasContradiction},
		{DeepDrillRetrievalLimit, "strategy returned no retrieval coverage for the unresolved claim", noCoverage},
		{DeepDrillProvenanceGap, "retrieved evidence depends on derived or weak provenance", weakProvenance || weakTier},
		{DeepDrillChronologyGap, "chronology remains ambiguous despite available provenance", chronologyQuestion && !weakProvenance && !weakTier},
		{DeepDrillSourceQuality, "retrieved source quality is unverifiable", topTier == DeepDrillUnknownSourceTier},
		{DeepDrillGenericSimilarity, "retrieved evidence resembles generic surface similarity rather than discriminating detail", !hasContradiction && !noCoverage && !weakProvenance && !weakTier},
	}

	for _, signal := range signals {
		if !signal.applicable {
			continue
		}
		if signal.uncertainty == current || signal.uncertainty == DeepDrillDiminishingReturns {
			continue
		}
		return signal.uncertainty, signal.reason, true
	}
	return current, "", false
}

func strategyEvidenceFingerprint(strategy DeepDrillStrategy, question string, evidence []ResearchResult) string {
	sources := make([]string, 0, len(evidence))
	for _, item := range evidence {
		sources = append(sources, firstNonEmpty(item.ChunkID, item.MemoryID, item.SourceFingerprint, item.SourceDocumentID, item.SourceTitle))
	}
	sources = uniqueTrimmed(sources)
	sort.Strings(sources)
	seed := strings.Join([]string{string(strategy), strings.ToLower(strings.TrimSpace(question)), strings.Join(sources, ",")}, "||")
	hash := sha1.Sum([]byte(seed))
	return hex.EncodeToString(hash[:])
}

func isRepeatStrategyFinding(state *DeepDrillState, strategy DeepDrillStrategy, question, evidenceFingerprint string) bool {
	if state == nil || len(state.StrategyHistory) == 0 || strings.TrimSpace(evidenceFingerprint) == "" {
		return false
	}
	for i := len(state.StrategyHistory) - 1; i >= 0; i-- {
		entry := state.StrategyHistory[i]
		if entry.Strategy == strategy && strings.TrimSpace(entry.Question) == strings.TrimSpace(question) && strings.TrimSpace(entry.EvidenceFingerprint) == strings.TrimSpace(evidenceFingerprint) {
			return true
		}
	}
	return false
}
