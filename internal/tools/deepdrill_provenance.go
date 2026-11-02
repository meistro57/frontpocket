package tools

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/meistro57/frontpocket/internal/store"
)

func sumProvenanceWeight(items []ResearchResult) float64 {
	total := 0.0
	for _, item := range items {
		total += provenanceTierWeight(provenanceTierForResult(item))
	}
	return total
}

func provenanceTierForResult(item ResearchResult) DeepDrillSourceTier {
	sourceType := strings.ToLower(strings.TrimSpace(item.SourceType))
	sourceTitle := strings.ToLower(strings.TrimSpace(item.SourceTitle))
	switch {
	case strings.Contains(sourceType, "document") || strings.HasSuffix(sourceTitle, ".md") || strings.HasSuffix(sourceTitle, ".pdf") || strings.HasSuffix(sourceTitle, ".docx"):
		return DeepDrillRawSource
	case strings.Contains(sourceType, "transcript"):
		return DeepDrillRawTranscript
	case strings.Contains(sourceType, "draft"):
		return DeepDrillVersionedDraft
	case strings.Contains(sourceType, "note") || strings.Contains(sourceType, "chat_export") || strings.Contains(sourceType, "jsonl"):
		return DeepDrillDirectNote
	case strings.Contains(sourceType, "audio_overview"):
		return DeepDrillAudioOverview
	case strings.Contains(sourceType, "presentation") || strings.Contains(sourceType, "diagram") || strings.Contains(sourceType, "video_presentation"):
		return DeepDrillPresentationSumm
	case strings.Contains(sourceType, "minddrill") || strings.Contains(sourceType, "summary"):
		return DeepDrillDerivedSummary
	default:
		return DeepDrillUnknownSourceTier
	}
}

func scoreEvidenceProvenance(results []ResearchResult) (float64, DeepDrillSourceTier) {
	if len(results) == 0 {
		return 0, DeepDrillUnknownSourceTier
	}
	score := 0.0
	topTier := DeepDrillUnknownSourceTier
	for _, item := range results {
		tier := provenanceTierForResult(item)
		if compareSourceTier(tier, topTier) > 0 {
			topTier = tier
		}
		score += provenanceTierWeight(tier)
	}
	return score / float64(len(results)), topTier
}

func provenanceTierWeight(tier DeepDrillSourceTier) float64 {
	switch tier {
	case DeepDrillRawSource:
		return 1.0
	case DeepDrillRawTranscript:
		return 0.9
	case DeepDrillVersionedDraft:
		return 0.88
	case DeepDrillDirectNote:
		return 0.8
	case DeepDrillDerivedSummary:
		return 0.45
	case DeepDrillAudioOverview:
		return 0.42
	case DeepDrillPresentationSumm:
		return 0.4
	case DeepDrillAISynthesis:
		return 0.2
	default:
		return 0.25
	}
}

func compareSourceTier(a, b DeepDrillSourceTier) int {
	order := map[DeepDrillSourceTier]int{
		DeepDrillUnknownSourceTier: 0,
		DeepDrillAISynthesis:       1,
		DeepDrillPresentationSumm:  2,
		DeepDrillAudioOverview:     3,
		DeepDrillDerivedSummary:    4,
		DeepDrillDirectNote:        5,
		DeepDrillVersionedDraft:    6,
		DeepDrillRawTranscript:     7,
		DeepDrillRawSource:         8,
	}
	return order[a] - order[b]
}

func provenanceProfileNote(results []ResearchResult) string {
	if len(results) == 0 {
		return "provenance profile: no matches"
	}
	counts := map[DeepDrillSourceTier]int{}
	for _, item := range results {
		counts[provenanceTierForResult(item)]++
	}
	parts := make([]string, 0, len(counts))
	for tier, count := range counts {
		parts = append(parts, fmt.Sprintf("%s=%d", tier, count))
	}
	sort.Strings(parts)
	return "provenance profile: " + strings.Join(parts, ", ")
}

