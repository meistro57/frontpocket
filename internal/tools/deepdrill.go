package tools

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/meistro57/frontpocket/internal/store"
)

const DeepDrillResearcherSystemPrompt = "You are DeepDrill, an autonomous research loop operating over FrontPocket and MindDrill. Semantic similarity is a lead, not evidence. Preserve competing models. Translation is not derivation. Mathematical formalism does not validate attached physical claims. Negative results matter. Do not treat ingestion chronology as intellectual chronology. Do not treat prior DeepDrill thoughts as independent evidence. Prefer questions that discriminate between models. Reduce confidence when provenance is weak. Return structured research artifacts only."

type DeepDrillUncertainty string

const (
	DeepDrillEvidenceGap             DeepDrillUncertainty = "EVIDENCE_GAP"
	DeepDrillProvenanceGap           DeepDrillUncertainty = "PROVENANCE_GAP"
	DeepDrillChronologyGap           DeepDrillUncertainty = "CHRONOLOGY_GAP"
	DeepDrillContradiction           DeepDrillUncertainty = "CONTRADICTION"
	DeepDrillAmbiguousClassification DeepDrillUncertainty = "AMBIGUOUS_CLASSIFICATION"
	DeepDrillGenericSimilarity       DeepDrillUncertainty = "GENERIC_SIMILARITY"
	DeepDrillRetrievalLimit          DeepDrillUncertainty = "RETRIEVAL_LIMIT"
	DeepDrillSourceQuality           DeepDrillUncertainty = "SOURCE_QUALITY"
	DeepDrillDiminishingReturns      DeepDrillUncertainty = "DIMINISHING_RETURNS"
)

type DeepDrillStrategy string

const (
	DeepDrillSemanticBroad       DeepDrillStrategy = "SEMANTIC_BROAD"
	DeepDrillKeywordTargeted     DeepDrillStrategy = "KEYWORD_TARGETED"
	DeepDrillDocumentExpansion   DeepDrillStrategy = "DOCUMENT_EXPANSION"
	DeepDrillCrossDocument       DeepDrillStrategy = "CROSS_DOCUMENT"
	DeepDrillInspectCollection   DeepDrillStrategy = "INSPECT_COLLECTION"
	DeepDrillNeighborChunks      DeepDrillStrategy = "NEIGHBOR_CHUNKS"
	DeepDrillPartitionTest       DeepDrillStrategy = "PARTITION_TEST"
	DeepDrillNegativeControl     DeepDrillStrategy = "NEGATIVE_CONTROL"
	DeepDrillChronologyTrace     DeepDrillStrategy = "CHRONOLOGY_TRACE"
	DeepDrillTimestampTrace      DeepDrillStrategy = "TIMESTAMP_TRACE"
	DeepDrillProvenanceProfile   DeepDrillStrategy = "PROVENANCE_PROFILE"
	DeepDrillProvenanceTrace     DeepDrillStrategy = "PROVENANCE_TRACE"
	DeepDrillRawSourceLookup     DeepDrillStrategy = "RAW_SOURCE_LOOKUP"
	DeepDrillCounterEvidenceScan DeepDrillStrategy = "COUNTEREVIDENCE_SCAN"
	DeepDrillConfidenceDowngrade DeepDrillStrategy = "PROVENANCE_DOWNWEIGHT"
	DeepDrillFreezeBranch        DeepDrillStrategy = "FREEZE_BRANCH"
)

type DeepDrillThoughtType string

