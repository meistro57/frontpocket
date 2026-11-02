package tools

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/meistro57/frontpocket/internal/store"
)

type DeepDrillThoughtSummary struct {
	Collection               string             `json:"collection"`
	SessionID                string             `json:"session_id,omitempty"`
	OpenQuestions            []DeepDrillThought `json:"open_questions,omitempty"`
	FrozenBranches           []DeepDrillThought `json:"frozen_branches,omitempty"`
	UnresolvedContradictions []DeepDrillThought `json:"unresolved_contradictions,omitempty"`
	RecentModelRevisions     []DeepDrillThought `json:"recent_model_revisions,omitempty"`
	HighInformationGainRuns  []DeepDrillThought `json:"high_information_gain_runs,omitempty"`
	NegativeResults          []DeepDrillThought `json:"negative_results,omitempty"`
	ThoughtCountByType       map[string]int     `json:"thought_count_by_type"`
	TotalThoughts            int                `json:"total_thoughts"`
}

func (rt *MindDrillResearchRuntime) storeDeepDrillThought(ctx context.Context, session ResearchSession, state *DeepDrillState, thought DeepDrillThought) (DeepDrillThought, bool, error) {
	if rt.embedder == nil {
		return DeepDrillThought{}, false, fmt.Errorf("embedder is not configured")
	}
	if rt.client == nil {
		return DeepDrillThought{}, false, fmt.Errorf("qdrant client is not configured")
	}
	if state == nil {
		state = newDefaultDeepDrillState(rt.deepDrillThoughts)
	}
	if strings.TrimSpace(state.ThoughtCollection) == "" {
		state.ThoughtCollection = rt.deepDrillThoughts
	}
	if strings.TrimSpace(thought.SessionID) == "" {
		thought.SessionID = session.SessionID
	}
	if thought.Timestamp.IsZero() {
		thought.Timestamp = time.Now().UTC()
	}
	thought.Fingerprint = deepDrillFingerprint(thought)
	if strings.TrimSpace(thought.ThoughtID) == "" {
		thought.ThoughtID = fmt.Sprintf("deepdrill_%s_%d", sanitizeForID(thought.SessionID), thought.Timestamp.UnixNano())
	}
	dimsProbe, err := rt.embedder.EmbedText(ctx, "deepdrill thought collection probe")
	if err != nil {
		return DeepDrillThought{}, false, err
	}
	if err := rt.client.EnsureCollection(ctx, state.ThoughtCollection, rt.vectorName, "Cosine", len(dimsProbe)); err != nil {
		return DeepDrillThought{}, false, err
	}
	duplicateCount, err := rt.client.CountPoints(ctx, state.ThoughtCollection, map[string]string{"fingerprint": thought.Fingerprint, "session_id": thought.SessionID, "source_type": "deepdrill_thought"})
	if err != nil {
		return DeepDrillThought{}, false, err
	}
	isDuplicate := duplicateCount > 0
	if isDuplicate {
		thought.Status = "rediscovery"
		if thought.InfoGain == "" {
			thought.InfoGain = DeepDrillInfoLow
		}
	}
	if thought.EvidenceOrigin == "" {
		thought.EvidenceOrigin = DeepDrillResearchThought
	}
	vectorText := thoughtVectorText(thought)
	vector, err := rt.embedder.EmbedText(ctx, vectorText)
	if err != nil {
		return DeepDrillThought{}, false, err
	}
	payload := deepDrillThoughtPayload(thought)
	if err := rt.client.UpsertPayloadVectors(ctx, state.ThoughtCollection, rt.vectorName, []store.QdrantPayloadVectorPoint{{ID: thought.ThoughtID, Vector: vector, Payload: payload}}); err != nil {
		return DeepDrillThought{}, false, err
	}
	return thought, isDuplicate, nil
}

