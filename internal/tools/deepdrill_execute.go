package tools

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

func (rt *MindDrillResearchRuntime) executeDeepDrillStep(ctx context.Context, plan DeepDrillPlan) (DeepDrillStepResult, *DeepDrillState, error) {
	session, collection, err := rt.requireBoundCollection(plan.SessionID)
	if err != nil {
		return DeepDrillStepResult{}, nil, err
	}
	state := session.DeepDrill
	if state == nil {
		state = newDefaultDeepDrillState(rt.deepDrillThoughts)
	}
	if state.BranchStatus == DeepDrillFrozen {
		if !shouldReopenFrozenBranch(state, plan.SelectedQuestion, plan.Collection) {
			// Terminal: no new research work. Report the current frozen state
			// without fabricating an iteration or a duplicate freeze artifact.
			state.LastUpdated = time.Now().UTC()
			session.DeepDrill = state
			rt.sessions.put(session)
			return DeepDrillStepResult{Step: state.Iterations, Uncertainty: DeepDrillDiminishingReturns, Question: plan.SelectedQuestion, Strategy: DeepDrillFreezeBranch, EvidenceCount: 0, ThoughtCount: 0, InfoGain: DeepDrillInfoLow, BranchStatus: state.BranchStatus, NegativeResult: true, SelectedSourceTier: state.LastSeenProvenanceTier, Terminal: true}, state, nil
		}
		state.BranchStatus = DeepDrillOpen
		state.FrozenReason = ""
		reopenThought := DeepDrillThought{Type: DeepDrillThoughtReopenDecision, Question: plan.SelectedQuestion, EvidenceSummary: "Frozen branch reopened because a novel question or collection context became available.", EvidenceOrigin: DeepDrillResearchThought, Status: "open"}
		if _, _, err := rt.storeDeepDrillThought(ctx, session, state, reopenThought); err != nil {
			return DeepDrillStepResult{}, nil, err
		}
	}

	modelBefore := cloneHypotheses(state.Hypotheses)
	evidence, warnings, err := rt.runDeepDrillStrategy(ctx, collection, state, plan.SelectedStrategy, plan.SelectedQuestion)
	if err != nil {
		return DeepDrillStepResult{}, nil, err
	}
	for idx := range evidence {
		if evidence[idx].RetrievalMetadata == nil {
			evidence[idx].RetrievalMetadata = map[string]any{}
		}
		evidence[idx].RetrievalMetadata["evidence_origin"] = string(DeepDrillSourceCorpus)
	}
	provenanceScore, topTier := scoreEvidenceProvenance(evidence)
	if state.LastSeenProvenanceTier == "" || compareSourceTier(topTier, state.LastSeenProvenanceTier) > 0 {
		state.LastSeenProvenanceTier = topTier
	}
	negativeResult := len(evidence) == 0
	thoughtCount := 0
	duplicateEvidence := false

	questionThought := DeepDrillThought{
		Type:             DeepDrillThoughtQuestion,
		Question:         plan.SelectedQuestion,
		UncertaintyClass: plan.TopUncertainty,
		Strategy:         plan.SelectedStrategy,
		QuerySpec:        map[string]any{"collection": collection},
		EvidenceSummary:  "DeepDrill selected this question because it is expected to best discriminate between active hypotheses.",
		EvidenceOrigin:   DeepDrillResearchThought,
		Status:           "planned",
	}
	if _, isDup, err := rt.storeDeepDrillThought(ctx, session, state, questionThought); err != nil {
		return DeepDrillStepResult{}, nil, err
	} else {
		duplicateEvidence = duplicateEvidence || isDup
		thoughtCount++
	}

	strategySummary := strings.TrimSpace(strings.Join(warnings, "; "))
	if strategySummary == "" {
		strategySummary = "Strategy executed successfully."
	}
	strategyThought := DeepDrillThought{
		Type:             DeepDrillThoughtStrategyRun,
		Question:         plan.SelectedQuestion,
		UncertaintyClass: plan.TopUncertainty,
		Strategy:         plan.SelectedStrategy,
		QuerySpec:        map[string]any{"collection": collection, "strategy": plan.SelectedStrategy},
		EvidenceSummary:  strategySummary,
		EvidenceOrigin:   DeepDrillResearchThought,
		Status:           "executed",
	}
	if _, isDup, err := rt.storeDeepDrillThought(ctx, session, state, strategyThought); err != nil {
		return DeepDrillStepResult{}, nil, err
	} else {
		duplicateEvidence = duplicateEvidence || isDup
		thoughtCount++
	}

	if negativeResult {
		negativeThought := DeepDrillThought{
			Type:             DeepDrillThoughtNegativeResult,
			Question:         plan.SelectedQuestion,
			UncertaintyClass: plan.TopUncertainty,
			Strategy:         plan.SelectedStrategy,
			EvidenceSummary:  "No occurrence was located within the searched corpus for this strategy run.",
			EvidenceOrigin:   DeepDrillSourceCorpus,
			Status:           "negative",
			NextAction:       "decrease branch priority or change strategy class",
		}
		if _, isDup, err := rt.storeDeepDrillThought(ctx, session, state, negativeThought); err != nil {
			return DeepDrillStepResult{}, nil, err
		} else {
			duplicateEvidence = duplicateEvidence || isDup
			thoughtCount++
		}
	} else {
		for _, item := range sampleResults(evidence, 3) {
			evidenceThought := DeepDrillThought{
				Type:              DeepDrillThoughtEvidence,
				Question:          plan.SelectedQuestion,
				UncertaintyClass:  plan.TopUncertainty,
				Strategy:          plan.SelectedStrategy,
				Sources:           uniqueTrimmed([]string{item.SourceFingerprint, item.SourceTitle, item.SourceDocumentID}),
				EvidenceSummary:   summarizeResearchResult(item),
				ProvenanceScore:   provenanceScore,
				EvidenceStrength:  clamp01(item.SimilarityScore),
				ContradictionFlag: hasContradictionSignal(item.Text),
				EvidenceOrigin:    DeepDrillSourceCorpus,
				Status:            "retrieved",
			}
			if _, isDup, err := rt.storeDeepDrillThought(ctx, session, state, evidenceThought); err != nil {
				return DeepDrillStepResult{}, nil, err
			} else {
				duplicateEvidence = duplicateEvidence || isDup
				thoughtCount++
			}
		}
	}

	hasContradiction := false
	for _, item := range evidence {
		if hasContradictionSignal(item.Text) {
			hasContradiction = true
			break
		}
	}
	evidenceFingerprint := strategyEvidenceFingerprint(plan.SelectedStrategy, plan.SelectedQuestion, evidence)
	repeatRun := isRepeatStrategyFinding(state, plan.SelectedStrategy, plan.SelectedQuestion, evidenceFingerprint)
	duplicateEvidence = duplicateEvidence || repeatRun
	if plan.SelectedStrategy == DeepDrillFreezeBranch {
		state.BranchStatus = DeepDrillFrozen
		state.FrozenReason = "diminishing returns: DeepDrill selected freeze policy"
	} else if !repeatRun {
		applyModelUpdate(state, plan.TopUncertainty, evidence, provenanceScore)
	}
	modelAfter := cloneHypotheses(state.Hypotheses)
	infoGain := scoreInformationGain(modelBefore, modelAfter, state, len(evidence), duplicateEvidence)

	diminishing := infoGain == DeepDrillInfoLow || duplicateEvidence
	if diminishing {
		state.ConsecutiveLowGain++
	} else {
		state.ConsecutiveLowGain = 0
	}

	reclassified := false
	notFreezeStrategy := plan.SelectedStrategy != DeepDrillFreezeBranch
	if diminishing && notFreezeStrategy {
		markStrategyExhausted(state, plan.TopUncertainty, plan.SelectedStrategy)
	}

	remaining, exhausted := strategyAvailability(state, plan.TopUncertainty)
	state.StrategiesRemaining = remaining
	state.StrategiesExhausted = exhausted

	// Moving to a fresh strategy within the same uncertainty is a strategy
	// switch, not a repeated failure, so reset the consecutive low-gain counter.
	if diminishing && len(remaining) > 0 {
		state.ConsecutiveLowGain = 0
	}

	// Only reclassify once the current uncertainty class is fully exhausted.
	if notFreezeStrategy && len(remaining) == 0 {
		if newUncertainty, reason, changed := reclassifyUncertainty(state, evidence, provenanceScore, topTier, hasContradiction, plan.SelectedQuestion); changed {
			if r2, e2 := strategyAvailability(state, newUncertainty); len(r2) > 0 {
				state.PreviousUncertainty = plan.TopUncertainty
				state.ReclassifiedUncertainty = newUncertainty
				state.ReclassificationReason = reason
				state.StrategiesRemaining = r2
				state.StrategiesExhausted = e2
				state.CurrentUncertainty = newUncertainty
				state.ConsecutiveLowGain = 0
				reclassified = true
			}
		}
	}

	state.Iterations++
	state.LastUpdated = time.Now().UTC()
	state.StrategyHistory = append(state.StrategyHistory, DeepDrillStrategyOutcome{Strategy: plan.SelectedStrategy, Uncertainty: plan.TopUncertainty, Question: plan.SelectedQuestion, InfoGain: infoGain, DuplicateEvidence: duplicateEvidence, EvidenceCount: len(evidence), EvidenceFingerprint: evidenceFingerprint, Timestamp: time.Now().UTC()})

	if detectStrategyCycle(state.StrategyHistory) {
		state.BranchStatus = DeepDrillFrozen
		state.FrozenReason = "scheduler-level diminishing returns: repeating uncertainty/strategy/evidence cycle detected"
	} else if notFreezeStrategy && !reclassified && len(remaining) == 0 {
		state.BranchStatus = DeepDrillFrozen
		state.FrozenReason = "all strategies exhausted and no actionable reclassification remains"
	} else if !reclassified && state.ConsecutiveLowGain >= rt.deepDrillFreezeAfterLow {
		state.BranchStatus = DeepDrillFrozen
		state.FrozenReason = fmt.Sprintf("%d consecutive low-gain runs", state.ConsecutiveLowGain)
	}

	// Emit exactly one terminal freeze artifact on the transition to FROZEN.
	if state.BranchStatus == DeepDrillFrozen {
		stopThought := DeepDrillThought{
			Type:             DeepDrillThoughtStopDecision,
			Question:         plan.SelectedQuestion,
			UncertaintyClass: DeepDrillDiminishingReturns,
			Strategy:         DeepDrillFreezeBranch,
			EvidenceSummary:  "Branch frozen due to diminishing returns under current corpus and strategy set.",
			InfoGain:         DeepDrillInfoLow,
			Status:           "frozen",
			NextAction:       "wait for new provenance tier or contradictory evidence",
			EvidenceOrigin:   DeepDrillResearchThought,
		}
		if _, _, err := rt.storeDeepDrillThought(ctx, session, state, stopThought); err != nil {
			return DeepDrillStepResult{}, nil, err
		}
		thoughtCount++
	}

	if reclassified {
		reclassifyThought := DeepDrillThought{
			Type:                    DeepDrillThoughtReclassify,
			Question:                plan.SelectedQuestion,
			UncertaintyClass:        plan.TopUncertainty,
			Strategy:                plan.SelectedStrategy,
			EvidenceSummary:         "DeepDrill reclassified the unresolved question to a more appropriate uncertainty class before freezing.",
			PreviousUncertainty:     state.PreviousUncertainty,
			ReclassifiedUncertainty: state.ReclassifiedUncertainty,
			ReclassificationReason:  state.ReclassificationReason,
			StrategiesRemaining:     state.StrategiesRemaining,
			StrategiesExhausted:     state.StrategiesExhausted,
			EvidenceOrigin:          DeepDrillResearchThought,
			Status:                  "reclassified",
			NextAction:              "run strategy for reclassified uncertainty",
		}
		if _, _, err := rt.storeDeepDrillThought(ctx, session, state, reclassifyThought); err != nil {
			return DeepDrillStepResult{}, nil, err
		}
		thoughtCount++
	}

	revisionThought := DeepDrillThought{
		Type:             DeepDrillThoughtModelRevision,
		Question:         plan.SelectedQuestion,
		UncertaintyClass: plan.TopUncertainty,
		Strategy:         plan.SelectedStrategy,
		EvidenceSummary:  "DeepDrill updated competing hypothesis state and confidence after evaluating this run.",
		ModelBefore:      exportHypothesisModel(modelBefore),
		ModelAfter:       exportHypothesisModel(modelAfter),
		InfoGain:         infoGain,
		ProvenanceScore:  provenanceScore,
		EvidenceOrigin:   DeepDrillResearchThought,
		Status:           strings.ToLower(string(state.BranchStatus)),
		NextAction:       nextActionFromState(state),
	}
	if reclassified {
		revisionThought.PreviousUncertainty = state.PreviousUncertainty
		revisionThought.ReclassifiedUncertainty = state.ReclassifiedUncertainty
		revisionThought.ReclassificationReason = state.ReclassificationReason
		revisionThought.StrategiesRemaining = state.StrategiesRemaining
		revisionThought.StrategiesExhausted = state.StrategiesExhausted
	}
	if _, _, err := rt.storeDeepDrillThought(ctx, session, state, revisionThought); err != nil {
		return DeepDrillStepResult{}, nil, err
	}
	thoughtCount++

	session.DeepDrill = state
	rt.sessions.put(session)
	return DeepDrillStepResult{Step: state.Iterations, Uncertainty: plan.TopUncertainty, Question: plan.SelectedQuestion, Strategy: plan.SelectedStrategy, EvidenceCount: len(evidence), ThoughtCount: thoughtCount, InfoGain: infoGain, BranchStatus: state.BranchStatus, DuplicateEvidence: duplicateEvidence, NegativeResult: negativeResult, SelectedSourceTier: topTier}, state, nil
}