const (
	DeepDrillThoughtQuestion         DeepDrillThoughtType = "QUESTION"
	DeepDrillThoughtHypothesisState  DeepDrillThoughtType = "HYPOTHESIS_STATE"
	DeepDrillThoughtUncertainty      DeepDrillThoughtType = "UNCERTAINTY"
	DeepDrillThoughtStrategyRun      DeepDrillThoughtType = "STRATEGY_RUN"
	DeepDrillThoughtEvidence         DeepDrillThoughtType = "EVIDENCE"
	DeepDrillThoughtCounterEvidence  DeepDrillThoughtType = "COUNTEREVIDENCE"
	DeepDrillThoughtNegativeResult   DeepDrillThoughtType = "NEGATIVE_RESULT"
	DeepDrillThoughtContradiction    DeepDrillThoughtType = "CONTRADICTION"
	DeepDrillThoughtModelRevision    DeepDrillThoughtType = "MODEL_REVISION"
	DeepDrillThoughtConfidenceUpdate DeepDrillThoughtType = "CONFIDENCE_UPDATE"
	DeepDrillThoughtSynthesis        DeepDrillThoughtType = "SYNTHESIS"
	DeepDrillThoughtStopDecision     DeepDrillThoughtType = "STOP_DECISION"
	DeepDrillThoughtReopenDecision   DeepDrillThoughtType = "REOPEN_DECISION"
	DeepDrillThoughtAnomaly          DeepDrillThoughtType = "ANOMALY"
	DeepDrillThoughtLineageEdge      DeepDrillThoughtType = "LINEAGE_EDGE"
)

type DeepDrillInfoGain string

const (
	DeepDrillInfoLow    DeepDrillInfoGain = "LOW"
	DeepDrillInfoMedium DeepDrillInfoGain = "MEDIUM"
	DeepDrillInfoHigh   DeepDrillInfoGain = "HIGH"
)

type DeepDrillBranchStatus string

const (
	DeepDrillOpen   DeepDrillBranchStatus = "OPEN"
	DeepDrillFrozen DeepDrillBranchStatus = "FROZEN"
)

type DeepDrillEvidenceOrigin string

const (
	DeepDrillSourceCorpus    DeepDrillEvidenceOrigin = "SOURCE_CORPUS"
	DeepDrillResearchThought DeepDrillEvidenceOrigin = "RESEARCH_THOUGHTS"
)

type DeepDrillSourceTier string

const (
	DeepDrillRawSource         DeepDrillSourceTier = "RAW_SOURCE"
	DeepDrillRawTranscript     DeepDrillSourceTier = "RAW_TRANSCRIPT"
	DeepDrillVersionedDraft    DeepDrillSourceTier = "VERSIONED_DRAFT"
	DeepDrillDirectNote        DeepDrillSourceTier = "DIRECT_NOTE"
	DeepDrillDerivedSummary    DeepDrillSourceTier = "DERIVED_SUMMARY"
	DeepDrillAudioOverview     DeepDrillSourceTier = "AUDIO_OVERVIEW"
	DeepDrillPresentationSumm  DeepDrillSourceTier = "PRESENTATION_SUMMARY"
	DeepDrillAISynthesis       DeepDrillSourceTier = "AI_SYNTHESIS"
	DeepDrillUnknownSourceTier DeepDrillSourceTier = "UNKNOWN"
)

type DeepDrillHypothesisState struct {
	ID                    string    `json:"id"`
	Statement             string    `json:"statement"`
	Rank                  int       `json:"rank"`
	Confidence            float64   `json:"confidence"`
	EvidenceStrength      float64   `json:"evidence_strength"`
	Status                string    `json:"status"`
	SupportingEvidenceIDs []string  `json:"supporting_evidence_ids,omitempty"`
	ContradictingEvidence []string  `json:"contradicting_evidence_ids,omitempty"`
	LastUpdated           time.Time `json:"last_updated"`
}

type DeepDrillStrategyOutcome struct {
	Strategy            DeepDrillStrategy    `json:"strategy"`
	Uncertainty         DeepDrillUncertainty `json:"uncertainty"`
	Question            string               `json:"question"`
	InfoGain            DeepDrillInfoGain    `json:"info_gain"`
	DuplicateEvidence   bool                 `json:"duplicate_evidence"`
	EvidenceCount       int                  `json:"evidence_count"`
	EvidenceFingerprint string               `json:"evidence_fingerprint,omitempty"`
	Timestamp           time.Time            `json:"timestamp"`
}

