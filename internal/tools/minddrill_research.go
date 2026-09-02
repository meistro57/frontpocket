package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/meistro57/frontpocket/internal/embed"
	"github.com/meistro57/frontpocket/internal/store"
	"github.com/meistro57/loadout"
)

type CollectionInspection struct {
	CollectionName           string                  `json:"collection_name"`
	CollectionExists         bool                    `json:"collection_exists"`
	EmbeddingModel           string                  `json:"embedding_model,omitempty"`
	EmbeddingProvider        string                  `json:"embedding_provider,omitempty"`
	VectorDimensions         int                     `json:"vector_dimensions,omitempty"`
	DocumentsDetected        int                     `json:"documents_detected"`
	ChunksOrVectors          int                     `json:"chunks_or_vectors"`
	UniqueSourceFiles        int                     `json:"unique_source_files"`
	MetadataFieldsPresent    []string                `json:"metadata_fields_present"`
	DateRange                map[string]string       `json:"date_range,omitempty"`
	LargestDocuments         []DocumentSizeBreakdown `json:"largest_documents,omitempty"`
	DuplicateSourceDocuments []DuplicateDocument     `json:"duplicate_source_documents,omitempty"`
	MissingProvenanceFields  []string                `json:"missing_provenance_fields"`
	CompatibilityProblems    []string                `json:"compatibility_problems,omitempty"`
	Warnings                 []string                `json:"warnings,omitempty"`
	SampledPoints            int                     `json:"sampled_points"`
}

type DocumentSizeBreakdown struct {
	DocumentID string `json:"document_id"`
	Chunks     int    `json:"chunks"`
}

type DuplicateDocument struct {
	DocumentKey string   `json:"document_key"`
	DocumentIDs []string `json:"document_ids"`
}

type ResearchSearchLedgerEntry struct {
	QueryText       string            `json:"query_text"`
	RetrievalMode   string            `json:"retrieval_mode"`
	Collection      string            `json:"collection"`
	Filters         map[string]string `json:"filters,omitempty"`
	Exclusions      []string          `json:"exclusions,omitempty"`
	Timestamp       time.Time         `json:"timestamp"`
	ResultSourceIDs []string          `json:"result_source_ids,omitempty"`
	Reason          string            `json:"reason,omitempty"`
}

type ResearchSession struct {
	SessionID         string                      `json:"session_id"`
	TargetCollection  string                      `json:"target_collection"`
	ResearchQuestion  string                      `json:"research_question,omitempty"`
	Hypotheses        []string                    `json:"hypotheses,omitempty"`
	SearchesPerformed []ResearchSearchLedgerEntry `json:"searches_performed,omitempty"`
	RetrievedSources  []string                    `json:"retrieved_sources,omitempty"`
	Evidence          []string                    `json:"evidence,omitempty"`
	Counterevidence   []string                    `json:"counterevidence,omitempty"`
	UnresolvedLeads   []string                    `json:"unresolved_leads,omitempty"`
	LineageEdges      []string                    `json:"lineage_edges,omitempty"`
	Notes             []string                    `json:"notes,omitempty"`
	UpdatedAt         time.Time                   `json:"updated_at"`
}