func (rt *MindDrillResearchRuntime) prepareDeepDrillSession(ctx context.Context, req DeepDrillPlanRequest) (ResearchSession, string, *DeepDrillState, error) {
	session, err := rt.ensureSession(req.SessionID)
	if err != nil {
		return ResearchSession{}, "", nil, err
	}
	if trimmed := strings.TrimSpace(req.Collection); trimmed != "" {
		if _, _, err := rt.ensureEmbeddingCompatibility(ctx, trimmed); err != nil {
			return ResearchSession{}, "", nil, err
		}
		session.TargetCollection = trimmed
	}
	if strings.TrimSpace(session.TargetCollection) == "" {
		return ResearchSession{}, "", nil, fmt.Errorf("session %q is not bound to a collection. Run bind_research_collection first", req.SessionID)
	}
	if trimmed := strings.TrimSpace(req.ResearchQuestion); trimmed != "" {
		session.ResearchQuestion = trimmed
	}
	state := session.DeepDrill
	if state == nil {
		state = newDefaultDeepDrillState(rt.deepDrillThoughts)
	}
	if len(req.Hypotheses) > 0 {
		state.Hypotheses = normalizeHypothesisSeeds(req.Hypotheses)
	} else if len(state.Hypotheses) == 0 {
		state.Hypotheses = normalizeHypothesesFromStrings(session.Hypotheses)
	}
	if len(req.KnownBlockers) > 0 {
		state.KnownBlockers = normalizeUncertainties(req.KnownBlockers)
	}
	if strings.TrimSpace(req.HighestValueQuestion) != "" {
		state.HighestValueQuestion = strings.TrimSpace(req.HighestValueQuestion)
	}
	if req.ForceReopen && state.BranchStatus == DeepDrillFrozen {
		state.BranchStatus = DeepDrillOpen
		state.FrozenReason = ""
	}
	state.LastUpdated = time.Now().UTC()
	session.DeepDrill = state
	rt.sessions.put(session)
	return session, session.TargetCollection, state, nil
}