func deepDrillThoughtPayload(thought DeepDrillThought) map[string]any {
	payload := map[string]any{
		"memory_id":          thought.ThoughtID,
		"session_id":         thought.SessionID,
		"conversation_id":    thought.SessionID,
		"timestamp":          thought.Timestamp.Format(time.RFC3339),
		"source_type":        "deepdrill_thought",
		"source_title":       "DeepDrill Research Thought",
		"speaker":            "system",
		"memory_kind":        "research_note",
		"text":               thoughtVectorText(thought),
		"summary":            strings.TrimSpace(thought.EvidenceSummary),
		"source_quote":       strings.TrimSpace(thought.Question),
		"project":            "DeepDrill",
		"thought_id":         thought.ThoughtID,
		"type":               string(thought.Type),
		"question":           thought.Question,
		"hypothesis_targets": thought.HypothesisTargets,
		"uncertainty_class":  string(thought.UncertaintyClass),
		"strategy":           string(thought.Strategy),
		"query_spec":         thought.QuerySpec,
		"sources":            thought.Sources,
		"evidence_summary":   thought.EvidenceSummary,
		"provenance_score":   thought.ProvenanceScore,
		"evidence_strength":  thought.EvidenceStrength,
		"confidence":         thought.Confidence,
		"contradiction_flag": thought.ContradictionFlag,
		"model_before":       thought.ModelBefore,
		"model_after":        thought.ModelAfter,
		"info_gain":          string(thought.InfoGain),
		"next_action":        thought.NextAction,
		"status":             thought.Status,
		"parent_thought_ids": thought.ParentThoughtIDs,
		"supersedes":         thought.Supersedes,
		"fingerprint":        thought.Fingerprint,
		"evidence_origin":    string(thought.EvidenceOrigin),
	}
	if thought.DuplicateOfThought != "" {
		payload["duplicate_of_thought"] = thought.DuplicateOfThought
	}
	if thought.PreviousUncertainty != "" {
		payload["previous_uncertainty"] = string(thought.PreviousUncertainty)
	}
	if thought.ReclassifiedUncertainty != "" {
		payload["reclassified_uncertainty"] = string(thought.ReclassifiedUncertainty)
	}
	if thought.ReclassificationReason != "" {
		payload["reclassification_reason"] = thought.ReclassificationReason
	}
	if len(thought.StrategiesRemaining) > 0 {
		payload["strategies_remaining"] = strategyStrings(thought.StrategiesRemaining)
	}
	if len(thought.StrategiesExhausted) > 0 {
		payload["strategies_exhausted"] = strategyStrings(thought.StrategiesExhausted)
	}
	return payload
}

func thoughtVectorText(thought DeepDrillThought) string {
	parts := []string{
		string(thought.Type),
		thought.Question,
		string(thought.UncertaintyClass),
		string(thought.Strategy),
		thought.EvidenceSummary,
		strings.Join(thought.Sources, " | "),
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func deepDrillFingerprint(thought DeepDrillThought) string {
	normalizedQuestion := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(thought.Question)), " "))
	sources := append([]string(nil), thought.Sources...)
	sort.Strings(sources)
	seed := strings.Join([]string{normalizedQuestion, string(thought.Strategy), strings.Join(sources, ","), strings.ToLower(strings.TrimSpace(thought.EvidenceSummary))}, "||")
	h := sha1.Sum([]byte(seed))
	return hex.EncodeToString(h[:])
}

func sanitizeForID(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return "session"
	}
	var b strings.Builder
	for _, r := range trimmed {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('_')
	}
	normalized := strings.Trim(b.String(), "_")
	if normalized == "" {
		return "session"
	}
	return normalized
}