func isWeakProvenanceTier(tier DeepDrillSourceTier) bool {
	switch tier {
	case DeepDrillDerivedSummary, DeepDrillAudioOverview, DeepDrillPresentationSumm, DeepDrillAISynthesis:
		return true
	default:
		return false
	}
}

func (rt *MindDrillResearchRuntime) DeepDrillProvenanceTrace(ctx context.Context, req DeepDrillProvenanceTraceRequest) (DeepDrillProvenanceTraceResult, error) {
	if rt == nil || rt.client == nil {
		return DeepDrillProvenanceTraceResult{}, fmt.Errorf("minddrill research runtime is not configured")
	}
	if req.Limit <= 0 {
		req.Limit = 1400
	}
	if req.Limit > 12000 {
		req.Limit = 12000
	}
	trace := DeepDrillProvenanceTraceResult{Collection: strings.TrimSpace(req.Collection), Weaknesses: []string{}, Upstream: []DeepDrillProvenanceEdge{}, Downstream: []DeepDrillProvenanceEdge{}, RelatedSources: []DeepDrillProvenanceEdge{}}
	anchorSource := strings.TrimSpace(req.SourceID)
	if strings.TrimSpace(req.ThoughtID) != "" {
		thought, err := rt.DeepDrillShowThought(ctx, req.ThoughtID)
		if err != nil {
			return DeepDrillProvenanceTraceResult{}, err
		}
		trace.AnchorThoughtID = thought.ThoughtID
		trace.AnchorThoughtStatus = normalizeArtifactStatus(thought)
		if trace.Collection == "" {
			if candidate := strings.TrimSpace(asString(thought.QuerySpec["collection"])); candidate != "" {
				trace.Collection = candidate
			}
		}
		if anchorSource == "" {
			if len(thought.Sources) > 0 {
				anchorSource = strings.TrimSpace(thought.Sources[0])
			} else {
				anchorSource = strings.TrimSpace(asString(firstNonNil(thought.QuerySpec["source"], thought.QuerySpec["source_id"], thought.QuerySpec["source_document_id"], thought.QuerySpec["document_id"])))
			}
		}
	}
	if trace.Collection == "" {
		if trimmed := strings.TrimSpace(req.SessionID); trimmed != "" {
			_, collection, err := rt.requireBoundCollection(trimmed)
			if err != nil {
				return DeepDrillProvenanceTraceResult{}, err
			}
			trace.Collection = collection
		}
	}
	if trace.Collection == "" {
		return DeepDrillProvenanceTraceResult{}, fmt.Errorf("collection is required (pass --collection or --session)")
	}
	if anchorSource == "" {
		return DeepDrillProvenanceTraceResult{}, fmt.Errorf("source_id could not be determined")
	}
	nodes, err := rt.collectDeepDrillProvenanceNodes(ctx, trace.Collection, req.Limit)
	if err != nil {
		return DeepDrillProvenanceTraceResult{}, err
	}
	return buildDeepDrillProvenanceTrace(anchorSource, trace, nodes), nil
}