func (rt *MindDrillResearchRuntime) runDeepDrillStrategy(ctx context.Context, collection string, state *DeepDrillState, strategy DeepDrillStrategy, question string) ([]ResearchResult, []string, error) {
	warnings := []string{}
	switch strategy {
	case DeepDrillSemanticBroad:
		results, runWarnings, err := rt.semanticSearch(ctx, collection, question, 10, nil, nil, 1, false)
		warnings = append(warnings, runWarnings...)
		return results, warnings, err
	case DeepDrillKeywordTargeted:
		results, runWarnings, err := rt.keywordSearch(ctx, collection, question, 10, nil, nil)
		warnings = append(warnings, runWarnings...)
		return results, warnings, err
	case DeepDrillDocumentExpansion:
		docID := ""
		if len(state.Hypotheses) > 0 && len(state.Hypotheses[0].SupportingEvidenceIDs) > 0 {
			docID = state.Hypotheses[0].SupportingEvidenceIDs[0]
		}
		if docID == "" {
			warnings = append(warnings, "document expansion skipped because no anchor document id exists yet")
			return nil, warnings, nil
		}
		results, err := rt.getDocumentChunks(ctx, collection, docID, 12)
		return results, warnings, err
	case DeepDrillCrossDocument:
		excluded := []string{}
		if len(state.Hypotheses) > 0 {
			excluded = append(excluded, state.Hypotheses[0].SupportingEvidenceIDs...)
		}
		results, runWarnings, err := rt.semanticSearch(ctx, collection, question, 10, nil, uniqueTrimmed(excluded), 0, false)
		warnings = append(warnings, runWarnings...)
		return results, warnings, err
	case DeepDrillInspectCollection:
		report, err := rt.inspectCollection(ctx, collection, 600)
		if err != nil {
			return nil, warnings, err
		}
		warnings = append(warnings, fmt.Sprintf("collection points=%d documents=%d missing_provenance=%d", report.ChunksOrVectors, report.DocumentsDetected, len(report.MissingProvenanceFields)))
		return nil, warnings, nil
	case DeepDrillNeighborChunks:
		if len(state.Hypotheses) == 0 || len(state.Hypotheses[0].SupportingEvidenceIDs) == 0 {
			warnings = append(warnings, "neighbor chunk trace skipped because no anchor chunk id exists")
			return nil, warnings, nil
		}
		results, err := rt.getNeighborChunks(ctx, collection, state.Hypotheses[0].SupportingEvidenceIDs[0], 2)
		return results, warnings, err
	case DeepDrillPartitionTest:
		results, runWarnings, err := rt.keywordSearch(ctx, collection, "E8 sacred translator bridge", 25, nil, nil)
		warnings = append(warnings, runWarnings...)
		return results, warnings, err
	case DeepDrillNegativeControl:
		results, runWarnings, err := rt.keywordSearch(ctx, collection, "generic structure framework architecture", 12, nil, nil)
		warnings = append(warnings, runWarnings...)
		return results, warnings, err
	case DeepDrillChronologyTrace:
		results, runWarnings, err := rt.keywordSearch(ctx, collection, question+" earliest first", 12, nil, nil)
		warnings = append(warnings, runWarnings...)
		return sortByCreation(results), warnings, err
	case DeepDrillTimestampTrace:
		results, runWarnings, err := rt.keywordSearch(ctx, collection, question+" timestamp draft", 12, nil, nil)
		warnings = append(warnings, runWarnings...)
		return sortByCreation(results), warnings, err
	case DeepDrillProvenanceProfile:
		results, runWarnings, err := rt.semanticSearch(ctx, collection, question, 12, nil, nil, 0, false)
		warnings = append(warnings, runWarnings...)
		warnings = append(warnings, provenanceProfileNote(results))
		return results, warnings, err
	case DeepDrillProvenanceTrace:
		results, runWarnings, err := rt.semanticSearch(ctx, collection, question, 12, nil, nil, 0, false)
		warnings = append(warnings, runWarnings...)
		if err != nil {
			return nil, warnings, err
		}
		if len(results) == 0 {
			warnings = append(warnings, "provenance trace found no anchor candidates")
			return results, warnings, nil
		}
		anchor := firstNonEmpty(results[0].SourceFingerprint, results[0].SourceDocumentID, results[0].Path, results[0].Filename, results[0].SourceTitle)
		trace, traceErr := rt.DeepDrillProvenanceTrace(ctx, DeepDrillProvenanceTraceRequest{Collection: collection, SourceID: anchor, Limit: 1200})
		if traceErr != nil {
			warnings = append(warnings, "provenance trace failed: "+traceErr.Error())
			return results, warnings, nil
		}
		warnings = append(warnings, fmt.Sprintf("provenance trace upstream=%d downstream=%d related=%d confidence=%.2f", len(trace.Upstream), len(trace.Downstream), len(trace.RelatedSources), trace.Confidence))
		if len(trace.Weaknesses) > 0 {
			warnings = append(warnings, "provenance weaknesses: "+strings.Join(trace.Weaknesses, "; "))
		}
		return results, warnings, nil
	case DeepDrillRawSourceLookup:
		results, runWarnings, err := rt.semanticSearch(ctx, collection, question, 12, map[string]string{"source_type": "document"}, nil, 0, false)
		warnings = append(warnings, runWarnings...)
		return results, warnings, err
	case DeepDrillCounterEvidenceScan:
		results, runWarnings, err := rt.keywordSearch(ctx, collection, question+" contradiction counterevidence however", 12, nil, nil)
		warnings = append(warnings, runWarnings...)
		return results, warnings, err
	case DeepDrillConfidenceDowngrade:
		warnings = append(warnings, "provenance-weighted confidence downgrade strategy executed")
		return nil, warnings, nil
	case DeepDrillSourceSequence:
		results, runWarnings, err := rt.keywordSearch(ctx, collection, question+" sequence ordering", 12, nil, nil)
		warnings = append(warnings, runWarnings...)
		return sortByCreation(results), warnings, err
	case DeepDrillSourceTypeProfile:
		results, runWarnings, err := rt.semanticSearch(ctx, collection, question, 12, nil, nil, 0, false)
		warnings = append(warnings, runWarnings...)
		warnings = append(warnings, provenanceProfileNote(results))
		return results, warnings, err
	case DeepDrillMarkerPartition:
		results, runWarnings, err := rt.keywordSearch(ctx, collection, "E8 sacred translator bridge", 25, nil, nil)
		warnings = append(warnings, runWarnings...)
		return results, warnings, err
	case DeepDrillVocabularyStrip:
		results, runWarnings, err := rt.keywordSearch(ctx, collection, stripGenericVocabulary(question), 12, nil, nil)
		warnings = append(warnings, runWarnings...)
		return results, warnings, err
	case DeepDrillCounterevidenceSearch:
		results, runWarnings, err := rt.keywordSearch(ctx, collection, question+" contradiction counterevidence however", 12, nil, nil)
		warnings = append(warnings, runWarnings...)
		return results, warnings, err
	case DeepDrillSourceComparison:
		results, runWarnings, err := rt.semanticSearch(ctx, collection, question, 12, nil, nil, 0, false)
		warnings = append(warnings, runWarnings...)
		return results, warnings, err
	case DeepDrillBroadScroll:
		report, err := rt.inspectCollection(ctx, collection, 600)
		if err != nil {
			return nil, warnings, err
		}
		warnings = append(warnings, fmt.Sprintf("broad scroll points=%d documents=%d missing_provenance=%d", report.ChunksOrVectors, report.DocumentsDetected, len(report.MissingProvenanceFields)))
		return nil, warnings, nil
	case DeepDrillCollectionInspection:
		report, err := rt.inspectCollection(ctx, collection, 600)
		if err != nil {
			return nil, warnings, err
		}
		warnings = append(warnings, fmt.Sprintf("collection inspection points=%d documents=%d missing_provenance=%d", report.ChunksOrVectors, report.DocumentsDetected, len(report.MissingProvenanceFields)))
		return nil, warnings, nil
	case DeepDrillFreezeBranch:
		warnings = append(warnings, "branch flagged for freeze due to diminishing returns")
		return nil, warnings, nil
	default:
		results, runWarnings, err := rt.semanticSearch(ctx, collection, question, 8, nil, nil, 0, false)
		warnings = append(warnings, runWarnings...)
		return results, warnings, err
	}
}