func (rt *MindDrillResearchRuntime) DeepDrillThoughts(ctx context.Context, query DeepDrillThoughtQuery) (DeepDrillThoughtList, error) {
	if rt == nil || rt.client == nil {
		return DeepDrillThoughtList{}, fmt.Errorf("minddrill research runtime is not configured")
	}
	collection := strings.TrimSpace(rt.deepDrillThoughts)
	if collection == "" {
		return DeepDrillThoughtList{}, fmt.Errorf("deepdrill thought collection is not configured")
	}
	if query.Limit <= 0 {
		query.Limit = 20
	}
	if query.Limit > 200 {
		query.Limit = 200
	}
	filters := map[string]string{"source_type": "deepdrill_thought"}
	if trimmed := strings.TrimSpace(query.SessionID); trimmed != "" {
		filters["session_id"] = trimmed
	}
	matched := make([]DeepDrillThought, 0, query.Limit)
	var offset any
	for len(matched) < query.Limit {
		page, next, err := rt.client.ScrollCollection(ctx, store.QdrantScrollRequest{Collection: collection, Limit: 256, Offset: offset, Filter: buildFilter(filters)})
		if err != nil {
			return DeepDrillThoughtList{}, err
		}
		if len(page) == 0 {
			break
		}
		for _, payload := range page {
			thought := deepDrillThoughtFromPayload(payload)
			if matchesDeepDrillThoughtQuery(thought, query) {
				matched = append(matched, thought)
				if len(matched) >= query.Limit {
					break
				}
			}
		}
		if next == nil {
			break
		}
		nextValue := strings.TrimSpace(fmt.Sprintf("%v", next))
		if nextValue == "" || nextValue == strings.TrimSpace(fmt.Sprintf("%v", offset)) {
			break
		}
		offset = next
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].Timestamp.Equal(matched[j].Timestamp) {
			return matched[i].ThoughtID > matched[j].ThoughtID
		}
		return matched[i].Timestamp.After(matched[j].Timestamp)
	})
	return DeepDrillThoughtList{Collection: collection, Query: query, Thoughts: matched}, nil
}

func (rt *MindDrillResearchRuntime) DeepDrillShowThought(ctx context.Context, thoughtID string) (DeepDrillThought, error) {
	trimmedID := strings.TrimSpace(thoughtID)
	if trimmedID == "" {
		return DeepDrillThought{}, fmt.Errorf("thought_id is required")
	}
	if rt == nil || rt.client == nil {
		return DeepDrillThought{}, fmt.Errorf("minddrill research runtime is not configured")
	}
	collection := strings.TrimSpace(rt.deepDrillThoughts)
	if collection == "" {
		return DeepDrillThought{}, fmt.Errorf("deepdrill thought collection is not configured")
	}
	filters := buildFilter(map[string]string{"source_type": "deepdrill_thought"})
	var offset any
	for {
		page, next, err := rt.client.ScrollCollection(ctx, store.QdrantScrollRequest{Collection: collection, Limit: 256, Offset: offset, Filter: filters})
		if err != nil {
			return DeepDrillThought{}, err
		}
		if len(page) == 0 {
			break
		}
		for _, payload := range page {
			thought := deepDrillThoughtFromPayload(payload)
			if thought.ThoughtID == trimmedID {
				return thought, nil
			}
		}
		if next == nil {
			break
		}
		nextValue := strings.TrimSpace(fmt.Sprintf("%v", next))
		if nextValue == "" || nextValue == strings.TrimSpace(fmt.Sprintf("%v", offset)) {
			break
		}
		offset = next
	}
	return DeepDrillThought{}, fmt.Errorf("thought %q not found", trimmedID)
}