type DeepDrillState struct {
	ThoughtCollection      string                     `json:"thought_collection"`
	BranchStatus           DeepDrillBranchStatus      `json:"branch_status"`
	FrozenReason           string                     `json:"frozen_reason,omitempty"`
	CurrentUncertainty     DeepDrillUncertainty       `json:"current_uncertainty,omitempty"`
	CurrentQuestion        string                     `json:"current_question,omitempty"`
	HighestValueQuestion   string                     `json:"highest_value_question,omitempty"`
	KnownBlockers          []DeepDrillUncertainty     `json:"known_blockers,omitempty"`
	Hypotheses             []DeepDrillHypothesisState `json:"hypotheses,omitempty"`
	ConsecutiveLowGain     int                        `json:"consecutive_low_gain"`
	Iterations             int                        `json:"iterations"`
	StrategyHistory        []DeepDrillStrategyOutcome `json:"strategy_history,omitempty"`
	LastSeenProvenanceTier DeepDrillSourceTier        `json:"last_seen_provenance_tier,omitempty"`
	LastUpdated            time.Time                  `json:"last_updated"`
}

type DeepDrillThought struct {
	ThoughtID          string                  `json:"thought_id"`
	SessionID          string                  `json:"session_id"`
	Timestamp          time.Time               `json:"timestamp"`
	Type               DeepDrillThoughtType    `json:"type"`
	Question           string                  `json:"question,omitempty"`
	HypothesisTargets  []string                `json:"hypothesis_targets,omitempty"`
	UncertaintyClass   DeepDrillUncertainty    `json:"uncertainty_class,omitempty"`
	Strategy           DeepDrillStrategy       `json:"strategy,omitempty"`
	QuerySpec          map[string]any          `json:"query_spec,omitempty"`
	Sources            []string                `json:"sources,omitempty"`
	EvidenceSummary    string                  `json:"evidence_summary,omitempty"`
	ProvenanceScore    float64                 `json:"provenance_score,omitempty"`
	EvidenceStrength   float64                 `json:"evidence_strength,omitempty"`
	Confidence         float64                 `json:"confidence,omitempty"`
	ContradictionFlag  bool                    `json:"contradiction_flag,omitempty"`
	ModelBefore        map[string]any          `json:"model_before,omitempty"`
	ModelAfter         map[string]any          `json:"model_after,omitempty"`
	InfoGain           DeepDrillInfoGain       `json:"info_gain,omitempty"`
	NextAction         string                  `json:"next_action,omitempty"`
	Status             string                  `json:"status,omitempty"`
	ParentThoughtIDs   []string                `json:"parent_thought_ids,omitempty"`
	Supersedes         []string                `json:"supersedes,omitempty"`
	Fingerprint        string                  `json:"fingerprint"`
	EvidenceOrigin     DeepDrillEvidenceOrigin `json:"evidence_origin"`
	DuplicateOfThought string                  `json:"duplicate_of_thought,omitempty"`
}

type DeepDrillPlanRequest struct {
	SessionID            string
	Collection           string
	ResearchQuestion     string
	Hypotheses           []DeepDrillHypothesisState
	KnownBlockers        []DeepDrillUncertainty
	HighestValueQuestion string
	ForceReopen          bool
}

type DeepDrillPlan struct {
	SessionID          string                `json:"session_id"`
	Collection         string                `json:"collection"`
	BranchStatus       DeepDrillBranchStatus `json:"branch_status"`
	TopUncertainty     DeepDrillUncertainty  `json:"top_uncertainty"`
	CandidateQuestions []string              `json:"candidate_questions"`
	SelectedQuestion   string                `json:"selected_question"`
	SelectedStrategy   DeepDrillStrategy     `json:"selected_strategy"`
	StrategyReason     string                `json:"strategy_reason"`
	ReuseStrategy      bool                  `json:"reuse_strategy"`
	SystemPrompt       string                `json:"system_prompt"`
	State              DeepDrillState        `json:"state"`
}

type DeepDrillRunRequest struct {
	DeepDrillPlanRequest
	Steps       int
	UntilStable bool
}

