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

type Ingestor struct {
	Chunker      Chunker
	Embedder     embed.Embedder
	Store        MemoryStore
	SourceType   string
	SourceTitle  string
	Project      string
	MemoryKind   string
	SpeakerRules SpeakerRules
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

	points := make([]MemoryPoint, 0, len(records))
	for idx, rec := range records {
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

		for cidx, chunk := range chunks {
			point := MemoryPoint{
				MemoryID:            buildMemoryID(rec.ConversationID, idx, cidx),
				ConversationID:      defaultValue(rec.ConversationID, "unknown_conversation"),
				SourceType:          defaultValue(rec.SourceType, defaultValue(i.SourceType, "chat_export")),
				SourceTitle:         defaultValue(rec.SourceTitle, defaultValue(i.SourceTitle, "Untitled Source")),
				Timestamp:           ts,
				Speaker:             strings.ToLower(strings.TrimSpace(rec.Speaker)),
				Project:             defaultValue(rec.Project, i.Project),
				Tags:                append([]string(nil), rec.Tags...),
				MemoryKind:          defaultValue(rec.MemoryKind, defaultValue(i.MemoryKind, KindProjectContext)),
				Text:                chunk,
				SourceQuote:         clampQuote(chunk),
				Summary:             summarize(chunk),
				EmbeddingProvider:   i.Embedder.ProviderName(),
				EmbeddingModel:      i.Embedder.ModelName(),
				EmbeddingDimensions: len(embeddings[cidx]),
				Vector:              embeddings[cidx],
			}
			points = append(points, point)
		}
	}

	if len(points) == 0 {
		return nil, nil
	}

	if err := i.Store.Upsert(ctx, points); err != nil {
		return nil, err
	}

	return points, nil
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