type ResearchResult struct {
	Collection        string         `json:"collection"`
	MemoryID          string         `json:"memory_id,omitempty"`
	SourceDocumentID  string         `json:"source_document_id,omitempty"`
	SourceTitle       string         `json:"source_title,omitempty"`
	Filename          string         `json:"filename,omitempty"`
	Path              string         `json:"path,omitempty"`
	Folder            string         `json:"folder,omitempty"`
	ChunkID           string         `json:"chunk_id,omitempty"`
	ChunkIndex        int            `json:"chunk_index,omitempty"`
	Page              string         `json:"page,omitempty"`
	SectionOrHeading  string         `json:"section_or_heading,omitempty"`
	SourceType        string         `json:"source_type,omitempty"`
	CreatedDate       string         `json:"created_date,omitempty"`
	ModifiedDate      string         `json:"modified_date,omitempty"`
	SimilarityScore   float64        `json:"similarity_score"`
	SourceFingerprint string         `json:"source_fingerprint,omitempty"`
	Text              string         `json:"text,omitempty"`
	Summary           string         `json:"summary,omitempty"`
	SourceQuote       string         `json:"source_quote,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	RetrievalMetadata map[string]any `json:"retrieval_metadata,omitempty"`
}

type researchSessionStore struct {
	mu       sync.Mutex
	filePath string
	sessions map[string]ResearchSession
}

type researchTool struct {
	name        string
	description string
	params      json.RawMessage
	run         func(context.Context, string) (string, error)
}

type MindDrillResearchRuntime struct {
	client     *store.QdrantClient
	embedder   embed.Embedder
	vectorName string
	logger     *slog.Logger
	sessions   *researchSessionStore
}

func (t *researchTool) Definition() loadout.ToolDefinition {
	return loadout.ToolDefinition{Name: t.name, Description: t.description, Parameters: t.params}
}

func (t *researchTool) Execute(ctx context.Context, argumentsJSON string) (string, error) {
	if t.run == nil {
		return "error: tool is not configured", nil
	}
	return t.run(ctx, argumentsJSON)
}

func NewMindDrillResearchRuntime(client *store.QdrantClient, embedder embed.Embedder, vectorName string, logger *slog.Logger) *MindDrillResearchRuntime {
	if logger == nil {
		logger = slog.Default()
	}
	filePath := strings.TrimSpace(os.Getenv("MINDDRILL_RESEARCH_SESSION_FILE"))
	if filePath == "" {
		filePath = "data/minddrill_research_sessions.json"
	}
	return &MindDrillResearchRuntime{
		client:     client,
		embedder:   embedder,
		vectorName: strings.TrimSpace(vectorName),
		logger:     logger,
		sessions:   newResearchSessionStore(filePath),
	}
}

func NewMindDrillResearchTools(runtime *MindDrillResearchRuntime) []*researchTool {
	if runtime == nil {
		return nil
	}
	return []*researchTool{
		runtime.newInspectCollectionTool(),
		runtime.newBindCollectionTool(),
		runtime.newSemanticSearchTool(),
		runtime.newKeywordSearchTool(),
		runtime.newMetadataSearchTool(),
		runtime.newDocumentChunksTool(),
		runtime.newNeighborChunksTool(),
		runtime.newRelatedOutsideTool(),
		runtime.newExcludeSearchTool(),
	}
}

func InspectCollection(ctx context.Context, runtime *MindDrillResearchRuntime, collection string, sampleLimit int) (CollectionInspection, error) {
	if runtime == nil || runtime.client == nil {
		return CollectionInspection{}, fmt.Errorf("minddrill research runtime is not configured")
	}
	return runtime.inspectCollection(ctx, collection, sampleLimit)
}

func newResearchSessionStore(filePath string) *researchSessionStore {
	store := &researchSessionStore{filePath: strings.TrimSpace(filePath), sessions: make(map[string]ResearchSession)}
	store.load()
	return store
}

func (s *researchSessionStore) load() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.filePath == "" {
		return
	}
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}
	if len(data) == 0 {
		return
	}
	_ = json.Unmarshal(data, &s.sessions)
}

func (s *researchSessionStore) saveLocked() {
	if s.filePath == "" {
		return
	}
	dir := filepath.Dir(s.filePath)
	if strings.TrimSpace(dir) != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	data, err := json.MarshalIndent(s.sessions, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(s.filePath, data, 0o644)
}

func (s *researchSessionStore) get(sessionID string) (ResearchSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[strings.TrimSpace(sessionID)]
	return session, ok
}

func (s *researchSessionStore) put(session ResearchSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(session.SessionID) == "" {
		return
	}
	s.sessions[session.SessionID] = session
	s.saveLocked()
}

func (rt *MindDrillResearchRuntime) inspectCollection(ctx context.Context, collection string, sampleLimit int) (CollectionInspection, error) {
	collection = strings.TrimSpace(collection)
	if collection == "" {
		return CollectionInspection{}, fmt.Errorf("collection is required")
	}
	if sampleLimit <= 0 {
		sampleLimit = 1200
	}
	if sampleLimit > 12000 {
		sampleLimit = 12000
	}

	descriptor, err := rt.client.DescribeCollection(ctx, collection, rt.vectorName)
	if err != nil {
		return CollectionInspection{}, err
	}
	inspection := CollectionInspection{
		CollectionName:          collection,
		CollectionExists:        descriptor.Exists,
		VectorDimensions:        descriptor.VectorSize,
		ChunksOrVectors:         descriptor.Points,
		MetadataFieldsPresent:   []string{},
		MissingProvenanceFields: []string{},
		Warnings:                []string{},
	}
	if !descriptor.Exists {
		inspection.CompatibilityProblems = append(inspection.CompatibilityProblems, "collection not found")
		return inspection, nil
	}

	payloads, warnings, err := rt.sampleCollectionPayloads(ctx, collection, sampleLimit)
	if err != nil {
		return CollectionInspection{}, err
	}
	inspection.Warnings = append(inspection.Warnings, warnings...)
	inspection.SampledPoints = len(payloads)
	if len(payloads) == 0 {
		inspection.Warnings = append(inspection.Warnings, "collection has no sampled payloads")
		return inspection, nil
	}

	fieldSet := make(map[string]struct{})
	sourceIDs := make(map[string]struct{})
	sourceFiles := make(map[string]struct{})
	docChunkCounts := make(map[string]int)
	dupSeeds := make(map[string]map[string]struct{})
	var minDate time.Time
	var maxDate time.Time
	embeddingModels := make(map[string]int)
	embeddingProviders := make(map[string]int)

	for _, payload := range payloads {
		for key := range payload {
			trimmed := strings.TrimSpace(key)
			if trimmed != "" {
				fieldSet[trimmed] = struct{}{}
			}
		}
		prov := normalizeProvenance(collection, payload, 0)
		if prov.SourceFingerprint != "" {
			sourceIDs[prov.SourceFingerprint] = struct{}{}
			docChunkCounts[prov.SourceFingerprint]++
		}
		if fileKey := firstNonEmpty(prov.Path, prov.Filename, prov.SourceTitle); fileKey != "" {
			sourceFiles[fileKey] = struct{}{}
		}
		dupKey := normalizeDupKey(prov)
		if dupKey != "" {
			if _, ok := dupSeeds[dupKey]; !ok {
				dupSeeds[dupKey] = make(map[string]struct{})
			}
			if prov.SourceFingerprint != "" {
				dupSeeds[dupKey][prov.SourceFingerprint] = struct{}{}
			}
		}
		ts := parseAnyTime(firstPayloadValue(payload, []string{"timestamp", "created_at", "created_date", "date", "modified_at", "modified_date"}))
		if !ts.IsZero() {
			if minDate.IsZero() || ts.Before(minDate) {
				minDate = ts
			}
			if maxDate.IsZero() || ts.After(maxDate) {
				maxDate = ts
			}
		}
		model := strings.TrimSpace(asString(firstPayloadValue(payload, []string{"embedding_model", "embed_model", "model"})))
		if model != "" {
			embeddingModels[model]++
		}
		provider := strings.TrimSpace(asString(firstPayloadValue(payload, []string{"embedding_provider", "embed_provider"})))
		if provider != "" {
			embeddingProviders[provider]++
		}
	}

	inspection.DocumentsDetected = len(sourceIDs)
	inspection.UniqueSourceFiles = len(sourceFiles)
	inspection.MetadataFieldsPresent = mapKeysSorted(fieldSet)
	if !minDate.IsZero() || !maxDate.IsZero() {
		inspection.DateRange = map[string]string{}
		if !minDate.IsZero() {
			inspection.DateRange["start"] = minDate.Format(time.RFC3339)
		}
		if !maxDate.IsZero() {
			inspection.DateRange["end"] = maxDate.Format(time.RFC3339)
		}
	}
	inspection.LargestDocuments = topDocuments(docChunkCounts, 8)
	inspection.DuplicateSourceDocuments = obviousDuplicates(dupSeeds)
	inspection.MissingProvenanceFields = missingProvenanceFields(fieldSet)
	inspection.EmbeddingModel = mostCommonKey(embeddingModels)
	inspection.EmbeddingProvider = mostCommonKey(embeddingProviders)

	problems := make([]string, 0)
	if rt.embedder != nil && descriptor.VectorSize > 0 {
		queryVector, embedErr := rt.embedder.EmbedText(ctx, "minddrill compatibility probe")
		if embedErr != nil {
			problems = append(problems, "could not verify query embedding compatibility: "+embedErr.Error())
		} else if len(queryVector) != descriptor.VectorSize {
			problems = append(problems, fmt.Sprintf("query embedding dimensions (%d) do not match collection vector size (%d)", len(queryVector), descriptor.VectorSize))
		}
	}
	if descriptor.VectorSize <= 0 {
		problems = append(problems, "collection vector dimensions are unknown")
	}
	if inspection.EmbeddingModel == "" {
		problems = append(problems, "collection-level embedding model metadata not found in sampled payloads")
	}
	inspection.CompatibilityProblems = problems
	if descriptor.Points > len(payloads) {
		inspection.Warnings = append(inspection.Warnings, fmt.Sprintf("inspection sampled %d/%d vectors", len(payloads), descriptor.Points))
	}
	return inspection, nil
}

func (rt *MindDrillResearchRuntime) sampleCollectionPayloads(ctx context.Context, collection string, sampleLimit int) ([]map[string]any, []string, error) {
	payloads := make([]map[string]any, 0, sampleLimit)
	warnings := make([]string, 0)
	var offset any
	for len(payloads) < sampleLimit {
		pageLimit := 256
		if remaining := sampleLimit - len(payloads); remaining < pageLimit {
			pageLimit = remaining
		}
		page, next, err := rt.client.ScrollCollection(ctx, store.QdrantScrollRequest{Collection: collection, Limit: pageLimit, Offset: offset})
		if err != nil {
			return nil, warnings, err
		}
		if len(page) == 0 {
			break
		}
		payloads = append(payloads, page...)
		if next == nil {
			break
		}
		nextValue := strings.TrimSpace(fmt.Sprintf("%v", next))
		if nextValue == "" || nextValue == strings.TrimSpace(fmt.Sprintf("%v", offset)) {
			break
		}
		offset = next
	}
	if len(payloads) == sampleLimit {
		warnings = append(warnings, "sample limit reached before full collection scan")
	}
	return payloads, warnings, nil
}

func (rt *MindDrillResearchRuntime) ensureSession(sessionID string) (ResearchSession, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ResearchSession{}, fmt.Errorf("session_id is required")
	}
	if existing, ok := rt.sessions.get(sessionID); ok {
		return existing, nil
	}
	session := ResearchSession{SessionID: sessionID, UpdatedAt: time.Now().UTC()}
	rt.sessions.put(session)
	return session, nil
}

func (rt *MindDrillResearchRuntime) requireBoundCollection(sessionID string) (ResearchSession, string, error) {
	session, err := rt.ensureSession(sessionID)
	if err != nil {
		return ResearchSession{}, "", err
	}
	collection := strings.TrimSpace(session.TargetCollection)
	if collection == "" {
		return session, "", fmt.Errorf("session %q is not bound to a collection. Run bind_research_collection first", sessionID)
	}
	descriptor, err := rt.client.DescribeCollection(context.Background(), collection, rt.vectorName)
	if err != nil {
		return session, "", err
	}
	if !descriptor.Exists {
		return session, "", fmt.Errorf("bound collection %q does not exist", collection)
	}
	return session, collection, nil
}

func (rt *MindDrillResearchRuntime) appendLedger(session *ResearchSession, entry ResearchSearchLedgerEntry) {
	if session == nil {
		return
	}
	entry.Timestamp = time.Now().UTC()
	session.SearchesPerformed = append(session.SearchesPerformed, entry)
	session.UpdatedAt = time.Now().UTC()
	rt.sessions.put(*session)
}

func (rt *MindDrillResearchRuntime) ensureEmbeddingCompatibility(ctx context.Context, collection string) (store.QdrantCollectionDescriptor, []string, error) {
	descriptor, err := rt.client.DescribeCollection(ctx, collection, rt.vectorName)
	if err != nil {
		return store.QdrantCollectionDescriptor{}, nil, err
	}
	if !descriptor.Exists {
		return descriptor, nil, fmt.Errorf("collection %q not found", collection)
	}
	warnings := make([]string, 0)
	if rt.embedder == nil {
		warnings = append(warnings, "embedder is not configured, compatibility could not be determined")
		return descriptor, warnings, nil
	}
	probe, err := rt.embedder.EmbedText(ctx, "minddrill compatibility probe")
	if err != nil {
		warnings = append(warnings, "query embedding failed during compatibility check: "+err.Error())
		return descriptor, warnings, nil
	}
	if descriptor.VectorSize > 0 && len(probe) != descriptor.VectorSize {
		return descriptor, warnings, fmt.Errorf("incompatible embedding spaces: query vector has %d dimensions but collection %q expects %d", len(probe), collection, descriptor.VectorSize)
	}
	if descriptor.VectorSize == 0 {
		warnings = append(warnings, "collection vector dimensions are unknown, compatibility could not be fully determined")
	}
	return descriptor, warnings, nil
}

func (rt *MindDrillResearchRuntime) semanticSearch(ctx context.Context, collection, query string, limit int, filters map[string]string, excludedSourceFingerprints []string, maxChunksPerDoc int, rawNearest bool) ([]ResearchResult, []string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil, fmt.Errorf("query is required")
	}
	if rt.embedder == nil {
		return nil, nil, fmt.Errorf("embedder is not configured")
	}
	if limit <= 0 {
		limit = 8
	}
	if limit > 50 {
		limit = 50
	}

	descriptor, warnings, err := rt.ensureEmbeddingCompatibility(ctx, collection)
	if err != nil {
		return nil, warnings, err
	}
	vector, err := rt.embedder.EmbedText(ctx, query)
	if err != nil {
		return nil, warnings, err
	}
	if descriptor.VectorSize > 0 && len(vector) != descriptor.VectorSize {
		return nil, warnings, fmt.Errorf("incompatible embedding spaces: query vector has %d dimensions but collection %q expects %d", len(vector), collection, descriptor.VectorSize)
	}

	searchLimit := limit
	if maxChunksPerDoc > 0 && !rawNearest {
		searchLimit = limit * 5
		if searchLimit > 200 {
			searchLimit = 200
		}
	}

	hits, err := rt.client.SearchCollection(ctx, store.QdrantSearchRequest{
		Collection: collection,
		VectorName: rt.vectorName,
		Vector:     vector,
		Limit:      searchLimit,
		Filter:     buildFilter(filters),
	})
	if err != nil {
		return nil, warnings, err
	}

	excluded := make(map[string]struct{}, len(excludedSourceFingerprints))
	for _, item := range excludedSourceFingerprints {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			excluded[trimmed] = struct{}{}
		}
	}

	results := make([]ResearchResult, 0, len(hits))
	perDocCount := make(map[string]int)
	for _, hit := range hits {
		candidate := normalizeProvenance(collection, hit.Payload, hit.Score)
		if matchesExcluded(candidate, excluded) {
			continue
		}
		if maxChunksPerDoc > 0 && !rawNearest {
			docKey := candidate.SourceFingerprint
			if docKey == "" {
				docKey = "_unknown"
			}
			if perDocCount[docKey] >= maxChunksPerDoc {
				continue
			}
			perDocCount[docKey]++
		}
		results = append(results, candidate)
		if len(results) >= limit {
			break
		}
	}
	return results, warnings, nil
}

func (rt *MindDrillResearchRuntime) keywordSearch(ctx context.Context, collection, query string, limit int, filters map[string]string, excludedSourceFingerprints []string) ([]ResearchResult, []string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil, fmt.Errorf("query is required")
	}
	if limit <= 0 {
		limit = 8
	}
	if limit > 50 {
		limit = 50
	}

	tokens := tokenizeQuery(query)
	excluded := make(map[string]struct{}, len(excludedSourceFingerprints))
	for _, item := range excludedSourceFingerprints {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			excluded[trimmed] = struct{}{}
		}
	}

	const scanCap = 3000
	results := make([]ResearchResult, 0, limit)
	warnings := []string{"keyword search scanned payload text because backend keyword indexes are not guaranteed"}
	var offset any
	scanned := 0
	for len(results) < limit && scanned < scanCap {
		page, next, err := rt.client.ScrollCollection(ctx, store.QdrantScrollRequest{Collection: collection, Limit: 256, Offset: offset, Filter: buildFilter(filters)})
		if err != nil {
			return nil, warnings, err
		}
		if len(page) == 0 {
			break
		}
		for _, payload := range page {
			scanned++
			item := normalizeProvenance(collection, payload, 0)
			if matchesExcluded(item, excluded) {
				continue
			}
			score := keywordMatchScore(tokens, item)
			if score <= 0 {
				continue
			}
			item.SimilarityScore = score
			results = append(results, item)
			if len(results) >= limit || scanned >= scanCap {
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
	if scanned >= scanCap {
		warnings = append(warnings, "keyword scan reached cap; results may be incomplete")
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].SimilarityScore == results[j].SimilarityScore {
			return results[i].MemoryID < results[j].MemoryID
		}
		return results[i].SimilarityScore > results[j].SimilarityScore
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, warnings, nil
}

func (rt *MindDrillResearchRuntime) getDocumentChunks(ctx context.Context, collection, documentID string, limit int) ([]ResearchResult, error) {
	documentID = strings.TrimSpace(documentID)
	if documentID == "" {
		return nil, fmt.Errorf("document_id is required")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	const scanCap = 5000
	results := make([]ResearchResult, 0, limit)
	var offset any
	scanned := 0
	for len(results) < limit && scanned < scanCap {
		page, next, err := rt.client.ScrollCollection(ctx, store.QdrantScrollRequest{Collection: collection, Limit: 256, Offset: offset})
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		for _, payload := range page {
			scanned++
			item := normalizeProvenance(collection, payload, 0)
			if item.SourceFingerprint == documentID || item.SourceDocumentID == documentID || item.Path == documentID || item.Filename == documentID {
				results = append(results, item)
				if len(results) >= limit {
					break
				}
			}
			if scanned >= scanCap {
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
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].ChunkIndex == results[j].ChunkIndex {
			return results[i].MemoryID < results[j].MemoryID
		}
		return results[i].ChunkIndex < results[j].ChunkIndex
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (rt *MindDrillResearchRuntime) getNeighborChunks(ctx context.Context, collection, anchorChunkID string, window int) ([]ResearchResult, error) {
	anchorChunkID = strings.TrimSpace(anchorChunkID)
	if anchorChunkID == "" {
		return nil, fmt.Errorf("chunk_id is required")
	}
	if window <= 0 {
		window = 2
	}
	if window > 10 {
		window = 10
	}
	const scanCap = 5000
	all := make([]ResearchResult, 0, 128)
	var anchor ResearchResult
	foundAnchor := false
	var offset any
	scanned := 0
	for scanned < scanCap {
		page, next, err := rt.client.ScrollCollection(ctx, store.QdrantScrollRequest{Collection: collection, Limit: 256, Offset: offset})
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		for _, payload := range page {
			scanned++
			item := normalizeProvenance(collection, payload, 0)
			all = append(all, item)
			if item.ChunkID == anchorChunkID || item.MemoryID == anchorChunkID {
				anchor = item
				foundAnchor = true
			}
			if scanned >= scanCap {
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
	if !foundAnchor {
		return nil, fmt.Errorf("chunk %q not found in collection", anchorChunkID)
	}

	docID := anchor.SourceFingerprint
	if docID == "" {
		docID = anchor.SourceDocumentID
	}
	sameDoc := make([]ResearchResult, 0)
	for _, item := range all {
		if item.SourceFingerprint == docID {
			sameDoc = append(sameDoc, item)
		}
	}
	if len(sameDoc) == 0 {
		return nil, nil
	}
	sort.SliceStable(sameDoc, func(i, j int) bool {
		if sameDoc[i].ChunkIndex == sameDoc[j].ChunkIndex {
			return sameDoc[i].MemoryID < sameDoc[j].MemoryID
		}
		return sameDoc[i].ChunkIndex < sameDoc[j].ChunkIndex
	})
	anchorIdx := 0
	for idx, item := range sameDoc {
		if item.ChunkID == anchorChunkID || item.MemoryID == anchorChunkID {
			anchorIdx = idx
			break
		}
	}
	start := anchorIdx - window
	if start < 0 {
		start = 0
	}
	end := anchorIdx + window + 1
	if end > len(sameDoc) {
		end = len(sameDoc)
	}
	return sameDoc[start:end], nil
}

func (rt *MindDrillResearchRuntime) newInspectCollectionTool() *researchTool {
	return &researchTool{
		name:        "inspect_collection",
		description: "Inspect a vector collection as a research corpus, including schema hints, source-document structure, provenance coverage, and compatibility warnings.",
		params:      json.RawMessage(`{"type":"object","properties":{"collection":{"type":"string"},"sample_limit":{"type":"integer","minimum":50,"maximum":12000}},"required":["collection"]}`),
		run: func(ctx context.Context, argumentsJSON string) (string, error) {
			var args struct {
				Collection string `json:"collection"`
				Sample     int    `json:"sample_limit"`
			}
			if strings.TrimSpace(argumentsJSON) != "" {
				if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
					return fmt.Sprintf("error: invalid arguments: %v", err), nil
				}
			}
			report, err := rt.inspectCollection(ctx, args.Collection, args.Sample)
			if err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			return toJSON(report), nil
		},
	}
}

func (rt *MindDrillResearchRuntime) newBindCollectionTool() *researchTool {
	return &researchTool{
		name:        "bind_research_collection",
		description: "Bind a research session to an explicit target collection and optionally set the research question and hypotheses. Fails clearly when the collection is missing or incompatible.",
		params:      json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string"},"collection":{"type":"string"},"research_question":{"type":"string"},"hypotheses":{"type":"array","items":{"type":"string"}}},"required":["session_id","collection"]}`),
		run: func(ctx context.Context, argumentsJSON string) (string, error) {
			var args struct {
				SessionID        string   `json:"session_id"`
				Collection       string   `json:"collection"`
				ResearchQuestion string   `json:"research_question"`
				Hypotheses       []string `json:"hypotheses"`
			}
			if strings.TrimSpace(argumentsJSON) != "" {
				if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
					return fmt.Sprintf("error: invalid arguments: %v", err), nil
				}
			}
			session, err := rt.ensureSession(args.SessionID)
			if err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			descriptor, warnings, err := rt.ensureEmbeddingCompatibility(ctx, strings.TrimSpace(args.Collection))
			if err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			session.TargetCollection = descriptor.Name
			session.ResearchQuestion = strings.TrimSpace(args.ResearchQuestion)
			session.Hypotheses = uniqueTrimmed(args.Hypotheses)
			session.UpdatedAt = time.Now().UTC()
			rt.sessions.put(session)
			rt.logger.Info("minddrill research collection bound", "session_id", session.SessionID, "collection", descriptor.Name)
			return toJSON(map[string]any{"session": session, "warnings": warnings}), nil
		},
	}
}