type DeepDrillStepResult struct {
	Step               int                   `json:"step"`
	Uncertainty        DeepDrillUncertainty  `json:"uncertainty"`
	Question           string                `json:"question"`
	Strategy           DeepDrillStrategy     `json:"strategy"`
	EvidenceCount      int                   `json:"evidence_count"`
	ThoughtCount       int                   `json:"thought_count"`
	InfoGain           DeepDrillInfoGain     `json:"info_gain"`
	BranchStatus       DeepDrillBranchStatus `json:"branch_status"`
	DuplicateEvidence  bool                  `json:"duplicate_evidence"`
	NegativeResult     bool                  `json:"negative_result"`
	SelectedSourceTier DeepDrillSourceTier   `json:"selected_source_tier"`
}

type DeepDrillRunResult struct {
	SessionID string                `json:"session_id"`
	Steps     []DeepDrillStepResult `json:"steps"`
	State     DeepDrillState        `json:"state"`
}

type DeepDrillThoughtQuery struct {
	SessionID   string
	Status      string
	Type        DeepDrillThoughtType
	Uncertainty DeepDrillUncertainty
	Hypothesis  string
	InfoGain    DeepDrillInfoGain
	Limit       int
}

type DeepDrillThoughtList struct {
	Collection string                `json:"collection"`
	Query      DeepDrillThoughtQuery `json:"query"`
	Thoughts   []DeepDrillThought    `json:"thoughts"`
}

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

type DeepDrillProvenanceRelation string

const (
	DeepDrillDerivedFrom      DeepDrillProvenanceRelation = "DERIVED_FROM"
	DeepDrillSameSourceFamily DeepDrillProvenanceRelation = "SAME_SOURCE_FAMILY"
	DeepDrillEarlierInSource  DeepDrillProvenanceRelation = "EARLIER_IN_SOURCE"
	DeepDrillLaterInSource    DeepDrillProvenanceRelation = "LATER_IN_SOURCE"
	DeepDrillCites            DeepDrillProvenanceRelation = "CITES"
	DeepDrillSummarizes       DeepDrillProvenanceRelation = "SUMMARIZES"
	DeepDrillTranscribes      DeepDrillProvenanceRelation = "TRANSCRIBES"
	DeepDrillPresents         DeepDrillProvenanceRelation = "PRESENTS"
	DeepDrillUnknownRelation  DeepDrillProvenanceRelation = "UNKNOWN_RELATION"
)

type DeepDrillProvenanceEdge struct {
	FromSourceID string                      `json:"from_source_id"`
	ToSourceID   string                      `json:"to_source_id"`
	Relation     DeepDrillProvenanceRelation `json:"relation"`
	Evidence     string                      `json:"evidence"`
	Confidence   float64                     `json:"confidence"`
	Basis        string                      `json:"basis"`
	Explicit     bool                        `json:"explicit"`
}

type DeepDrillProvenanceTraceRequest struct {
	SessionID  string
	Collection string
	ThoughtID  string
	SourceID   string
	Limit      int
}

type DeepDrillProvenanceTraceResult struct {
	Source              string                    `json:"source"`
	Type                string                    `json:"type"`
	Title               string                    `json:"title,omitempty"`
	Collection          string                    `json:"collection"`
	AnchorThoughtID     string                    `json:"anchor_thought_id,omitempty"`
	AnchorThoughtStatus string                    `json:"anchor_thought_status,omitempty"`
	Upstream            []DeepDrillProvenanceEdge `json:"upstream,omitempty"`
	Downstream          []DeepDrillProvenanceEdge `json:"downstream,omitempty"`
	RelatedSources      []DeepDrillProvenanceEdge `json:"related_sources,omitempty"`
	ChronologySignals   []string                  `json:"chronology_signals,omitempty"`
	Weaknesses          []string                  `json:"weaknesses,omitempty"`
	Confidence          float64                   `json:"confidence"`
}

