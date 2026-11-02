package tools

import (
	"time"
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

	DeepDrillSourceSequence        DeepDrillStrategy = "SOURCE_SEQUENCE"
	DeepDrillSourceTypeProfile     DeepDrillStrategy = "SOURCE_TYPE_PROFILE"
	DeepDrillMarkerPartition       DeepDrillStrategy = "MARKER_PARTITION"
	DeepDrillVocabularyStrip       DeepDrillStrategy = "VOCABULARY_STRIP"
	DeepDrillCounterevidenceSearch DeepDrillStrategy = "COUNTEREVIDENCE_SEARCH"
	DeepDrillSourceComparison      DeepDrillStrategy = "SOURCE_COMPARISON"
	DeepDrillBroadScroll           DeepDrillStrategy = "BROAD_SCROLL"
	DeepDrillCollectionInspection  DeepDrillStrategy = "COLLECTION_INSPECTION"
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
	DeepDrillThoughtReclassify       DeepDrillThoughtType = "RECLASSIFY_UNCERTAINTY"
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

const (
	// deepDrillRelevanceThreshold is the minimum fraction of a hypothesis's
	// meaningful terms that must appear in an evidence chunk for that chunk
	// to be associated with the hypothesis.
	deepDrillRelevanceThreshold = 0.5

	// deepDrillConfidenceScale converts net evidence weight (support minus
	// counterevidence, provenance-scaled) into a confidence delta.
	deepDrillConfidenceScale = 0.06

	// deepDrillEvidenceStrengthScale converts net evidence weight into an
	// evidence_strength delta for the same hypothesis.
	deepDrillEvidenceStrengthScale = 0.08

	// deepDrillMaterialConfidenceDelta is the minimum confidence change that
	// counts as a material model change for information gain.
	deepDrillMaterialConfidenceDelta = 0.01
)

type DeepDrillHypothesisState struct {
	ID                         string    `json:"id"`
	Statement                  string    `json:"statement"`
	Rank                       int       `json:"rank"`
	Confidence                 float64   `json:"confidence"`
	PriorConfidence            float64   `json:"prior_confidence,omitempty"`
	ConfidenceDelta            float64   `json:"confidence_delta,omitempty"`
	EvidenceStrength           float64   `json:"evidence_strength"`
	SupportWeight              float64   `json:"support_weight,omitempty"`
	CounterevidenceWeight      float64   `json:"counterevidence_weight,omitempty"`
	ProvenanceWeight           float64   `json:"provenance_weight,omitempty"`
	UniqueSupportCount         int       `json:"unique_support_count,omitempty"`
	UniqueCounterevidenceCount int       `json:"unique_counterevidence_count,omitempty"`
	RankDelta                  int       `json:"rank_delta,omitempty"`
	ScoringReason              string    `json:"scoring_reason,omitempty"`
	Status                     string    `json:"status"`
	SupportingEvidenceIDs      []string  `json:"supporting_evidence_ids,omitempty"`
	ContradictingEvidence      []string  `json:"contradicting_evidence_ids,omitempty"`
	AmbiguousEvidenceIDs       []string  `json:"ambiguous_evidence_ids,omitempty"`
	UniqueAmbiguousCount       int       `json:"unique_ambiguous_count,omitempty"`
	LastUpdated                time.Time `json:"last_updated"`
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

type DeepDrillStrategyExhaustion struct {
	Uncertainty         DeepDrillUncertainty `json:"uncertainty"`
	Strategy            DeepDrillStrategy    `json:"strategy"`
	ModelKey            string               `json:"model_key"`
	ProvenanceTier      DeepDrillSourceTier  `json:"provenance_tier,omitempty"`
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

	PreviousUncertainty     DeepDrillUncertainty          `json:"previous_uncertainty,omitempty"`
	ReclassifiedUncertainty DeepDrillUncertainty          `json:"reclassified_uncertainty,omitempty"`
	ReclassificationReason  string                        `json:"reclassification_reason,omitempty"`
	StrategiesRemaining     []DeepDrillStrategy           `json:"strategies_remaining,omitempty"`
	StrategiesExhausted     []DeepDrillStrategy           `json:"strategies_exhausted,omitempty"`
	StrategyExhaustion      []DeepDrillStrategyExhaustion `json:"strategy_exhaustion,omitempty"`

	LastUpdated time.Time `json:"last_updated"`
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

	PreviousUncertainty     DeepDrillUncertainty `json:"previous_uncertainty,omitempty"`
	ReclassifiedUncertainty DeepDrillUncertainty `json:"reclassified_uncertainty,omitempty"`
	ReclassificationReason  string               `json:"reclassification_reason,omitempty"`
	StrategiesRemaining     []DeepDrillStrategy  `json:"strategies_remaining,omitempty"`
	StrategiesExhausted     []DeepDrillStrategy  `json:"strategies_exhausted,omitempty"`
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
	Terminal           bool                  `json:"terminal,omitempty"`
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

type deepDrillPolarity string

const (
	deepDrillPolaritySupport   deepDrillPolarity = "support"
	deepDrillPolarityCounter   deepDrillPolarity = "counter"
	deepDrillPolarityAmbiguous deepDrillPolarity = "ambiguous"
	deepDrillPolarityNone      deepDrillPolarity = "none"
)

// deepDrillEvidenceClassification is the per-(evidence, hypothesis) decision
// record. It carries every intermediate score so diagnostics can show exactly
// why an item was or was not associated with a hypothesis.
type deepDrillEvidenceClassification struct {
	EvidenceID         string
	UsableTextFields   []string
	HypothesisConcepts []string
	EvidenceConcepts   []string
	LexicalOverlap     float64
	ConceptMatches     []string
	RelationMatches    []string
	NegationSignals    []string
	HedgeSignals       []string
	RelevanceScore     float64
	Relevant           bool
	Polarity           deepDrillPolarity
	Reason             string
}

// deepDrillEvidenceEnvelope assembles every usable text field from a result so
// classification operates over the richest available body while preserving the
// field boundaries for diagnostics.
type deepDrillEvidenceEnvelope struct {
	fields []string
	body   string
}

// deepDrillConceptCanonical normalizes surface tokens to canonical concepts so
// morphological and relational variants align ("developed"/"development" ->
// "develop", "joined"/"recombined" -> "recombine"). Unmapped tokens pass
// through unchanged, keeping this an explicit, auditable vocabulary rather
// than a hidden stemmer.
var deepDrillConceptCanonical = map[string]string{
	"develop": "develop", "developed": "develop", "development": "develop",
	"emerge": "develop", "emerged": "develop", "emergent": "develop",
	"originate": "develop", "originated": "develop", "origin": "develop", "origins": "develop",
	"arise": "develop", "arose": "develop", "arises": "develop",

	"independent": "independent", "independently": "independent",
	"separate": "independent", "separately": "independent", "separation": "independent",

	"recombine": "recombine", "recombined": "recombine", "recombination": "recombine",
	"join": "recombine", "joined": "recombine", "joining": "recombine",
	"merge": "recombine", "merged": "recombine", "merging": "recombine",
	"synthesize": "recombine", "synthesized": "recombine", "synthesis": "recombine",

	"upstream": "precede", "enable": "precede", "enabled": "precede",
	"precede": "precede", "preceded": "precede", "led": "precede", "lead": "precede",

	"downstream": "map", "reinterpret": "map", "reinterpreted": "map",
	"reframe": "map", "reframed": "map", "map": "map", "mapped": "map",
	"translate": "map", "translated": "map", "translation": "map",

	"summary": "summary", "summaries": "summary", "summarize": "summary", "summarized": "summary",
	"overview": "summary", "overviews": "summary", "abstract": "summary",

	"chunk": "chunk", "chunks": "chunk", "chunking": "chunk", "segmentation": "chunk",

	"ingest": "ingest", "ingestion": "ingest", "ingested": "ingest",
	"import": "ingest", "imported": "ingest",

	"retrieve": "retrieve", "retrieval": "retrieve", "retrieved": "retrieve",

	"artifact": "produce", "artifacts": "produce", "induced": "produce",
	"produce": "produce", "produced": "produce", "generate": "produce", "generated": "produce", "generation": "produce",

	"transcript": "transcript", "transcripts": "transcript", "draft": "transcript", "drafts": "transcript",

	"branch": "branch", "branches": "branch",
	"lineage": "lineage", "hybrid": "hybrid", "hybrids": "hybrid",
	"tradition": "tradition", "traditions": "tradition",
	"term": "terminology", "terms": "terminology", "terminology": "terminology",
	"vocabulary": "vocabulary", "vocab": "vocabulary",
	"connection": "connection", "connections": "connection",
	"bridge": "bridge", "bridges": "bridge",
	"apparent": "apparent", "appears": "appear", "appear": "appear",
	"absent": "absent", "absence": "absent",
	"sacred": "sacred", "symbolic": "symbolic", "symbolism": "symbolic",
	"technical": "technical",
}

// deepDrillRelationConcepts names the canonical concepts that represent a
// relation rather than a topic. A hypothesis and evidence sharing one of these
// families is a strong relevance signal that does not depend on verbatim
// sentence overlap.
var deepDrillRelationConcepts = map[string]string{
	"develop":     "DEVELOP_ORIGIN",
	"independent": "INDEPENDENT",
	"recombine":   "RECOMBINE",
	"precede":     "PRECEDE_ENABLE",
	"map":         "REINTERPRET_MAP",
	"summary":     "SUMMARY_OVERVIEW",
	"chunk":       "CHUNKING",
	"ingest":      "INGESTION",
	"retrieve":    "RETRIEVAL",
	"produce":     "ARTIFACT_PRODUCE",
}

var deepDrillStopwords = map[string]struct{}{
	"the": {}, "and": {}, "for": {}, "are": {}, "was": {}, "were": {},
	"that": {}, "this": {}, "with": {}, "from": {}, "have": {}, "has": {},
	"had": {}, "not": {}, "but": {}, "does": {}, "did": {}, "what": {},
	"when": {}, "where": {}, "which": {}, "who": {}, "how": {}, "why": {},
	"its": {}, "they": {}, "them": {}, "their": {}, "there": {}, "here": {},
	"about": {}, "into": {}, "onto": {}, "over": {}, "under": {}, "than": {},
	"then": {}, "these": {}, "those": {}, "such": {}, "some": {}, "any": {},
	"each": {}, "both": {}, "also": {}, "can": {}, "will": {}, "would": {},
	"should": {}, "could": {}, "may": {}, "might": {}, "must": {},
}

// deepDrillClassificationRow is a diagnostic record that mirrors the current
// evidence-classification decision for a single evidence item against a single
// hypothesis. It exposes the exact terms, component scores, polarity, and final
// classification reason so a stalled run can be audited without guessing where
// evidence association broke down.
type deepDrillClassificationRow struct {
	EvidenceID       string   `json:"evidence_id"`
	HypothesisID     string   `json:"hypothesis_id"`
	UsableTextFields []string `json:"usable_text_fields,omitempty"`
	LexicalOverlap   float64  `json:"lexical_overlap"`
	ConceptMatches   []string `json:"concept_matches,omitempty"`
	RelationMatches  []string `json:"relation_matches,omitempty"`
	NegationSignals  []string `json:"negation_signals,omitempty"`
	RelevanceScore   float64  `json:"relevance_score"`
	Relevant         bool     `json:"relevant"`
	Polarity         string   `json:"polarity"`
	Reason           string   `json:"classification_reason"`
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
