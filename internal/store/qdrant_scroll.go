package store

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
)

func (s *QdrantMemoryStore) ScrollRaw(ctx context.Context, limit int, offset string, filters memory.SearchFilters, since, until time.Time, includeCanonical bool) ([]memory.MemoryPoint, string, error) {
	if strings.TrimSpace(s.collection) == "" {
		return nil, "", fmt.Errorf("QDRANT_COLLECTION is required")
	}
	if limit <= 0 {
		limit = 128
	}

	body := map[string]any{
		"limit":        limit,
		"with_payload": true,
		"with_vector":  false,
	}
	if filter := toQdrantFilter(filters); filter != nil {
		body["filter"] = filter
	}
	if strings.TrimSpace(offset) != "" {
		if parsed, err := strconv.ParseInt(offset, 10, 64); err == nil {
			body["offset"] = parsed
		} else {
			body["offset"] = offset
		}
	}

	var response struct {
		Result struct {
			Points []struct {
				Payload map[string]any `json:"payload"`
			} `json:"points"`
			NextPageOffset any `json:"next_page_offset"`
		} `json:"result"`
	}

	status, err := s.client.doJSON(ctx, http.MethodPost, fmt.Sprintf("/collections/%s/points/scroll", s.collection), body, &response)
	if err != nil {
		if status == http.StatusNotFound {
			return []memory.MemoryPoint{}, "", nil
		}
		return nil, "", err
	}

	points := make([]memory.MemoryPoint, 0, len(response.Result.Points))
	for _, raw := range response.Result.Points {
		point := memoryPointFromPayload(raw.Payload)
		if !includeCanonical && point.Canonical {
			continue
		}
		if !since.IsZero() && point.Timestamp.Before(since) {
			continue
		}
		if !until.IsZero() && point.Timestamp.After(until) {
			continue
		}
		points = append(points, point)
	}

	next := ""
	if response.Result.NextPageOffset != nil {
		next = strings.TrimSpace(fmt.Sprintf("%v", response.Result.NextPageOffset))
	}
	return points, next, nil
}

func memoryPointFromPayload(payload map[string]any) memory.MemoryPoint {
	return memory.MemoryPoint{
		MemoryID:               asString(payload["memory_id"]),
		ConversationID:         asString(payload["conversation_id"]),
		SessionID:              asString(payload["session_id"]),
		SourceType:             asString(payload["source_type"]),
		SourceTitle:            asString(payload["source_title"]),
		Timestamp:              asTime(payload["timestamp"]),
		Speaker:                asString(payload["speaker"]),
		Project:                asString(payload["project"]),
		Tags:                   asStringSlice(payload["tags"]),
		UserStarred:            asBool(payload["user_starred"]),
		UserShared:             asBool(payload["user_shared"]),
		ShareID:                asString(payload["share_id"]),
		FeedbackRating:         asString(payload["feedback_rating"]),
		FeedbackNote:           asString(payload["feedback_note"]),
		FeedbackAt:             asString(payload["feedback_at"]),
		AttachmentFilename:     asString(payload["attachment_filename"]),
		AttachmentMimeType:     asString(payload["attachment_mime_type"]),
		AttachmentCategory:     asString(payload["attachment_category"]),
		AttachmentSourceSystem: asString(payload["attachment_source_system"]),
		MemoryKind:             asString(payload["memory_kind"]),
		Importance:             asFloat(payload["importance"]),
		Text:                   asString(payload["text"]),
		SourceQuote:            asString(payload["source_quote"]),
		Summary:                asString(payload["summary"]),
		UsedMemoryIDs:          asStringSlice(payload["used_memory_ids"]),
		EmbeddingProvider:      asString(payload["embedding_provider"]),
		EmbeddingModel:         asString(payload["embedding_model"]),
		EmbeddingDimensions:    asInt(payload["embedding_dimensions"]),
		Canonical:              asBool(payload["canonical"]),
		Confidence:             asString(payload["confidence"]),
		Status:                 asString(payload["status"]),
		SourceMemoryIDs:        asStringSlice(payload["source_memory_ids"]),
		SourceQuotes:           asStringSlice(payload["source_quotes"]),
		ReviewedAt:             asTimePtr(payload["reviewed_at"]),
		ReviewedBy:             asString(payload["reviewed_by"]),
		CreatedByLoop:          asBool(payload["created_by_loop"]),
		Supersedes:             asStringSlice(payload["supersedes"]),
		MergedFrom:             asStringSlice(payload["merged_from"]),
		ApproximateDate:        asString(payload["approximate_date"]),
		DateBasis:              asString(payload["date_basis"]),
		RejectionReason:        asString(payload["rejection_reason"]),
		MergeTargetID:          asString(payload["merge_target_id"]),
		AIProvider:             asString(payload["ai_provider"]),
		AIModel:                asString(payload["ai_model"]),
	}
}

func asFloat(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	default:
		return 0
	}
}
