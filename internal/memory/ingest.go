package memory

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/meistro57/frontpocket/internal/embed"
)

func ParseJSONL(input string) ([]MessageRecord, error) {
	scanner := bufio.NewScanner(strings.NewReader(input))
	records := make([]MessageRecord, 0)
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var rec MessageRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("invalid JSONL at line %d: %w", lineNumber, err)
		}
		records = append(records, rec)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return records, nil
}

const DefaultUpsertBatchSize = 128

type MemoryIDExistser interface {
	MemoryIDsExist(ctx context.Context, ids []string) (map[string]bool, error)
}

type ConversationLegacyChecker interface {
	ConversationHasLegacy(ctx context.Context, conversationID string) (bool, error)
}

type Ingestor struct {
	Chunker      Chunker
	Embedder     embed.Embedder
	Store        MemoryStore
	SourceType   string
	SourceTitle  string
	Project      string
	MemoryKind   string
	AIProvider   string
	AIModel      string
	SpeakerRules SpeakerRules
	ProgressFn   func(ProgressEvent)
	BatchSize    int
	Journal      ResumeJournal
	LegacyMode   string
}

type Checkpoint struct {
	LastRecordIndex int `json:"last_record_index"`
	TotalRecords    int `json:"total_records"`
	ChunksEmbedded  int `json:"chunks_embedded"`
}

type ResumeJournal interface {
	LastRecordIndex() int
	Commit(cp Checkpoint) error
}

type ProgressEvent struct {
	RecordsProcessed int
	RecordsTotal     int
	ChunksEmbedded   int
	Elapsed          time.Duration
}

type SpeakerRules struct {
	StoreAssistant bool
	StoreUser      bool
	StoreSystem    bool
}

type IDMapEntry struct {
	OldMemoryID string
	NewMemoryID string
}

type LegacyRewriteStore interface {
	ListConversationPointIDs(ctx context.Context, conversationID string) ([]string, error)
	DeleteByPointIDs(ctx context.Context, pointIDs []string) error
}

type IngestResult struct {
	Points                     []MemoryPoint
	RecordsSkippedExisting     int
	ChunksSkippedExisting      int
	ConversationsLegacySkipped int
	RewriteMappings            []IDMapEntry
	LegacyPointsDeleted        int
}