func (rt *MindDrillResearchRuntime) DeepDrillThoughtSummary(ctx context.Context, query DeepDrillThoughtQuery) (DeepDrillThoughtSummary, error) {
	query.Limit = 500
	list, err := rt.DeepDrillThoughts(ctx, query)
	if err != nil {
		return DeepDrillThoughtSummary{}, err
	}
	summary := DeepDrillThoughtSummary{
		Collection:         list.Collection,
		SessionID:          strings.TrimSpace(query.SessionID),
		ThoughtCountByType: map[string]int{},
		TotalThoughts:      len(list.Thoughts),
	}
	for _, thought := range list.Thoughts {
		summary.ThoughtCountByType[string(thought.Type)]++
		if thought.Type == DeepDrillThoughtQuestion && !isThoughtResolved(thought) {
			summary.OpenQuestions = append(summary.OpenQuestions, thought)
		}
		if normalizeArtifactStatus(thought) == "FROZEN" {
			summary.FrozenBranches = append(summary.FrozenBranches, thought)
		}
		if thought.Type == DeepDrillThoughtContradiction && !isThoughtResolved(thought) {
			summary.UnresolvedContradictions = append(summary.UnresolvedContradictions, thought)
		}
		if thought.Type == DeepDrillThoughtModelRevision {
			summary.RecentModelRevisions = append(summary.RecentModelRevisions, thought)
		}
		if thought.InfoGain == DeepDrillInfoHigh {
			summary.HighInformationGainRuns = append(summary.HighInformationGainRuns, thought)
		}
		if thought.Type == DeepDrillThoughtNegativeResult {
			summary.NegativeResults = append(summary.NegativeResults, thought)
		}
	}
	summary.OpenQuestions = sampleThoughts(summary.OpenQuestions, 8)
	summary.FrozenBranches = sampleThoughts(summary.FrozenBranches, 8)
	summary.UnresolvedContradictions = sampleThoughts(summary.UnresolvedContradictions, 8)
	summary.RecentModelRevisions = sampleThoughts(summary.RecentModelRevisions, 8)
	summary.HighInformationGainRuns = sampleThoughts(summary.HighInformationGainRuns, 8)
	summary.NegativeResults = sampleThoughts(summary.NegativeResults, 8)
	return summary, nil
}

func deepDrillThoughtFromPayload(payload map[string]any) DeepDrillThought {
	thought := DeepDrillThought{
		ThoughtID:          strings.TrimSpace(asString(firstPayloadValue(payload, []string{"thought_id", "memory_id", "id"}))),
		SessionID:          strings.TrimSpace(asString(firstPayloadValue(payload, []string{"session_id", "conversation_id"}))),
		Timestamp:          parseAnyTime(firstPayloadValue(payload, []string{"timestamp", "created_at", "created_date"})),
		Type:               DeepDrillThoughtType(strings.TrimSpace(asString(firstPayloadValue(payload, []string{"type"})))),
		Question:           strings.TrimSpace(asString(firstPayloadValue(payload, []string{"question", "source_quote"}))),
		HypothesisTargets:  uniqueTrimmed(asStringSlice(firstPayloadValue(payload, []string{"hypothesis_targets"}))),
		UncertaintyClass:   DeepDrillUncertainty(strings.TrimSpace(asString(firstPayloadValue(payload, []string{"uncertainty_class"})))),
		Strategy:           DeepDrillStrategy(strings.TrimSpace(asString(firstPayloadValue(payload, []string{"strategy"})))),
		QuerySpec:          asMap(firstPayloadValue(payload, []string{"query_spec"})),
		Sources:            uniqueTrimmed(asStringSlice(firstPayloadValue(payload, []string{"sources"}))),
		EvidenceSummary:    strings.TrimSpace(asString(firstPayloadValue(payload, []string{"evidence_summary", "summary"}))),
		ProvenanceScore:    asFloat(firstPayloadValue(payload, []string{"provenance_score"})),
		EvidenceStrength:   asFloat(firstPayloadValue(payload, []string{"evidence_strength"})),
		Confidence:         asFloat(firstPayloadValue(payload, []string{"confidence"})),
		ContradictionFlag:  asBool(firstPayloadValue(payload, []string{"contradiction_flag"})),
		ModelBefore:        asMap(firstPayloadValue(payload, []string{"model_before"})),
		ModelAfter:         asMap(firstPayloadValue(payload, []string{"model_after"})),
		InfoGain:           DeepDrillInfoGain(strings.ToUpper(strings.TrimSpace(asString(firstPayloadValue(payload, []string{"info_gain"}))))),
		NextAction:         strings.TrimSpace(asString(firstPayloadValue(payload, []string{"next_action"}))),
		Status:             strings.TrimSpace(asString(firstPayloadValue(payload, []string{"status"}))),
		ParentThoughtIDs:   uniqueTrimmed(asStringSlice(firstPayloadValue(payload, []string{"parent_thought_ids"}))),
		Supersedes:         uniqueTrimmed(asStringSlice(firstPayloadValue(payload, []string{"supersedes"}))),
		Fingerprint:        strings.TrimSpace(asString(firstPayloadValue(payload, []string{"fingerprint"}))),
		EvidenceOrigin:     DeepDrillEvidenceOrigin(strings.TrimSpace(asString(firstPayloadValue(payload, []string{"evidence_origin"})))),
		DuplicateOfThought: strings.TrimSpace(asString(firstPayloadValue(payload, []string{"duplicate_of_thought"}))),

		PreviousUncertainty:     DeepDrillUncertainty(strings.TrimSpace(asString(firstPayloadValue(payload, []string{"previous_uncertainty"})))),
		ReclassifiedUncertainty: DeepDrillUncertainty(strings.TrimSpace(asString(firstPayloadValue(payload, []string{"reclassified_uncertainty"})))),
		ReclassificationReason:  strings.TrimSpace(asString(firstPayloadValue(payload, []string{"reclassification_reason"}))),
		StrategiesRemaining:     parseDeepDrillStrategies(firstPayloadValue(payload, []string{"strategies_remaining"})),
		StrategiesExhausted:     parseDeepDrillStrategies(firstPayloadValue(payload, []string{"strategies_exhausted"})),
	}
	if thought.Timestamp.IsZero() {
		thought.Timestamp = parseAnyTime(firstPayloadValue(payload, []string{"modified_at", "updated_at"}))
	}
	if thought.EvidenceOrigin == "" {
		thought.EvidenceOrigin = DeepDrillResearchThought
	}
	if thought.ThoughtID == "" {
		thought.ThoughtID = strings.TrimSpace(asString(firstPayloadValue(payload, []string{"memory_id", "id"})))
	}
	return thought
}