func (rt *MindDrillResearchRuntime) ConfigureDeepDrill(thoughtCollection string, freezeAfterLowGain int) {
	if rt == nil {
		return
	}
	if strings.TrimSpace(thoughtCollection) != "" {
		rt.deepDrillThoughts = strings.TrimSpace(thoughtCollection)
	}
	if freezeAfterLowGain > 0 {
		rt.deepDrillFreezeAfterLow = freezeAfterLowGain
	}
}

func (rt *MindDrillResearchRuntime) DeepDrillPlan(ctx context.Context, req DeepDrillPlanRequest) (DeepDrillPlan, error) {
	session, collection, state, err := rt.prepareDeepDrillSession(ctx, req)
	if err != nil {
		return DeepDrillPlan{}, err
	}
	uncertainty := identifyHighestValueUncertainty(session, state)
	questions := generateCompetingQuestions(session, state, uncertainty)
	selectedQuestion := selectMostDiscriminatingQuestion(questions, state)
	strategy, reason, reused := selectStrategy(state, uncertainty)
	state.CurrentUncertainty = uncertainty
	state.CurrentQuestion = selectedQuestion
	state.LastUpdated = time.Now().UTC()
	session.DeepDrill = state
	rt.sessions.put(session)
	return DeepDrillPlan{
		SessionID:          session.SessionID,
		Collection:         collection,
		BranchStatus:       state.BranchStatus,
		TopUncertainty:     uncertainty,
		CandidateQuestions: questions,
		SelectedQuestion:   selectedQuestion,
		SelectedStrategy:   strategy,
		StrategyReason:     reason,
		ReuseStrategy:      reused,
		SystemPrompt:       DeepDrillResearcherSystemPrompt,
		State:              *state,
	}, nil
}