func applyModelUpdate(state *DeepDrillState, uncertainty DeepDrillUncertainty, evidence []ResearchResult, provenanceScore float64) {
	if len(state.Hypotheses) == 0 {
		return
	}
	now := time.Now().UTC()
	for idx := range state.Hypotheses {
		h := &state.Hypotheses[idx]
		h.PriorConfidence = h.Confidence
		h.LastUpdated = now
		if strings.TrimSpace(h.Status) == "" {
			h.Status = "active"
		}
	}

	supportByID, counterByID, ambiguousByID := classifyEvidenceForHypotheses(state.Hypotheses, evidence)

	for idx := range state.Hypotheses {
		h := &state.Hypotheses[idx]
		priorConfidence := h.Confidence

		newSupport := unseenEvidence(h.SupportingEvidenceIDs, supportByID[h.ID])
		newCounter := unseenEvidence(h.ContradictingEvidence, counterByID[h.ID])
		newAmbiguous := unseenEvidence(h.AmbiguousEvidenceIDs, ambiguousByID[h.ID])

		supportWeight := sumProvenanceWeight(newSupport)
		counterWeight := sumProvenanceWeight(newCounter)
		netWeight := supportWeight - counterWeight

		h.SupportWeight = supportWeight
		h.CounterevidenceWeight = counterWeight
		h.ProvenanceWeight = clamp01(provenanceScore)
		h.UniqueSupportCount = len(newSupport)
		h.UniqueCounterevidenceCount = len(newCounter)
		h.UniqueAmbiguousCount = len(newAmbiguous)

		h.Confidence = clamp01(h.Confidence + netWeight*deepDrillConfidenceScale)
		h.ConfidenceDelta = h.Confidence - priorConfidence
		h.EvidenceStrength = clamp01(h.EvidenceStrength + netWeight*deepDrillEvidenceStrengthScale)

		if len(newCounter) > 0 {
			h.Status = "contested"
		} else if h.Status == "contested" && len(h.ContradictingEvidence) == 0 {
			h.Status = "active"
		}

		h.SupportingEvidenceIDs = uniqueTrimmed(append(h.SupportingEvidenceIDs, evidenceIDs(newSupport)...))
		h.ContradictingEvidence = uniqueTrimmed(append(h.ContradictingEvidence, evidenceIDs(newCounter)...))
		h.AmbiguousEvidenceIDs = uniqueTrimmed(append(h.AmbiguousEvidenceIDs, evidenceIDs(newAmbiguous)...))

		h.ScoringReason = scoringReason(len(newSupport), len(newCounter), len(newAmbiguous), supportWeight, counterWeight)
	}

	rerankHypotheses(state.Hypotheses)
}