func (i Ingestor) Ingest(ctx context.Context, records []MessageRecord) (IngestResult, error) {
	if i.Store == nil {
		return IngestResult{}, errors.New("memory store is required")
	}
	if i.Embedder == nil {
		return IngestResult{}, errors.New("embedder is required")
	}
	if len(records) == 0 {
		return IngestResult{}, nil
	}

	batchSize := i.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultUpsertBatchSize
	}

	inserted := make([]MemoryPoint, 0, len(records))
	batch := make([]MemoryPoint, 0, batchSize)

	resumeFrom := -1
	if i.Journal != nil {
		resumeFrom = i.Journal.LastRecordIndex()
	}

	legacyMode := strings.ToLower(strings.TrimSpace(i.LegacyMode))
	if legacyMode == "" {
		legacyMode = "skip"
	}

	legacyChecker, hasLegacyChecker := i.Store.(ConversationLegacyChecker)
	existingChecker, hasExistingChecker := i.Store.(MemoryIDExistser)
	rewriteStore, hasRewriteStore := i.Store.(LegacyRewriteStore)
	legacyCache := map[string]bool{}

	start := time.Now()
	chunksEmbedded := 0
	recordsSkippedExisting := 0
	chunksSkippedExisting := 0
	conversationsLegacySkipped := 0
	rewriteMappings := make([]IDMapEntry, 0)
	legacyPointsDeleted := 0

	flush := func(throughRecord int) error {
		if len(batch) == 0 {
			return nil
		}
		if err := i.Store.Upsert(ctx, batch); err != nil {
			return err
		}
		for j := range batch {
			batch[j].Vector = nil
			inserted = append(inserted, batch[j])
		}
		batch = batch[:0]
		if i.Journal != nil {
			if err := i.Journal.Commit(Checkpoint{
				LastRecordIndex: throughRecord,
				TotalRecords:    len(records),
				ChunksEmbedded:  chunksEmbedded,
			}); err != nil {
				return err
			}
		}
		return nil
	}

	rewritePlans := map[string][]rewriteRecord{}

	for idx, rec := range records {
		if idx <= resumeFrom {
			continue
		}
		if !i.shouldStoreSpeaker(rec.Speaker) {
			continue
		}
		if strings.TrimSpace(rec.Text) == "" {
			continue
		}

		conversationID := defaultValue(rec.ConversationID, "unknown_conversation")
		// The legacy guard only applies to records that carry a stable source
		// message id (archive re-imports). Records without one (API/JSONL) emit
		// legacy-style ids themselves — guarding them would skip a conversation
		// the moment our own first point lands. Dedupe for them stays purely
		// existence-check based.
		hasStableID := strings.TrimSpace(rec.MessageID) != ""

		legacy := false
		if hasLegacyChecker && hasStableID {
			cached, ok := legacyCache[conversationID]
			if !ok {
				found, err := legacyChecker.ConversationHasLegacy(ctx, conversationID)
				if err != nil {
					return IngestResult{}, err
				}
				cached = found
				legacyCache[conversationID] = cached
			}
			legacy = cached
		}

		if legacyMode == "skip" && legacy {
			conversationsLegacySkipped++
			continue
		}

		chunks := i.Chunker.ChunkText(rec.Text)
		if len(chunks) == 0 {
			continue
		}

		ts, err := parseTimestamp(rec.Timestamp)
		if err != nil {
			return IngestResult{}, fmt.Errorf("record %d: %w", idx, err)
		}

		ids := make([]string, 0, len(chunks))
		for cidx := range chunks {
			ids = append(ids, buildMemoryID(rec.ConversationID, rec.MessageID, idx, cidx))
		}

		if legacyMode == "rewrite" && hasStableID {
			if legacy {
				rewritePlans[conversationID] = append(rewritePlans[conversationID], rewriteRecord{
					record:       rec,
					recordIndex:  idx,
					chunks:       chunks,
					ids:          ids,
					timestamp:    ts,
					conversation: conversationID,
				})
				continue
			}
			for _, id := range ids {
				rewriteMappings = append(rewriteMappings, IDMapEntry{OldMemoryID: "", NewMemoryID: id})
			}
		}

		existing := map[string]bool{}
		if hasExistingChecker {
			existing, err = existingChecker.MemoryIDsExist(ctx, ids)
			if err != nil {
				return IngestResult{}, err
			}
		}

		missingChunkIdx := make([]int, 0, len(chunks))
		for cidx, id := range ids {
			if existing[id] {
				chunksSkippedExisting++
				continue
			}
			missingChunkIdx = append(missingChunkIdx, cidx)
		}
		if len(missingChunkIdx) == 0 {
			recordsSkippedExisting++
			continue
		}

		chunkTexts := make([]string, 0, len(missingChunkIdx))
		for _, cidx := range missingChunkIdx {
			chunkTexts = append(chunkTexts, chunks[cidx])
		}

		embeddings, err := i.Embedder.EmbedBatch(ctx, chunkTexts)
		if err != nil {
			return IngestResult{}, fmt.Errorf("record %d: embedding failed: %w", idx, err)
		}
		if len(embeddings) != len(chunkTexts) {
			return IngestResult{}, fmt.Errorf("record %d: embedding count mismatch", idx)
		}
		chunksEmbedded += len(chunkTexts)

		if i.ProgressFn != nil && (idx+1)%100 == 0 {
			i.ProgressFn(ProgressEvent{
				RecordsProcessed: idx + 1,
				RecordsTotal:     len(records),
				ChunksEmbedded:   chunksEmbedded,
				Elapsed:          time.Since(start),
			})
		}

		for embIdx, cidx := range missingChunkIdx {
			chunk := chunks[cidx]
			point := MemoryPoint{
				MemoryID:               ids[cidx],
				ConversationID:         conversationID,
				MessageID:              strings.TrimSpace(rec.MessageID),
				SourceType:             defaultValue(rec.SourceType, defaultValue(i.SourceType, "chat_export")),
				SourceTitle:            defaultValue(rec.SourceTitle, defaultValue(i.SourceTitle, "Untitled Source")),
				Timestamp:              ts,
				Speaker:                strings.ToLower(strings.TrimSpace(rec.Speaker)),
				Project:                defaultValue(rec.Project, i.Project),
				Tags:                   append([]string(nil), rec.Tags...),
				UserStarred:            rec.UserStarred,
				UserShared:             rec.UserShared,
				ShareID:                rec.ShareID,
				FeedbackRating:         rec.FeedbackRating,
				FeedbackNote:           rec.FeedbackNote,
				FeedbackAt:             rec.FeedbackAt,
				AttachmentFilename:     rec.AttachmentFilename,
				AttachmentMimeType:     rec.AttachmentMimeType,
				AttachmentCategory:     rec.AttachmentCategory,
				AttachmentSourceSystem: rec.AttachmentSourceSystem,
				MemoryKind:             defaultValue(rec.MemoryKind, defaultValue(i.MemoryKind, KindProjectContext)),
				Text:                   chunk,
				AIProvider:             defaultValue(rec.AIProvider, i.AIProvider),
				AIModel:                defaultValue(rec.AIModel, i.AIModel),
				SourceQuote:            clampQuote(chunk),
				Summary:                summarize(chunk),
				EmbeddingProvider:      i.Embedder.ProviderName(),
				EmbeddingModel:         i.Embedder.ModelName(),
				EmbeddingDimensions:    len(embeddings[embIdx]),
				Vector:                 embeddings[embIdx],
			}
			batch = append(batch, point)
		}

		if len(batch) >= batchSize {
			if err := flush(idx); err != nil {
				return IngestResult{}, err
			}
		}
	}

	if legacyMode == "rewrite" {
		if !hasRewriteStore {
			return IngestResult{}, fmt.Errorf("legacy rewrite mode requires store support")
		}
		for conversationID, planned := range rewritePlans {
			rewriteInserted, mappings, deletedLegacyCount, skippedChunks, skippedRecords, err := i.rewriteConversation(ctx, rewriteStore, existingChecker, hasExistingChecker, planned)
			if err != nil {
				return IngestResult{}, err
			}
			for idx := range rewriteInserted {
				rewriteInserted[idx].Vector = nil
				inserted = append(inserted, rewriteInserted[idx])
			}
			rewriteMappings = append(rewriteMappings, mappings...)
			chunksEmbedded += len(rewriteInserted)
			chunksSkippedExisting += skippedChunks
			recordsSkippedExisting += skippedRecords
			legacyPointsDeleted += deletedLegacyCount
			legacyCache[conversationID] = false
		}
	}

	if err := flush(len(records) - 1); err != nil {
		return IngestResult{}, err
	}

	if i.ProgressFn != nil {
		i.ProgressFn(ProgressEvent{
			RecordsProcessed: len(records),
			RecordsTotal:     len(records),
			ChunksEmbedded:   chunksEmbedded,
			Elapsed:          time.Since(start),
		})
	}

	return IngestResult{
		Points:                     inserted,
		RecordsSkippedExisting:     recordsSkippedExisting,
		ChunksSkippedExisting:      chunksSkippedExisting,
		ConversationsLegacySkipped: conversationsLegacySkipped,
		RewriteMappings:            rewriteMappings,
		LegacyPointsDeleted:        legacyPointsDeleted,
	}, nil
}