func (rt *MindDrillResearchRuntime) collectDeepDrillProvenanceNodes(ctx context.Context, collection string, limit int) ([]deepDrillProvenanceNode, error) {
	nodes := make([]deepDrillProvenanceNode, 0, limit)
	var offset any
	for len(nodes) < limit {
		pageSize := 256
		if remain := limit - len(nodes); remain < pageSize {
			pageSize = remain
		}
		page, next, err := rt.client.ScrollCollection(ctx, store.QdrantScrollRequest{Collection: collection, Limit: pageSize, Offset: offset})
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		for _, payload := range page {
			result := normalizeProvenance(collection, payload, 0)
			nodes = append(nodes, deepDrillNodeFromPayload(result, payload))
			if len(nodes) >= limit {
				break
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
	return nodes, nil
}

func buildDeepDrillProvenanceTrace(anchorSource string, trace DeepDrillProvenanceTraceResult, nodes []deepDrillProvenanceNode) DeepDrillProvenanceTraceResult {
	anchorSource = strings.TrimSpace(anchorSource)
	trace.Source = anchorSource
	if trace.Weaknesses == nil {
		trace.Weaknesses = []string{}
	}
	anchors := make([]deepDrillProvenanceNode, 0)
	for _, node := range nodes {
		if sameSourceIdentity(node, anchorSource) {
			anchors = append(anchors, node)
		}
	}
	if len(anchors) == 0 {
		trace.RelatedSources = append(trace.RelatedSources, DeepDrillProvenanceEdge{FromSourceID: anchorSource, ToSourceID: "", Relation: DeepDrillUnknownRelation, Evidence: "No matching source node found in collection metadata.", Confidence: 0.2, Basis: "no source id/path/title match", Explicit: false})
		trace.Weaknesses = append(trace.Weaknesses, "anchor source metadata not found in scanned collection")
		trace.Confidence = 0.2
		trace.Type = "unknown"
		return trace
	}
	anchor := bestAnchorNode(anchors)
	trace.Type = anchor.SourceType
	trace.Title = firstNonEmpty(anchor.SourceTitle, anchor.Filename, anchor.DocumentID)

	upstream := make([]DeepDrillProvenanceEdge, 0)
	downstream := make([]DeepDrillProvenanceEdge, 0)
	related := make([]DeepDrillProvenanceEdge, 0)
	chronologySignals := make([]string, 0)
	edgeConfidenceTotal := 0.0
	edgeCount := 0
	seenEdge := map[string]struct{}{}
	hasOriginalChronology := false
	hasOnlyIngestionChronology := false
	familyMembers := map[string]int{}

	anchorIDs := uniqueTrimmed(append([]string{anchor.SourceID, anchor.DocumentID, anchor.MemoryID, anchor.ChunkID}, append(anchor.DerivedFrom, anchor.CitedSources...)...))
	for _, node := range nodes {
		nodeID := firstNonEmpty(node.SourceID, node.DocumentID, node.MemoryID, node.ChunkID)
		if nodeID == "" {
			continue
		}
		if node.FamilyID != "" {
			familyMembers[node.FamilyID]++
		}
		if sameSourceIdentity(node, anchorSource) {
			if chronologyEdge, ok := chronologyWithinSource(anchor, node); ok {
				if addUniqueEdge(&related, chronologyEdge, seenEdge) {
					edgeConfidenceTotal += chronologyEdge.Confidence
					edgeCount++
				}
				chronologySignals = append(chronologySignals, chronologyEdge.Basis)
				if strings.Contains(strings.ToLower(chronologyEdge.Basis), "original timestamp") || strings.Contains(strings.ToLower(chronologyEdge.Basis), "chunk") || strings.Contains(strings.ToLower(chronologyEdge.Basis), "segment") || strings.Contains(strings.ToLower(chronologyEdge.Basis), "turn") {
					hasOriginalChronology = true
				}
			}
			continue
		}
		if intersects(node.DerivedFrom, anchorIDs) || intersects([]string{node.ParentSource}, anchorIDs) {
			edge := DeepDrillProvenanceEdge{FromSourceID: nodeID, ToSourceID: anchor.SourceID, Relation: DeepDrillDerivedFrom, Evidence: "payload contains derived-from or parent-source reference to anchor", Confidence: 0.92, Basis: "explicit metadata keys: derived_from/parent_source", Explicit: true}
			if addUniqueEdge(&upstream, edge, seenEdge) {
				edgeConfidenceTotal += edge.Confidence
				edgeCount++
			}
		}
		if intersects(anchor.DerivedFrom, []string{nodeID, node.DocumentID}) || strings.EqualFold(anchor.ParentSource, nodeID) {
			edge := DeepDrillProvenanceEdge{FromSourceID: anchor.SourceID, ToSourceID: nodeID, Relation: DeepDrillDerivedFrom, Evidence: "anchor metadata cites this source as parent/derived input", Confidence: 0.9, Basis: "explicit metadata keys on anchor", Explicit: true}
			if addUniqueEdge(&downstream, edge, seenEdge) {
				edgeConfidenceTotal += edge.Confidence
				edgeCount++
			}
		}
		if intersects(node.CitedSources, anchorIDs) {
			edge := DeepDrillProvenanceEdge{FromSourceID: nodeID, ToSourceID: anchor.SourceID, Relation: DeepDrillCites, Evidence: "payload citations include anchor source identifier", Confidence: 0.88, Basis: "explicit citation identifiers", Explicit: true}
			if addUniqueEdge(&upstream, edge, seenEdge) {
				edgeConfidenceTotal += edge.Confidence
				edgeCount++
			}
		}
		if intersects(anchor.CitedSources, []string{nodeID, node.DocumentID}) {
			edge := DeepDrillProvenanceEdge{FromSourceID: anchor.SourceID, ToSourceID: nodeID, Relation: DeepDrillCites, Evidence: "anchor cites this source identifier", Confidence: 0.88, Basis: "explicit citation identifiers", Explicit: true}
			if addUniqueEdge(&downstream, edge, seenEdge) {
				edgeConfidenceTotal += edge.Confidence
				edgeCount++
			}
		}
		if node.FamilyID != "" && node.FamilyID == anchor.FamilyID {
			relation := DeepDrillSameSourceFamily
			if isDerivedLikeType(node.SourceType) {
				relation = DeepDrillSummarizes
			} else if isTranscriptLikeType(node.SourceType) {
				relation = DeepDrillTranscribes
			} else if isPresentationLikeType(node.SourceType) {
				relation = DeepDrillPresents
			}
			edge := DeepDrillProvenanceEdge{FromSourceID: anchor.SourceID, ToSourceID: nodeID, Relation: relation, Evidence: "source titles/filenames normalize to same family key", Confidence: 0.58, Basis: "inferred shared source family", Explicit: false}
			if relation != DeepDrillSameSourceFamily {
				edge.Confidence = 0.66
			}
			if addUniqueEdge(&related, edge, seenEdge) {
				edgeConfidenceTotal += edge.Confidence
				edgeCount++
			}
		}
		if chronologyEdge, ok := chronologyCrossSource(anchor, node); ok {
			if addUniqueEdge(&related, chronologyEdge, seenEdge) {
				edgeConfidenceTotal += chronologyEdge.Confidence
				edgeCount++
			}
			chronologySignals = append(chronologySignals, chronologyEdge.Basis)
			if strings.Contains(strings.ToLower(chronologyEdge.Basis), "original timestamp") {
				hasOriginalChronology = true
			}
		}
		if !node.OriginalTimestamp.IsZero() && !anchor.OriginalTimestamp.IsZero() {
			hasOriginalChronology = true
		}
		if node.OriginalTimestamp.IsZero() && !node.IngestionTimestamp.IsZero() {
			hasOnlyIngestionChronology = true
		}
	}
	if len(upstream) == 0 && len(downstream) == 0 && len(related) == 0 {
		related = append(related, DeepDrillProvenanceEdge{FromSourceID: anchor.SourceID, ToSourceID: "", Relation: DeepDrillUnknownRelation, Evidence: "No explicit or inferable link found among scanned metadata records.", Confidence: 0.24, Basis: "missing citation/parent/family linkage", Explicit: false})
	}
	if !hasOriginalChronology {
		if hasOnlyIngestionChronology {
			trace.Weaknesses = append(trace.Weaknesses, "only ingestion timestamps were available; intellectual chronology not inferred from ingestion order")
		} else {
			trace.Weaknesses = append(trace.Weaknesses, "no original chronology metadata (chunk/turn/segment/original timestamp) found")
		}
	}
	if strings.TrimSpace(anchor.SourceType) == "" {
		trace.Weaknesses = append(trace.Weaknesses, "source_type missing on anchor")
	}
	if anchor.FamilyID == "" {
		trace.Weaknesses = append(trace.Weaknesses, "source family key missing on anchor")
	}
	if anchor.FamilyID != "" && familyMembers[anchor.FamilyID] > 1 {
		trace.RelatedSources = append(trace.RelatedSources, DeepDrillProvenanceEdge{FromSourceID: anchor.SourceID, ToSourceID: anchor.FamilyID, Relation: DeepDrillSameSourceFamily, Evidence: fmt.Sprintf("%d records share this source family key", familyMembers[anchor.FamilyID]), Confidence: 0.63, Basis: "duplicate source family detection", Explicit: false})
	}
	trace.Upstream = upstream
	trace.Downstream = downstream
	trace.RelatedSources = append(trace.RelatedSources, related...)
	trace.ChronologySignals = uniqueTrimmed(chronologySignals)

	base := provenanceTierWeight(sourceTierFromNode(anchor))
	if edgeCount > 0 {
		trace.Confidence = clamp01((base * 0.55) + ((edgeConfidenceTotal / float64(edgeCount)) * 0.45))
	} else {
		trace.Confidence = clamp01(base * 0.65)
	}
	if len(trace.Weaknesses) > 0 {
		trace.Confidence = clamp01(trace.Confidence - (0.05 * float64(len(trace.Weaknesses))))
	}
	return trace
}

func deepDrillNodeFromPayload(result ResearchResult, payload map[string]any) deepDrillProvenanceNode {
	node := deepDrillProvenanceNode{
		SourceID:           firstNonEmpty(result.SourceFingerprint, result.SourceDocumentID, result.Path, result.Filename, result.SourceTitle),
		DocumentID:         strings.TrimSpace(result.SourceDocumentID),
		ConversationID:     strings.TrimSpace(asString(firstPayloadValue(payload, []string{"conversation_id", "session_id"}))),
		SourceType:         strings.TrimSpace(result.SourceType),
		Filename:           strings.TrimSpace(result.Filename),
		SourceTitle:        strings.TrimSpace(result.SourceTitle),
		MemoryID:           strings.TrimSpace(result.MemoryID),
		ChunkID:            strings.TrimSpace(result.ChunkID),
		ChunkIndex:         result.ChunkIndex,
		SegmentStart:       asFloat(firstPayloadValue(payload, []string{"start", "segment_start", "start_seconds"})),
		SegmentEnd:         asFloat(firstPayloadValue(payload, []string{"end", "segment_end", "end_seconds"})),
		OriginalTimestamp:  parseAnyTime(firstPayloadValue(payload, []string{"timestamp", "source_timestamp", "event_timestamp"})),
		IngestionTimestamp: parseAnyTime(firstPayloadValue(payload, []string{"created_at", "ingested_at", "modified_at"})),
		DerivedFrom: uniqueTrimmed(asStringSlice(firstNonNil(
			firstPayloadValue(payload, []string{"derived_from"}),
			firstPayloadValue(payload, []string{"source_memory_ids"}),
			firstPayloadValue(payload, []string{"used_memory_ids"}),
		))),
		CitedSources: uniqueTrimmed(asStringSlice(firstNonNil(
			firstPayloadValue(payload, []string{"cited_source_ids"}),
			firstPayloadValue(payload, []string{"citations"}),
			firstPayloadValue(payload, []string{"source_quotes"}),
		))),
		ParentSource: strings.TrimSpace(asString(firstNonNil(
			firstPayloadValue(payload, []string{"parent_source_id"}),
			firstPayloadValue(payload, []string{"source_parent_id"}),
			firstPayloadValue(payload, []string{"upstream_source_id"}),
		))),
	}
	if node.ChunkIndex <= 0 {
		node.ChunkIndex = asInt(firstPayloadValue(payload, []string{"chunk_index", "chunk_idx", "position", "segment_index"}))
	}
	node.TurnIndex = parseTurnIndex(node.MemoryID)
	node.FamilyID = sourceFamilyKey(firstNonEmpty(node.SourceTitle, node.Filename, result.Path, node.SourceID))
	return node
}

func parseTurnIndex(memoryID string) int {
	trimmed := strings.TrimSpace(memoryID)
	if trimmed == "" {
		return 0
	}
	idx := strings.Index(trimmed, "turn_")
	if idx == -1 {
		return 0
	}
	start := idx + len("turn_")
	end := start
	for end < len(trimmed) {
		c := trimmed[end]
		if c < '0' || c > '9' {
			break
		}
		end++
	}
	if end <= start {
		return 0
	}
	parsed, err := strconv.Atoi(trimmed[start:end])
	if err != nil {
		return 0
	}
	return parsed
}

func sourceFamilyKey(seed string) string {
	trimmed := strings.ToLower(strings.TrimSpace(seed))
	if trimmed == "" {
		return ""
	}
	replacements := []string{"(1)", "(2)", " copy", "_copy", "-copy", " copy 1"}
	for _, item := range replacements {
		trimmed = strings.ReplaceAll(trimmed, item, "")
	}
	var b strings.Builder
	for _, r := range trimmed {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func sameSourceIdentity(node deepDrillProvenanceNode, anchor string) bool {
	anchor = strings.TrimSpace(anchor)
	if anchor == "" {
		return false
	}
	for _, candidate := range []string{node.SourceID, node.DocumentID, node.Filename, node.SourceTitle, node.MemoryID, node.ChunkID} {
		if strings.EqualFold(strings.TrimSpace(candidate), anchor) {
			return true
		}
	}
	return false
}

func bestAnchorNode(anchors []deepDrillProvenanceNode) deepDrillProvenanceNode {
	best := anchors[0]
	for _, candidate := range anchors[1:] {
		if scoreAnchorNode(candidate) > scoreAnchorNode(best) {
			best = candidate
		}
	}
	return best
}

func scoreAnchorNode(node deepDrillProvenanceNode) int {
	score := 0
	if node.ChunkIndex > 0 {
		score += 2
	}
	if node.TurnIndex > 0 {
		score += 2
	}
	if !node.OriginalTimestamp.IsZero() {
		score += 3
	}
	if strings.TrimSpace(node.ParentSource) != "" {
		score += 2
	}
	if len(node.CitedSources) > 0 {
		score += 1
	}
	if len(node.DerivedFrom) > 0 {
		score += 1
	}
	return score
}

func chronologyWithinSource(anchor, peer deepDrillProvenanceNode) (DeepDrillProvenanceEdge, bool) {
	if firstNonEmpty(anchor.MemoryID, anchor.ChunkID) == firstNonEmpty(peer.MemoryID, peer.ChunkID) {
		return DeepDrillProvenanceEdge{}, false
	}
	if anchor.ChunkIndex > 0 && peer.ChunkIndex > 0 {
		relation := DeepDrillEarlierInSource
		if peer.ChunkIndex > anchor.ChunkIndex {
			relation = DeepDrillLaterInSource
		}
		return DeepDrillProvenanceEdge{FromSourceID: anchor.SourceID, ToSourceID: peer.SourceID, Relation: relation, Evidence: fmt.Sprintf("chunk ordering %d vs %d", anchor.ChunkIndex, peer.ChunkIndex), Confidence: 0.82, Basis: "chunk_index ordering within source", Explicit: false}, true
	}
	if anchor.TurnIndex > 0 && peer.TurnIndex > 0 {
		relation := DeepDrillEarlierInSource
		if peer.TurnIndex > anchor.TurnIndex {
			relation = DeepDrillLaterInSource
		}
		return DeepDrillProvenanceEdge{FromSourceID: anchor.SourceID, ToSourceID: peer.SourceID, Relation: relation, Evidence: fmt.Sprintf("turn ordering %d vs %d", anchor.TurnIndex, peer.TurnIndex), Confidence: 0.77, Basis: "turn index ordering in memory id", Explicit: false}, true
	}
	if anchor.SegmentEnd > 0 && peer.SegmentStart > 0 {
		relation := DeepDrillEarlierInSource
		if peer.SegmentStart > anchor.SegmentEnd {
			relation = DeepDrillLaterInSource
		}
		return DeepDrillProvenanceEdge{FromSourceID: anchor.SourceID, ToSourceID: peer.SourceID, Relation: relation, Evidence: fmt.Sprintf("segment offsets %.2f-%.2f vs %.2f-%.2f", anchor.SegmentStart, anchor.SegmentEnd, peer.SegmentStart, peer.SegmentEnd), Confidence: 0.79, Basis: "segment offset ordering", Explicit: false}, true
	}
	if !anchor.OriginalTimestamp.IsZero() && !peer.OriginalTimestamp.IsZero() {
		relation := DeepDrillEarlierInSource
		if peer.OriginalTimestamp.After(anchor.OriginalTimestamp) {
			relation = DeepDrillLaterInSource
		}
		return DeepDrillProvenanceEdge{FromSourceID: anchor.SourceID, ToSourceID: peer.SourceID, Relation: relation, Evidence: fmt.Sprintf("original timestamps %s vs %s", anchor.OriginalTimestamp.Format(time.RFC3339), peer.OriginalTimestamp.Format(time.RFC3339)), Confidence: 0.74, Basis: "original timestamp ordering", Explicit: false}, true
	}
	return DeepDrillProvenanceEdge{}, false
}

func chronologyCrossSource(anchor, node deepDrillProvenanceNode) (DeepDrillProvenanceEdge, bool) {
	if anchor.FamilyID == "" || node.FamilyID == "" || anchor.FamilyID != node.FamilyID {
		return DeepDrillProvenanceEdge{}, false
	}
	if anchor.OriginalTimestamp.IsZero() || node.OriginalTimestamp.IsZero() {
		return DeepDrillProvenanceEdge{}, false
	}
	relation := DeepDrillEarlierInSource
	if node.OriginalTimestamp.After(anchor.OriginalTimestamp) {
		relation = DeepDrillLaterInSource
	}
	return DeepDrillProvenanceEdge{FromSourceID: anchor.SourceID, ToSourceID: firstNonEmpty(node.SourceID, node.DocumentID), Relation: relation, Evidence: fmt.Sprintf("same family with original timestamps %s vs %s", anchor.OriginalTimestamp.Format(time.RFC3339), node.OriginalTimestamp.Format(time.RFC3339)), Confidence: 0.69, Basis: "same-family original timestamp signal", Explicit: false}, true
}

func intersects(a []string, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, item := range a {
		trimmed := strings.ToLower(strings.TrimSpace(item))
		if trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
	for _, item := range b {
		trimmed := strings.ToLower(strings.TrimSpace(item))
		if trimmed == "" {
			continue
		}
		if _, ok := set[trimmed]; ok {
			return true
		}
	}
	return false
}

func addUniqueEdge(target *[]DeepDrillProvenanceEdge, edge DeepDrillProvenanceEdge, seen map[string]struct{}) bool {
	key := strings.Join([]string{edge.FromSourceID, edge.ToSourceID, string(edge.Relation), edge.Basis}, "||")
	if _, ok := seen[key]; ok {
		return false
	}
	seen[key] = struct{}{}
	*target = append(*target, edge)
	return true
}

func sourceTierFromNode(node deepDrillProvenanceNode) DeepDrillSourceTier {
	return provenanceTierForResult(ResearchResult{SourceType: node.SourceType, SourceTitle: firstNonEmpty(node.SourceTitle, node.Filename)})
}

func isDerivedLikeType(sourceType string) bool {
	lower := strings.ToLower(strings.TrimSpace(sourceType))
	return strings.Contains(lower, "summary") || strings.Contains(lower, "overview")
}

func isTranscriptLikeType(sourceType string) bool {
	lower := strings.ToLower(strings.TrimSpace(sourceType))
	return strings.Contains(lower, "transcript") || strings.Contains(lower, "audio")
}

func isPresentationLikeType(sourceType string) bool {
	lower := strings.ToLower(strings.TrimSpace(sourceType))
	return strings.Contains(lower, "presentation") || strings.Contains(lower, "slide") || strings.Contains(lower, "diagram")
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			switch typed := value.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					return value
				}
			default:
				return value
			}
		}
	}
	return nil
}