func (rt *MindDrillResearchRuntime) newSemanticSearchTool() *researchTool {
	return &researchTool{
		name:        "semantic_search",
		description: "Run semantic retrieval in the session-bound collection with optional metadata filters, document exclusions, and diversity control.",
		params:      json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string"},"query":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":50},"filters":{"type":"object","additionalProperties":{"type":"string"}},"exclude_documents":{"type":"array","items":{"type":"string"}},"max_chunks_per_document":{"type":"integer","minimum":1,"maximum":20},"raw_nearest_neighbor":{"type":"boolean"},"reason":{"type":"string"}},"required":["session_id","query"]}`),
		run: func(ctx context.Context, argumentsJSON string) (string, error) {
			var args struct {
				SessionID          string            `json:"session_id"`
				Query              string            `json:"query"`
				Limit              int               `json:"limit"`
				Filters            map[string]string `json:"filters"`
				ExcludeDocuments   []string          `json:"exclude_documents"`
				MaxChunksPerSource int               `json:"max_chunks_per_document"`
				RawNearest         bool              `json:"raw_nearest_neighbor"`
				Reason             string            `json:"reason"`
			}
			if strings.TrimSpace(argumentsJSON) != "" {
				if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
					return fmt.Sprintf("error: invalid arguments: %v", err), nil
				}
			}
			session, collection, err := rt.requireBoundCollection(args.SessionID)
			if err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			results, warnings, err := rt.semanticSearch(ctx, collection, args.Query, args.Limit, args.Filters, uniqueTrimmed(args.ExcludeDocuments), args.MaxChunksPerSource, args.RawNearest)
			if err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			sources := uniqueSourceFingerprints(results)
			session.RetrievedSources = uniqueMerge(session.RetrievedSources, sources)
			rt.appendLedger(&session, ResearchSearchLedgerEntry{QueryText: strings.TrimSpace(args.Query), RetrievalMode: mapMode(args.RawNearest, "semantic_raw_nn", "semantic"), Collection: collection, Filters: cleanFilterMap(args.Filters), Exclusions: uniqueTrimmed(args.ExcludeDocuments), ResultSourceIDs: sources, Reason: strings.TrimSpace(args.Reason)})
			return toJSON(map[string]any{"results": results, "warnings": warnings, "session": session}), nil
		},
	}
}

func (rt *MindDrillResearchRuntime) newKeywordSearchTool() *researchTool {
	return &researchTool{
		name:        "keyword_search",
		description: "Run exact/keyword text matching over the session-bound collection with optional metadata filters and source exclusions.",
		params:      json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string"},"query":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":50},"filters":{"type":"object","additionalProperties":{"type":"string"}},"exclude_documents":{"type":"array","items":{"type":"string"}},"reason":{"type":"string"}},"required":["session_id","query"]}`),
		run: func(ctx context.Context, argumentsJSON string) (string, error) {
			var args struct {
				SessionID        string            `json:"session_id"`
				Query            string            `json:"query"`
				Limit            int               `json:"limit"`
				Filters          map[string]string `json:"filters"`
				ExcludeDocuments []string          `json:"exclude_documents"`
				Reason           string            `json:"reason"`
			}
			if strings.TrimSpace(argumentsJSON) != "" {
				if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
					return fmt.Sprintf("error: invalid arguments: %v", err), nil
				}
			}
			session, collection, err := rt.requireBoundCollection(args.SessionID)
			if err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			results, warnings, err := rt.keywordSearch(ctx, collection, args.Query, args.Limit, args.Filters, args.ExcludeDocuments)
			if err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			sources := uniqueSourceFingerprints(results)
			session.RetrievedSources = uniqueMerge(session.RetrievedSources, sources)
			rt.appendLedger(&session, ResearchSearchLedgerEntry{QueryText: strings.TrimSpace(args.Query), RetrievalMode: "keyword", Collection: collection, Filters: cleanFilterMap(args.Filters), Exclusions: uniqueTrimmed(args.ExcludeDocuments), ResultSourceIDs: sources, Reason: strings.TrimSpace(args.Reason)})
			return toJSON(map[string]any{"results": results, "warnings": warnings, "session": session}), nil
		},
	}
}