func (rt *MindDrillResearchRuntime) DeepDrillRun(ctx context.Context, req DeepDrillRunRequest) (DeepDrillRunResult, error) {
	if req.Steps <= 0 {
		req.Steps = 1
	}
	if req.Steps > 50 {
		req.Steps = 50
	}
	result := DeepDrillRunResult{SessionID: strings.TrimSpace(req.DeepDrillPlanRequest.SessionID), Steps: []DeepDrillStepResult{}}
	maxIterations := req.Steps
	if req.UntilStable {
		maxIterations = 25
	}
	for i := 0; i < maxIterations; i++ {
		plan, err := rt.DeepDrillPlan(ctx, req.DeepDrillPlanRequest)
		if err != nil {
			return result, err
		}
		step, state, err := rt.executeDeepDrillStep(ctx, plan)
		if err != nil {
			return result, err
		}
		result.Steps = append(result.Steps, step)
		result.State = *state
		if !req.UntilStable && len(result.Steps) >= req.Steps {
			break
		}
		if req.UntilStable && state.BranchStatus == DeepDrillFrozen {
			break
		}
	}
	return result, nil
}

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
			stopThought := DeepDrillThought{
				Type:             DeepDrillThoughtStopDecision,
				Question:         plan.SelectedQuestion,
				UncertaintyClass: DeepDrillDiminishingReturns,
				Strategy:         DeepDrillFreezeBranch,
				EvidenceSummary:  "Branch remains frozen due to diminishing returns under current corpus and strategy set.",
				InfoGain:         DeepDrillInfoLow,
				Status:           "frozen",
				NextAction:       "wait for new provenance tier or contradictory evidence",
				EvidenceOrigin:   DeepDrillResearchThought,
			}
			if _, _, err := rt.storeDeepDrillThought(ctx, session, state, stopThought); err != nil {
				return DeepDrillStepResult{}, nil, err
			}
			state.LastUpdated = time.Now().UTC()
			session.DeepDrill = state
			rt.sessions.put(session)
			return DeepDrillStepResult{Step: state.Iterations + 1, Uncertainty: DeepDrillDiminishingReturns, Question: plan.SelectedQuestion, Strategy: DeepDrillFreezeBranch, EvidenceCount: 0, ThoughtCount: 1, InfoGain: DeepDrillInfoLow, BranchStatus: state.BranchStatus, NegativeResult: true, SelectedSourceTier: state.LastSeenProvenanceTier}, state, nil
		}
		state.BranchStatus = DeepDrillOpen
		state.FrozenReason = ""
		reopenThought := DeepDrillThought{Type: DeepDrillThoughtReopenDecision, Question: plan.SelectedQuestion, EvidenceSummary: "Frozen branch reopened because a novel question or collection context became available.", EvidenceOrigin: DeepDrillResearchThought, Status: "open"}
		if _, _, err := rt.storeDeepDrillThought(ctx, session, state, reopenThought); err != nil {
			return DeepDrillStepResult{}, nil, err
		}
	}

	modelBefore := exportHypothesisModel(state.Hypotheses)
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
		applyModelUpdate(state, plan.TopUncertainty, evidence, provenanceScore, hasContradiction)
	}
	modelAfter := exportHypothesisModel(state.Hypotheses)
	infoGain := scoreInformationGain(modelBefore, modelAfter, state, len(evidence), duplicateEvidence, topTier)
	if infoGain == DeepDrillInfoLow {
		state.ConsecutiveLowGain++
	} else {
		state.ConsecutiveLowGain = 0
	}
	if state.ConsecutiveLowGain >= rt.deepDrillFreezeAfterLow {
		state.BranchStatus = DeepDrillFrozen
		state.FrozenReason = fmt.Sprintf("%d consecutive low-gain runs", state.ConsecutiveLowGain)
	}
	state.Iterations++
	state.LastUpdated = time.Now().UTC()
	state.StrategyHistory = append(state.StrategyHistory, DeepDrillStrategyOutcome{Strategy: plan.SelectedStrategy, Uncertainty: plan.TopUncertainty, Question: plan.SelectedQuestion, InfoGain: infoGain, DuplicateEvidence: duplicateEvidence, EvidenceCount: len(evidence), EvidenceFingerprint: evidenceFingerprint, Timestamp: time.Now().UTC()})

	revisionThought := DeepDrillThought{
		Type:             DeepDrillThoughtModelRevision,
		Question:         plan.SelectedQuestion,
		UncertaintyClass: plan.TopUncertainty,
		Strategy:         plan.SelectedStrategy,
		EvidenceSummary:  "DeepDrill updated competing hypothesis state and confidence after evaluating this run.",
		ModelBefore:      modelBefore,
		ModelAfter:       modelAfter,
		InfoGain:         infoGain,
		ProvenanceScore:  provenanceScore,
		EvidenceOrigin:   DeepDrillResearchThought,
		Status:           strings.ToLower(string(state.BranchStatus)),
		NextAction:       nextActionFromState(state),
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

func newDefaultDeepDrillState(thoughtCollection string) *DeepDrillState {
	return &DeepDrillState{
		ThoughtCollection: strings.TrimSpace(thoughtCollection),
		BranchStatus:      DeepDrillOpen,
		KnownBlockers:     []DeepDrillUncertainty{},
		Hypotheses:        []DeepDrillHypothesisState{},
		LastUpdated:       time.Now().UTC(),
	}
}

func identifyHighestValueUncertainty(session ResearchSession, state *DeepDrillState) DeepDrillUncertainty {
	if state == nil {
		return DeepDrillEvidenceGap
	}
	if state.BranchStatus == DeepDrillFrozen {
		return DeepDrillDiminishingReturns
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
	if state == nil || len(state.StrategyHistory) == 0 {
		return candidates[0], "no prior runs, using primary strategy for uncertainty", false
	}
	usage := make(map[DeepDrillStrategy]int)
	success := make(map[DeepDrillStrategy]int)
	for _, entry := range state.StrategyHistory {
		usage[entry.Strategy]++
		if entry.InfoGain == DeepDrillInfoMedium || entry.InfoGain == DeepDrillInfoHigh {
			success[entry.Strategy]++
		}
	}
	best := candidates[0]
	bestScore := -1
	reused := false
	for _, strategy := range candidates {
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
		return []DeepDrillStrategy{DeepDrillInspectCollection, DeepDrillCrossDocument, DeepDrillNeighborChunks}
	case DeepDrillAmbiguousClassification:
		return []DeepDrillStrategy{DeepDrillPartitionTest, DeepDrillNegativeControl, DeepDrillCrossDocument}
	case DeepDrillGenericSimilarity:
		return []DeepDrillStrategy{DeepDrillNegativeControl, DeepDrillKeywordTargeted, DeepDrillCrossDocument}
	case DeepDrillChronologyGap:
		return []DeepDrillStrategy{DeepDrillProvenanceTrace, DeepDrillChronologyTrace, DeepDrillTimestampTrace, DeepDrillDocumentExpansion}
	case DeepDrillProvenanceGap:
		return []DeepDrillStrategy{DeepDrillProvenanceTrace, DeepDrillProvenanceProfile, DeepDrillRawSourceLookup, DeepDrillConfidenceDowngrade}
	case DeepDrillContradiction:
		return []DeepDrillStrategy{DeepDrillCounterEvidenceScan, DeepDrillCrossDocument, DeepDrillKeywordTargeted}
	case DeepDrillSourceQuality:
		return []DeepDrillStrategy{DeepDrillProvenanceTrace, DeepDrillProvenanceProfile, DeepDrillConfidenceDowngrade}
	case DeepDrillDiminishingReturns:
		return []DeepDrillStrategy{DeepDrillFreezeBranch}
	default:
		return []DeepDrillStrategy{DeepDrillSemanticBroad, DeepDrillKeywordTargeted}
	}
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
	case DeepDrillFreezeBranch:
		warnings = append(warnings, "branch flagged for freeze due to diminishing returns")
		return nil, warnings, nil
	default:
		results, runWarnings, err := rt.semanticSearch(ctx, collection, question, 8, nil, nil, 0, false)
		warnings = append(warnings, runWarnings...)
		return results, warnings, err
	}
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

func applyModelUpdate(state *DeepDrillState, uncertainty DeepDrillUncertainty, evidence []ResearchResult, provenanceScore float64, hasContradiction bool) {
	now := time.Now().UTC()
	if len(state.Hypotheses) == 0 {
		return
	}
	for idx := range state.Hypotheses {
		state.Hypotheses[idx].LastUpdated = now
		if strings.TrimSpace(state.Hypotheses[idx].Status) == "" {
			state.Hypotheses[idx].Status = "active"
		}
	}
	if uncertainty == DeepDrillProvenanceGap || uncertainty == DeepDrillSourceQuality {
		delta := 0.08
		if provenanceScore > 0.7 {
			delta = -0.03
		}
		for idx := range state.Hypotheses {
			state.Hypotheses[idx].Confidence = clamp01(state.Hypotheses[idx].Confidence - delta)
		}
		return
	}
	if len(evidence) == 0 {
		for idx := range state.Hypotheses {
			state.Hypotheses[idx].EvidenceStrength = clamp01(state.Hypotheses[idx].EvidenceStrength - 0.02)
		}
		return
	}
	for idx := range state.Hypotheses {
		state.Hypotheses[idx].EvidenceStrength = clamp01(state.Hypotheses[idx].EvidenceStrength + 0.03)
	}
	if hasContradiction {
		for idx := range state.Hypotheses {
			state.Hypotheses[idx].Confidence = clamp01(state.Hypotheses[idx].Confidence - 0.04)
			state.Hypotheses[idx].Status = "contested"
		}
	}
	for _, item := range sampleResults(evidence, 5) {
		evidenceID := firstNonEmpty(item.MemoryID, item.ChunkID, item.SourceFingerprint)
		if evidenceID == "" {
			continue
		}
		state.Hypotheses[0].SupportingEvidenceIDs = uniqueTrimmed(append(state.Hypotheses[0].SupportingEvidenceIDs, evidenceID))
		if hasContradiction && len(state.Hypotheses) > 1 {
			state.Hypotheses[1].ContradictingEvidence = uniqueTrimmed(append(state.Hypotheses[1].ContradictingEvidence, evidenceID))
		}
	}
}

func scoreInformationGain(modelBefore, modelAfter map[string]any, state *DeepDrillState, evidenceCount int, duplicate bool, tier DeepDrillSourceTier) DeepDrillInfoGain {
	if state != nil && state.BranchStatus == DeepDrillFrozen {
		return DeepDrillInfoHigh
	}
	if duplicate {
		return DeepDrillInfoLow
	}
	if tier == DeepDrillRawSource || tier == DeepDrillVersionedDraft || tier == DeepDrillRawTranscript {
		if !mapsEqual(modelBefore, modelAfter) {
			return DeepDrillInfoHigh
		}
		return DeepDrillInfoMedium
	}
	if !mapsEqual(modelBefore, modelAfter) {
		return DeepDrillInfoHigh
	}
	if evidenceCount > 0 {
		return DeepDrillInfoMedium
	}
	return DeepDrillInfoLow
}

func mapsEqual(a, b map[string]any) bool {
	left, errA := json.Marshal(a)
	right, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(left) == string(right)
}

func exportHypothesisModel(hypotheses []DeepDrillHypothesisState) map[string]any {
	rows := make([]map[string]any, 0, len(hypotheses))
	for _, hypothesis := range hypotheses {
		rows = append(rows, map[string]any{
			"id":                hypothesis.ID,
			"rank":              hypothesis.Rank,
			"confidence":        hypothesis.Confidence,
			"evidence_strength": hypothesis.EvidenceStrength,
			"status":            hypothesis.Status,
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

type deepDrillProvenanceNode struct {
	SourceID           string
	DocumentID         string
	ConversationID     string
	SourceType         string
	Filename           string
	SourceTitle        string
	FamilyID           string
	MemoryID           string
	ChunkID            string
	ChunkIndex         int
	TurnIndex          int
	SegmentStart       float64
	SegmentEnd         float64
	OriginalTimestamp  time.Time
	IngestionTimestamp time.Time
	DerivedFrom        []string
	CitedSources       []string
	ParentSource       string
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

func asStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(asString(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		if strings.Contains(trimmed, ",") {
			parts := strings.Split(trimmed, ",")
			return uniqueTrimmed(parts)
		}
		return []string{trimmed}
	default:
		return nil
	}
}

func asMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, entry := range typed {
			out[key] = entry
		}
		return out
	default:
		return nil
	}
}

func asFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0
		}
		return parsed
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func asBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func ParseDeepDrillThoughtType(value string) DeepDrillThoughtType {
	return DeepDrillThoughtType(strings.ToUpper(strings.TrimSpace(value)))
}

func ParseDeepDrillInfoGain(value string) DeepDrillInfoGain {
	return DeepDrillInfoGain(strings.ToUpper(strings.TrimSpace(value)))
}

func NormalizeDeepDrillArtifactStatus(thought DeepDrillThought) string {
	return normalizeArtifactStatus(thought)
}

func ParseDeepDrillUncertaintiesCSV(value string) []DeepDrillUncertainty {
	parts := strings.Split(value, ",")
	rows := make([]DeepDrillUncertainty, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(strings.ToUpper(part))
		if trimmed == "" {
			continue
		}
		rows = append(rows, DeepDrillUncertainty(trimmed))
	}
	return normalizeUncertainties(rows)
}

func ParseDeepDrillHypothesesCSV(value string) []DeepDrillHypothesisState {
	parts := strings.Split(value, ";")
	rows := make([]DeepDrillHypothesisState, 0, len(parts))
	for idx, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		id := ""
		statement := trimmed
		if split := strings.SplitN(trimmed, "|", 2); len(split) == 2 {
			id = strings.TrimSpace(split[0])
			statement = strings.TrimSpace(split[1])
		}
		if id == "" {
			id = fmt.Sprintf("H%d", idx+1)
		}
		rows = append(rows, DeepDrillHypothesisState{ID: id, Statement: statement, Rank: idx + 1, Confidence: 0.42, EvidenceStrength: 0.2, Status: "active"})
	}
	return normalizeHypothesisSeeds(rows)
}

func ParseDeepDrillFreezeAfter(value string, fallback int) int {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}