func scoringReason(supportCount, counterCount, ambiguousCount int, supportWeight, counterWeight float64) string {
	switch {
	case supportCount == 0 && counterCount == 0 && ambiguousCount == 0:
		return "no new discriminating evidence"
	case supportCount == 0 && counterCount == 0 && ambiguousCount > 0:
		return "ambiguous evidence, polarity unresolved"
	case supportCount > 0 && counterCount == 0:
		return fmt.Sprintf("new supporting evidence (weight %.2f)", supportWeight)
	case supportCount == 0 && counterCount > 0:
		return fmt.Sprintf("new counterevidence (weight %.2f)", counterWeight)
	default:
		return fmt.Sprintf("mixed evidence: support %.2f vs counter %.2f", supportWeight, counterWeight)
	}
}

func unseenEvidence(seenIDs []string, candidates []ResearchResult) []ResearchResult {
	if len(candidates) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(seenIDs))
	for _, id := range seenIDs {
		if id != "" {
			seen[id] = struct{}{}
		}
	}
	out := make([]ResearchResult, 0, len(candidates))
	for _, item := range candidates {
		id := evidenceIDForResult(item)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		out = append(out, item)
	}
	return out
}

func evidenceIDForResult(item ResearchResult) string {
	return firstNonEmpty(item.MemoryID, item.ChunkID, item.SourceFingerprint, item.SourceDocumentID, item.SourceTitle)
}