func (rt *MindDrillResearchRuntime) newMetadataSearchTool() *researchTool {
	return &researchTool{
		name:        "search_with_metadata",
		description: "Run semantic retrieval with explicit metadata filters in the bound collection.",
		params:      json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string"},"query":{"type":"string"},"filters":{"type":"object","additionalProperties":{"type":"string"}},"limit":{"type":"integer","minimum":1,"maximum":50},"exclude_documents":{"type":"array","items":{"type":"string"}},"reason":{"type":"string"}},"required":["session_id","query","filters"]}`),
		run: func(ctx context.Context, argumentsJSON string) (string, error) {
			var args struct {
				SessionID        string            `json:"session_id"`
				Query            string            `json:"query"`
				Filters          map[string]string `json:"filters"`
				Limit            int               `json:"limit"`
				ExcludeDocuments []string          `json:"exclude_documents"`
				Reason           string            `json:"reason"`
			}
			if strings.TrimSpace(argumentsJSON) != "" {
				if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
					return fmt.Sprintf("error: invalid arguments: %v", err), nil
				}
			}
			session, collection, err := rt.requireBoundCollection(args.SessionID)
			if err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			results, warnings, err := rt.semanticSearch(ctx, collection, args.Query, args.Limit, args.Filters, args.ExcludeDocuments, 0, false)
			if err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			sources := uniqueSourceFingerprints(results)
			session.RetrievedSources = uniqueMerge(session.RetrievedSources, sources)
			rt.appendLedger(&session, ResearchSearchLedgerEntry{QueryText: strings.TrimSpace(args.Query), RetrievalMode: "semantic_metadata", Collection: collection, Filters: cleanFilterMap(args.Filters), Exclusions: uniqueTrimmed(args.ExcludeDocuments), ResultSourceIDs: sources, Reason: strings.TrimSpace(args.Reason)})
			return toJSON(map[string]any{"results": results, "warnings": warnings, "session": session}), nil
		},
	}
}