func matchesDeepDrillThoughtQuery(thought DeepDrillThought, query DeepDrillThoughtQuery) bool {
	if trimmed := strings.TrimSpace(query.SessionID); trimmed != "" && thought.SessionID != trimmed {
		return false
	}
	if query.Type != "" && thought.Type != query.Type {
		return false
	}
	if query.Uncertainty != "" && thought.UncertaintyClass != query.Uncertainty {
		return false
	}
	if query.InfoGain != "" && thought.InfoGain != query.InfoGain {
		return false
	}
	if trimmed := strings.TrimSpace(query.Hypothesis); trimmed != "" {
		ok := false
		for _, target := range thought.HypothesisTargets {
			if strings.EqualFold(strings.TrimSpace(target), trimmed) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if trimmed := strings.TrimSpace(query.Status); trimmed != "" {
		status := normalizeArtifactStatus(thought)
		if !strings.EqualFold(status, trimmed) && !strings.EqualFold(strings.TrimSpace(thought.Status), trimmed) {
			return false
		}
	}
	return true
}

func normalizeArtifactStatus(thought DeepDrillThought) string {
	status := strings.ToUpper(strings.TrimSpace(thought.Status))
	if status == "" {
		switch thought.Type {
		case DeepDrillThoughtStopDecision:
			if thought.Strategy == DeepDrillFreezeBranch {
				return "FROZEN"
			}
		case DeepDrillThoughtNegativeResult:
			return "OPEN"
		}
		return "OPEN"
	}
	if strings.Contains(status, "RESOLV") {
		return "RESOLVED"
	}
	if strings.Contains(status, "SUPERSEDED") {
		return "SUPERSEDED"
	}
	if strings.Contains(status, "FROZEN") {
		return "FROZEN"
	}
	if strings.Contains(status, "REDISCOVERY") {
		return "OPEN"
	}
	return status
}

func isThoughtResolved(thought DeepDrillThought) bool {
	status := normalizeArtifactStatus(thought)
	return status == "RESOLVED" || status == "SUPERSEDED"
}

func sampleThoughts(items []DeepDrillThought, limit int) []DeepDrillThought {
	if len(items) == 0 || limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}