type rewriteRecord struct {
	record       MessageRecord
	recordIndex  int
	chunks       []string
	ids          []string
	timestamp    time.Time
	conversation string
}

func (i Ingestor) rewriteConversation(ctx context.Context, rewriteStore LegacyRewriteStore, existingChecker MemoryIDExistser, hasExistingChecker bool, planned []rewriteRecord) ([]MemoryPoint, []IDMapEntry, int, int, int, error) {
	if len(planned) == 0 {
		return []MemoryPoint{}, nil, 0, 0, 0, nil
	}

	insertBatch := make([]MemoryPoint, 0)
	skippedChunkCount := 0
	skippedRecordCount := 0
	mappings := make([]IDMapEntry, 0)

	for _, item := range planned {
		existing := map[string]bool{}
		var err error
		if hasExistingChecker {
			existing, err = existingChecker.MemoryIDsExist(ctx, item.ids)
			if err != nil {
				return nil, nil, 0, 0, 0, err
			}
		}

		missingChunkIdx := make([]int, 0, len(item.chunks))
		for cidx, id := range item.ids {
			if existing[id] {
				skippedChunkCount++
				continue
			}
			missingChunkIdx = append(missingChunkIdx, cidx)
		}
		if len(missingChunkIdx) == 0 {
			skippedRecordCount++
		}

		if len(missingChunkIdx) > 0 {
			chunkTexts := make([]string, 0, len(missingChunkIdx))
			for _, cidx := range missingChunkIdx {
				chunkTexts = append(chunkTexts, item.chunks[cidx])
			}

			embeddings, err := i.Embedder.EmbedBatch(ctx, chunkTexts)
			if err != nil {
				return nil, nil, 0, 0, 0, fmt.Errorf("record %d: embedding failed: %w", item.recordIndex, err)
			}
			if len(embeddings) != len(chunkTexts) {
				return nil, nil, 0, 0, 0, fmt.Errorf("record %d: embedding count mismatch", item.recordIndex)
			}

			for embIdx, cidx := range missingChunkIdx {
				chunk := item.chunks[cidx]
				point := MemoryPoint{
					MemoryID:               item.ids[cidx],
					ConversationID:         item.conversation,
					MessageID:              strings.TrimSpace(item.record.MessageID),
					SourceType:             defaultValue(item.record.SourceType, defaultValue(i.SourceType, "chat_export")),
					SourceTitle:            defaultValue(item.record.SourceTitle, defaultValue(i.SourceTitle, "Untitled Source")),
					Timestamp:              item.timestamp,
					Speaker:                strings.ToLower(strings.TrimSpace(item.record.Speaker)),
					Project:                defaultValue(item.record.Project, i.Project),
					Tags:                   append([]string(nil), item.record.Tags...),
					UserStarred:            item.record.UserStarred,
					UserShared:             item.record.UserShared,
					ShareID:                item.record.ShareID,
					FeedbackRating:         item.record.FeedbackRating,
					FeedbackNote:           item.record.FeedbackNote,
					FeedbackAt:             item.record.FeedbackAt,
					AttachmentFilename:     item.record.AttachmentFilename,
					AttachmentMimeType:     item.record.AttachmentMimeType,
					AttachmentCategory:     item.record.AttachmentCategory,
					AttachmentSourceSystem: item.record.AttachmentSourceSystem,
					MemoryKind:             defaultValue(item.record.MemoryKind, defaultValue(i.MemoryKind, KindProjectContext)),
					Text:                   chunk,
					AIProvider:             defaultValue(item.record.AIProvider, i.AIProvider),
					AIModel:                defaultValue(item.record.AIModel, i.AIModel),
					SourceQuote:            clampQuote(chunk),
					Summary:                summarize(chunk),
					EmbeddingProvider:      i.Embedder.ProviderName(),
					EmbeddingModel:         i.Embedder.ModelName(),
					EmbeddingDimensions:    len(embeddings[embIdx]),
					Vector:                 embeddings[embIdx],
				}
				insertBatch = append(insertBatch, point)
			}
		}
	}

	if len(insertBatch) > 0 {
		if err := i.Store.Upsert(ctx, insertBatch); err != nil {
			return nil, nil, 0, 0, 0, err
		}
	}

	conversationID := planned[0].conversation
	legacyPointIDs, err := rewriteStore.ListConversationPointIDs(ctx, conversationID)
	if err != nil {
		return nil, nil, 0, 0, 0, err
	}

	legacyByOrder := make([]string, 0)
	for _, pointID := range legacyPointIDs {
		if strings.Contains(pointID, "_msg_") {
			continue
		}
		legacyByOrder = append(legacyByOrder, pointID)
	}

	newByOrder := make([]string, 0)
	for _, item := range planned {
		newByOrder = append(newByOrder, item.ids...)
	}

	for idx := 0; idx < len(legacyByOrder) && idx < len(newByOrder); idx++ {
		mappings = append(mappings, IDMapEntry{OldMemoryID: legacyByOrder[idx], NewMemoryID: newByOrder[idx]})
	}

	if len(legacyByOrder) > 0 {
		if err := rewriteStore.DeleteByPointIDs(ctx, legacyByOrder); err != nil {
			return nil, nil, 0, 0, 0, err
		}
	}

	return insertBatch, mappings, len(legacyByOrder), skippedChunkCount, skippedRecordCount, nil
}