func (rt *MindDrillResearchRuntime) newDocumentChunksTool() *researchTool {
	return &researchTool{
		name:        "get_document_chunks",
		description: "Retrieve additional chunks from the same source document inside the bound collection.",
		params:      json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string"},"document_id":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":200},"reason":{"type":"string"}},"required":["session_id","document_id"]}`),
		run: func(ctx context.Context, argumentsJSON string) (string, error) {
			var args struct {
				SessionID  string `json:"session_id"`
				DocumentID string `json:"document_id"`
				Limit      int    `json:"limit"`
				Reason     string `json:"reason"`
			}
			if strings.TrimSpace(argumentsJSON) != "" {
				if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
					return fmt.Sprintf("error: invalid arguments: %v", err), nil
				}
			}
			session, collection, err := rt.requireBoundCollection(args.SessionID)
			if err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			results, err := rt.getDocumentChunks(ctx, collection, args.DocumentID, args.Limit)
			if err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			sources := uniqueSourceFingerprints(results)
			session.RetrievedSources = uniqueMerge(session.RetrievedSources, sources)
			rt.appendLedger(&session, ResearchSearchLedgerEntry{QueryText: args.DocumentID, RetrievalMode: "document_chunks", Collection: collection, ResultSourceIDs: sources, Reason: strings.TrimSpace(args.Reason)})
			return toJSON(map[string]any{"results": results, "session": session}), nil
		},
	}
}

func (rt *MindDrillResearchRuntime) newNeighborChunksTool() *researchTool {
	return &researchTool{
		name:        "get_neighbor_chunks",
		description: "Retrieve surrounding chunks from the same source document around a given chunk or memory id.",
		params:      json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string"},"chunk_id":{"type":"string"},"window":{"type":"integer","minimum":1,"maximum":10},"reason":{"type":"string"}},"required":["session_id","chunk_id"]}`),
		run: func(ctx context.Context, argumentsJSON string) (string, error) {
			var args struct {
				SessionID string `json:"session_id"`
				ChunkID   string `json:"chunk_id"`
				Window    int    `json:"window"`
				Reason    string `json:"reason"`
			}
			if strings.TrimSpace(argumentsJSON) != "" {
				if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
					return fmt.Sprintf("error: invalid arguments: %v", err), nil
				}
			}
			session, collection, err := rt.requireBoundCollection(args.SessionID)
			if err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			results, err := rt.getNeighborChunks(ctx, collection, args.ChunkID, args.Window)
			if err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			sources := uniqueSourceFingerprints(results)
			session.RetrievedSources = uniqueMerge(session.RetrievedSources, sources)
			rt.appendLedger(&session, ResearchSearchLedgerEntry{QueryText: args.ChunkID, RetrievalMode: "neighbor_chunks", Collection: collection, ResultSourceIDs: sources, Reason: strings.TrimSpace(args.Reason)})
			return toJSON(map[string]any{"results": results, "session": session}), nil
		},
	}
}

