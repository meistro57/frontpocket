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

// DefaultUpsertBatchSize is the number of memory points flushed to the store at
// once when an Ingestor does not specify its own BatchSize. It bounds how many
// embedding vectors are held in memory during a large import.
const DefaultUpsertBatchSize = 128

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
	// BatchSize controls how many points are upserted per store write. Points
	// are flushed as soon as the buffer reaches this size so the whole import
	// is never resident in memory at once. Zero uses DefaultUpsertBatchSize.
	BatchSize int
	// Journal, when set, records ingest progress after every successful batch
	// flush so an interrupted import can be resumed. Records already covered by
	// the journal are skipped on a subsequent run. Optional.
	Journal ResumeJournal
}

// Checkpoint captures how far an ingest has progressed. It is persisted by a
// ResumeJournal after each batch is durably stored.
type Checkpoint struct {
	LastRecordIndex int `json:"last_record_index"`
	TotalRecords    int `json:"total_records"`
	ChunksEmbedded  int `json:"chunks_embedded"`
}

// ResumeJournal persists ingest progress so an interrupted run can continue.
// Implementations must only report a record as committed once its points have
// been durably stored.
type ResumeJournal interface {
	// LastRecordIndex returns the index of the last fully-ingested record, or
	// -1 when nothing has been committed yet.
	LastRecordIndex() int
	// Commit durably records that every record up to and including
	// cp.LastRecordIndex has been stored.
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

func (i Ingestor) Ingest(ctx context.Context, records []MessageRecord) ([]MemoryPoint, error) {
	if i.Store == nil {
		return nil, errors.New("memory store is required")
	}
	if i.Embedder == nil {
		return nil, errors.New("embedder is required")
	}
	if len(records) == 0 {
		return nil, nil
	}

	batchSize := i.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultUpsertBatchSize
	}

	// inserted retains the lightweight metadata (IDs) callers report on, while
	// batch holds the in-flight vectors that get flushed and discarded.
	inserted := make([]MemoryPoint, 0, len(records))
	batch := make([]MemoryPoint, 0, batchSize)

	resumeFrom := -1
	if i.Journal != nil {
		resumeFrom = i.Journal.LastRecordIndex()
	}

	start := time.Now()
	chunksEmbedded := 0

	// flush writes the buffered batch to the store, frees its vectors, and
	// records progress through throughRecord (the index of the last record
	// whose points are now fully stored). Flushes happen on record boundaries
	// so the checkpoint always reflects whole records.
	flush := func(throughRecord int) error {
		if len(batch) == 0 {
			return nil
		}
		if err := i.Store.Upsert(ctx, batch); err != nil {
			return err
		}
		// Keep the metadata for the return value but drop the vectors so the
		// embeddings for already-stored points don't accumulate in memory.
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

		chunks := i.Chunker.ChunkText(rec.Text)
		if len(chunks) == 0 {
			continue
		}

		ts, err := parseTimestamp(rec.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("record %d: %w", idx, err)
		}

		embeddings, err := i.Embedder.EmbedBatch(ctx, chunks)
		if err != nil {
			return nil, fmt.Errorf("record %d: embedding failed: %w", idx, err)
		}
		if len(embeddings) != len(chunks) {
			return nil, fmt.Errorf("record %d: embedding count mismatch", idx)
		}
		chunksEmbedded += len(chunks)

		if i.ProgressFn != nil && (idx+1)%100 == 0 {
			i.ProgressFn(ProgressEvent{
				RecordsProcessed: idx + 1,
				RecordsTotal:     len(records),
				ChunksEmbedded:   chunksEmbedded,
				Elapsed:          time.Since(start),
			})
		}

		for cidx, chunk := range chunks {
			point := MemoryPoint{
				MemoryID:               buildMemoryID(rec.ConversationID, idx, cidx),
				ConversationID:         defaultValue(rec.ConversationID, "unknown_conversation"),
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
				EmbeddingDimensions:    len(embeddings[cidx]),
				Vector:                 embeddings[cidx],
			}
			batch = append(batch, point)
		}

		// Flush on a record boundary so the journal checkpoint never lands in
		// the middle of a record's chunks.
		if len(batch) >= batchSize {
			if err := flush(idx); err != nil {
				return nil, err
			}
		}
	}

	if err := flush(len(records) - 1); err != nil {
		return nil, err
	}

	if len(inserted) == 0 {
		return nil, nil
	}

	if i.ProgressFn != nil {
		i.ProgressFn(ProgressEvent{
			RecordsProcessed: len(records),
			RecordsTotal:     len(records),
			ChunksEmbedded:   chunksEmbedded,
			Elapsed:          time.Since(start),
		})
	}

	return inserted, nil
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

func buildMemoryID(conversationID string, recordIndex, chunkIndex int) string {
	cid := defaultValue(strings.TrimSpace(conversationID), "chat")
	cid = strings.ReplaceAll(cid, " ", "_")
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