func (i Ingestor) shouldStoreSpeaker(speaker string) bool {
	s := strings.ToLower(strings.TrimSpace(speaker))
	switch s {
	case "assistant":
		return i.SpeakerRules.StoreAssistant
	case "system":
		return i.SpeakerRules.StoreSystem
	default:
		return i.SpeakerRules.StoreUser
	}
}

func parseTimestamp(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Now().UTC(), nil
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timestamp %q: %w", raw, err)
	}
	return parsed, nil
}

func buildMemoryID(conversationID, messageID string, recordIndex, chunkIndex int) string {
	cid := defaultValue(strings.TrimSpace(conversationID), "chat")
	cid = strings.ReplaceAll(cid, " ", "_")
	mid := strings.TrimSpace(messageID)
	if mid != "" {
		mid = strings.ReplaceAll(mid, " ", "_")
		return fmt.Sprintf("%s_msg_%s_chunk_%03d", cid, mid, chunkIndex+1)
	}
	return fmt.Sprintf("%s_turn_%03d_chunk_%03d", cid, recordIndex+1, chunkIndex+1)
}

func defaultValue(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
}

func summarize(text string) string {
	r := []rune(strings.TrimSpace(text))
	if len(r) <= 220 {
		return string(r)
	}
	return string(r[:220]) + "..."
}

func clampQuote(text string) string {
	r := []rune(strings.TrimSpace(text))
	if len(r) <= 180 {
		return string(r)
	}
	return string(r[:180]) + "..."
}