func (rt *MindDrillResearchRuntime) newRelatedOutsideTool() *researchTool {
	return &researchTool{
		name:        "find_related_outside_document",
		description: "Find semantically related passages from other documents, excluding a specified source document.",
		params:      json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string"},"query":{"type":"string"},"source_document_id":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":50},"reason":{"type":"string"}},"required":["session_id","query","source_document_id"]}`),
		run: func(ctx context.Context, argumentsJSON string) (string, error) {
			var args struct {
				SessionID        string `json:"session_id"`
				Query            string `json:"query"`
				SourceDocumentID string `json:"source_document_id"`
				Limit            int    `json:"limit"`
				Reason           string `json:"reason"`
			}
			if strings.TrimSpace(argumentsJSON) != "" {
				if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
					return fmt.Sprintf("error: invalid arguments: %v", err), nil
				}
			}
			session, collection, err := rt.requireBoundCollection(args.SessionID)
			if err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			excluded := []string{strings.TrimSpace(args.SourceDocumentID)}
			results, warnings, err := rt.semanticSearch(ctx, collection, args.Query, args.Limit, nil, excluded, 0, false)
			if err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			sources := uniqueSourceFingerprints(results)
			session.RetrievedSources = uniqueMerge(session.RetrievedSources, sources)
			rt.appendLedger(&session, ResearchSearchLedgerEntry{QueryText: strings.TrimSpace(args.Query), RetrievalMode: "related_outside_document", Collection: collection, Exclusions: excluded, ResultSourceIDs: sources, Reason: strings.TrimSpace(args.Reason)})
			return toJSON(map[string]any{"results": results, "warnings": warnings, "session": session}), nil
		},
	}
}

func (rt *MindDrillResearchRuntime) newExcludeSearchTool() *researchTool {
	return &researchTool{
		name:        "search_excluding_documents",
		description: "Run semantic search while excluding one or more source documents.",
		params:      json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string"},"query":{"type":"string"},"exclude_documents":{"type":"array","items":{"type":"string"}},"limit":{"type":"integer","minimum":1,"maximum":50},"reason":{"type":"string"}},"required":["session_id","query","exclude_documents"]}`),
		run: func(ctx context.Context, argumentsJSON string) (string, error) {
			var args struct {
				SessionID        string   `json:"session_id"`
				Query            string   `json:"query"`
				ExcludeDocuments []string `json:"exclude_documents"`
				Limit            int      `json:"limit"`
				Reason           string   `json:"reason"`
			}
			if strings.TrimSpace(argumentsJSON) != "" {
				if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
					return fmt.Sprintf("error: invalid arguments: %v", err), nil
				}
			}
			session, collection, err := rt.requireBoundCollection(args.SessionID)
			if err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			excluded := uniqueTrimmed(args.ExcludeDocuments)
			results, warnings, err := rt.semanticSearch(ctx, collection, args.Query, args.Limit, nil, excluded, 0, false)
			if err != nil {
				return fmt.Sprintf("error: %v", err), nil
			}
			sources := uniqueSourceFingerprints(results)
			session.RetrievedSources = uniqueMerge(session.RetrievedSources, sources)
			rt.appendLedger(&session, ResearchSearchLedgerEntry{QueryText: strings.TrimSpace(args.Query), RetrievalMode: "semantic_excluding_documents", Collection: collection, Exclusions: excluded, ResultSourceIDs: sources, Reason: strings.TrimSpace(args.Reason)})
			return toJSON(map[string]any{"results": results, "warnings": warnings, "session": session}), nil
		},
	}
}