func evidenceIDs(items []ResearchResult) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if id := evidenceIDForResult(item); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func materialModelChange(before, after []DeepDrillHypothesisState) bool {
	if len(before) != len(after) {
		return true
	}
	beforeByID := make(map[string]DeepDrillHypothesisState, len(before))
	for _, hypothesis := range before {
		beforeByID[hypothesis.ID] = hypothesis
	}
	for _, current := range after {
		prior, ok := beforeByID[current.ID]
		if !ok {
			return true
		}
		if prior.Rank != current.Rank {
			return true
		}
		if prior.Status != current.Status {
			return true
		}
		if math.Abs(prior.Confidence-current.Confidence) >= deepDrillMaterialConfidenceDelta {
			return true
		}
		if !stringSlicesEqual(prior.SupportingEvidenceIDs, current.SupportingEvidenceIDs) {
			return true
		}
		if !stringSlicesEqual(prior.ContradictingEvidence, current.ContradictingEvidence) {
			return true
		}
	}
	return false
}

func sortByCreation(results []ResearchResult) []ResearchResult {
	cloned := append([]ResearchResult(nil), results...)
	sort.SliceStable(cloned, func(i, j int) bool {
		iT := parseAnyTime(firstNonEmpty(cloned[i].CreatedDate, cloned[i].ModifiedDate))
		jT := parseAnyTime(firstNonEmpty(cloned[j].CreatedDate, cloned[j].ModifiedDate))
		if iT.IsZero() && jT.IsZero() {
			return cloned[i].MemoryID < cloned[j].MemoryID
		}
		if iT.IsZero() {
			return false
		}
		if jT.IsZero() {
			return true
		}
		return iT.Before(jT)
	})
	return cloned
}

func hasContradictionSignal(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "contradict") || strings.Contains(lower, "however") || strings.Contains(lower, "inconsistent") || strings.Contains(lower, "does not")
}

func summarizeResearchResult(item ResearchResult) string {
	text := strings.TrimSpace(item.Text)
	if len(text) > 220 {
		text = text[:220]
	}
	return strings.TrimSpace(fmt.Sprintf("%s [%s] %s", firstNonEmpty(item.SourceTitle, item.SourceDocumentID, item.SourceFingerprint), item.SourceType, text))
}

func sampleResults(results []ResearchResult, limit int) []ResearchResult {
	if limit <= 0 || len(results) <= limit {
		return append([]ResearchResult(nil), results...)
	}
	return append([]ResearchResult(nil), results[:limit]...)
}

func defaultConfidence(value float64) float64 {
	if value <= 0 {
		return 0.42
	}
	return clamp01(value)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
