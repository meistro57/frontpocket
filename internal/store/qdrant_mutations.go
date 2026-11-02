package store

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// SetMemoryPayloadFields merges fields into the payload of existing points.
// Vectors are untouched, so a correction keeps the original embedding until the
// point is re-ingested.
func (s *QdrantMemoryStore) SetMemoryPayloadFields(ctx context.Context, memoryIDs []string, fields map[string]any) error {
	if len(fields) == 0 {
		return fmt.Errorf("no payload fields supplied")
	}

	qdrantIDs := make([]string, 0, len(memoryIDs))
	for _, memoryID := range memoryIDs {
		trimmed := strings.TrimSpace(memoryID)
		if trimmed == "" {
			continue
		}
		qdrantIDs = append(qdrantIDs, qdrantPointID(trimmed))
	}
	if len(qdrantIDs) == 0 {
		return fmt.Errorf("at least one memory id is required")
	}

	body := map[string]any{
		"payload": fields,
		"points":  qdrantIDs,
	}
	if _, err := s.client.doJSON(ctx, http.MethodPost, fmt.Sprintf("/collections/%s/points/payload?wait=true", s.collection), body, nil); err != nil {
		return err
	}

	if fallback, ok := s.fallback.(interface {
		SetMemoryPayloadFields(context.Context, []string, map[string]any) error
	}); ok {
		_ = fallback.SetMemoryPayloadFields(ctx, memoryIDs, fields)
	}
	return nil
}

// DeleteMemoryPoints removes points for good. Soft deletion happens at the API
// layer by setting a status field, so recall can stop returning the memory
// without destroying the record.
func (s *QdrantMemoryStore) DeleteMemoryPoints(ctx context.Context, memoryIDs []string) error {
	return s.DeleteByPointIDs(ctx, memoryIDs)
}