func normalizeProvenance(collection string, payload map[string]any, score float64) ResearchResult {
	cloned := make(map[string]any, len(payload))
	for key, value := range payload {
		cloned[key] = value
	}
	item := ResearchResult{
		Collection:        collection,
		MemoryID:          strings.TrimSpace(asString(firstPayloadValue(payload, []string{"memory_id", "id"}))),
		SourceDocumentID:  strings.TrimSpace(asString(firstPayloadValue(payload, []string{"source_document_id", "document_id", "doc_id", "source_id"}))),
		SourceTitle:       strings.TrimSpace(asString(firstPayloadValue(payload, []string{"source_title", "title", "document_title"}))),
		Filename:          strings.TrimSpace(asString(firstPayloadValue(payload, []string{"filename", "file_name", "attachment_filename"}))),
		Path:              strings.TrimSpace(asString(firstPayloadValue(payload, []string{"path", "file_path", "source_path"}))),
		Folder:            strings.TrimSpace(asString(firstPayloadValue(payload, []string{"folder", "directory", "source_folder"}))),
		ChunkID:           strings.TrimSpace(asString(firstPayloadValue(payload, []string{"chunk_id", "chunk_uid"}))),
		ChunkIndex:        asInt(firstPayloadValue(payload, []string{"chunk_index", "chunk_idx", "position", "segment_index"})),
		Page:              strings.TrimSpace(asString(firstPayloadValue(payload, []string{"page", "page_number"}))),
		SectionOrHeading:  strings.TrimSpace(asString(firstPayloadValue(payload, []string{"section", "heading", "header"}))),
		SourceType:        strings.TrimSpace(asString(firstPayloadValue(payload, []string{"source_type", "document_type"}))),
		CreatedDate:       normalizeDate(firstPayloadValue(payload, []string{"created_date", "created_at"})),
		ModifiedDate:      normalizeDate(firstPayloadValue(payload, []string{"modified_date", "modified_at", "updated_at"})),
		SimilarityScore:   score,
		Text:              strings.TrimSpace(asString(firstPayloadValue(payload, []string{"text", "content", "chunk_text"}))),
		Summary:           strings.TrimSpace(asString(firstPayloadValue(payload, []string{"summary", "abstract"}))),
		SourceQuote:       strings.TrimSpace(asString(firstPayloadValue(payload, []string{"source_quote", "quote"}))),
		Metadata:          cloned,
		RetrievalMetadata: map[string]any{"collection": collection, "similarity_score": score},
	}
	if item.ChunkID == "" {
		item.ChunkID = item.MemoryID
	}
	item.SourceFingerprint = sourceFingerprint(item)
	return item
}

func sourceFingerprint(item ResearchResult) string {
	return firstNonEmpty(item.SourceDocumentID, item.Path, item.Filename, item.SourceTitle)
}

func firstPayloadValue(payload map[string]any, keys []string) any {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			return value
		}
	}
	return nil
}

func buildFilter(filters map[string]string) map[string]any {
	if len(filters) == 0 {
		return nil
	}
	must := make([]map[string]any, 0, len(filters))
	for key, value := range filters {
		trimmedKey := strings.TrimSpace(key)
		trimmedValue := strings.TrimSpace(value)
		if trimmedKey == "" || trimmedValue == "" {
			continue
		}
		must = append(must, map[string]any{"key": trimmedKey, "match": map[string]any{"value": trimmedValue}})
	}
	if len(must) == 0 {
		return nil
	}
	return map[string]any{"must": must}
}

func tokenizeQuery(query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	parts := strings.FieldsFunc(query, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	return uniqueTrimmed(parts)
}

func keywordMatchScore(tokens []string, item ResearchResult) float64 {
	if len(tokens) == 0 {
		return 0
	}
	text := strings.ToLower(strings.Join([]string{item.Text, item.SourceQuote, item.Summary, item.SourceTitle, item.Filename, item.Path, item.SectionOrHeading}, " "))
	if strings.TrimSpace(text) == "" {
		return 0
	}
	matched := 0
	for _, token := range tokens {
		if strings.Contains(text, strings.ToLower(token)) {
			matched++
		}
	}
	if matched == 0 {
		return 0
	}
	return float64(matched) / float64(len(tokens))
}

func topDocuments(counts map[string]int, max int) []DocumentSizeBreakdown {
	if len(counts) == 0 {
		return nil
	}
	items := make([]DocumentSizeBreakdown, 0, len(counts))
	for key, value := range counts {
		items = append(items, DocumentSizeBreakdown{DocumentID: key, Chunks: value})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Chunks == items[j].Chunks {
			return items[i].DocumentID < items[j].DocumentID
		}
		return items[i].Chunks > items[j].Chunks
	})
	if max > 0 && len(items) > max {
		items = items[:max]
	}
	return items
}

func obviousDuplicates(seed map[string]map[string]struct{}) []DuplicateDocument {
	items := make([]DuplicateDocument, 0)
	for key, ids := range seed {
		if len(ids) <= 1 {
			continue
		}
		entry := DuplicateDocument{DocumentKey: key, DocumentIDs: mapKeysSorted(ids)}
		items = append(items, entry)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].DocumentKey < items[j].DocumentKey
	})
	return items
}

func missingProvenanceFields(fieldSet map[string]struct{}) []string {
	required := [][]string{
		{"source_document_id", "document_id", "doc_id", "source_id"},
		{"source_title", "title", "document_title"},
		{"filename", "file_name", "attachment_filename"},
		{"path", "file_path", "source_path"},
		{"chunk_id", "chunk_uid"},
		{"chunk_index", "chunk_idx", "position", "segment_index"},
		{"page", "page_number"},
		{"section", "heading", "header"},
		{"source_type", "document_type"},
		{"created_date", "created_at"},
		{"modified_date", "modified_at", "updated_at"},
	}
	labels := []string{"source/document_id", "title", "filename", "path/folder", "chunk_id", "chunk_index", "page", "section/heading", "source_type", "created_date", "modified_date"}
	missing := make([]string, 0)
	for idx, aliasGroup := range required {
		present := false
		for _, alias := range aliasGroup {
			if _, ok := fieldSet[alias]; ok {
				present = true
				break
			}
		}
		if !present {
			missing = append(missing, labels[idx])
		}
	}
	return missing
}

func mapKeysSorted[T any](input map[string]T) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func asString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func asInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0
		}
		return int(parsed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func normalizeDate(value any) string {
	ts := parseAnyTime(value)
	if ts.IsZero() {
		return strings.TrimSpace(asString(value))
	}
	return ts.Format(time.RFC3339)
}

func parseAnyTime(value any) time.Time {
	raw := strings.TrimSpace(asString(value))
	if raw == "" {
		return time.Time{}
	}
	formats := []string{time.RFC3339, "2006-01-02", "2006-01-02 15:04:05", time.RFC3339Nano}
	for _, format := range formats {
		if parsed, err := time.Parse(format, raw); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func mostCommonKey(values map[string]int) string {
	bestKey := ""
	bestValue := 0
	for key, count := range values {
		if count > bestValue || (count == bestValue && key < bestKey) {
			bestKey = key
			bestValue = count
		}
	}
	return bestKey
}

func toJSON(value any) string {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func uniqueTrimmed(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func uniqueSourceFingerprints(results []ResearchResult) []string {
	items := make([]string, 0, len(results))
	for _, result := range results {
		if trimmed := strings.TrimSpace(result.SourceFingerprint); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return uniqueTrimmed(items)
}

func matchesExcluded(item ResearchResult, excluded map[string]struct{}) bool {
	if len(excluded) == 0 {
		return false
	}
	keys := []string{item.SourceFingerprint, item.SourceDocumentID, item.Path, item.Filename, item.SourceTitle}
	for _, key := range keys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		if _, ok := excluded[trimmed]; ok {
			return true
		}
	}
	return false
}

func uniqueMerge(a, b []string) []string {
	merged := make([]string, 0, len(a)+len(b))
	merged = append(merged, a...)
	merged = append(merged, b...)
	return uniqueTrimmed(merged)
}

func cleanFilterMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		trimmedKey := strings.TrimSpace(key)
		trimmedValue := strings.TrimSpace(value)
		if trimmedKey == "" || trimmedValue == "" {
			continue
		}
		out[trimmedKey] = trimmedValue
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeDupKey(item ResearchResult) string {
	seed := firstNonEmpty(item.Path, item.Filename, item.SourceTitle)
	if seed == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(seed))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func mapMode(raw bool, yes, no string) string {
	if raw {
		return yes
	}
	return no
}
