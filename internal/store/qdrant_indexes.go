package store

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// PayloadIndexSpec describes a Qdrant payload index that filtered reads depend on.
type PayloadIndexSpec struct {
	Field  string
	Schema string
}

// memoryPayloadIndexes covers every payload field used by search, browse, and
// stats filters. Unindexed filters force Qdrant to scan, which degrades as the
// corpus grows.
var memoryPayloadIndexes = []PayloadIndexSpec{
	{Field: "project", Schema: "keyword"},
	{Field: "memory_kind", Schema: "keyword"},
	{Field: "speaker", Schema: "keyword"},
	{Field: "source_type", Schema: "keyword"},
	{Field: "conversation_id", Schema: "keyword"},
	{Field: "ai_provider", Schema: "keyword"},
	{Field: "ai_model", Schema: "keyword"},
	{Field: "feedback_rating", Schema: "keyword"},
	{Field: "attachment_source_system", Schema: "keyword"},
	{Field: "tags", Schema: "keyword"},
	{Field: "status", Schema: "keyword"},
	{Field: "user_starred", Schema: "bool"},
	{Field: "user_shared", Schema: "bool"},
	{Field: "canonical", Schema: "bool"},
	{Field: "timestamp", Schema: "datetime"},
}

// MemoryPayloadIndexes returns the payload indexes FrontPocket expects on a
// memory collection.
func MemoryPayloadIndexes() []PayloadIndexSpec {
	out := make([]PayloadIndexSpec, len(memoryPayloadIndexes))
	copy(out, memoryPayloadIndexes)
	return out
}

// CreatePayloadIndex creates (or confirms) a single payload index. Qdrant treats
// re-creating an existing index as a no-op, so this is safe to call repeatedly.
func (c *QdrantClient) CreatePayloadIndex(ctx context.Context, collection, field, schema string) error {
	trimmedCollection := strings.TrimSpace(collection)
	if trimmedCollection == "" {
		return fmt.Errorf("collection is required")
	}
	trimmedField := strings.TrimSpace(field)
	if trimmedField == "" {
		return fmt.Errorf("index field is required")
	}
	trimmedSchema := strings.TrimSpace(schema)
	if trimmedSchema == "" {
		return fmt.Errorf("index schema is required")
	}

	body := map[string]any{
		"field_name":   trimmedField,
		"field_schema": trimmedSchema,
	}
	path := fmt.Sprintf("/collections/%s/index", trimmedCollection)
	if _, err := c.doJSON(ctx, http.MethodPut, path, body, nil); err != nil {
		return fmt.Errorf("could not create index on %q: %w", trimmedField, err)
	}
	return nil
}

// EnsurePayloadIndexes creates every supplied index, returning the first error.
func (c *QdrantClient) EnsurePayloadIndexes(ctx context.Context, collection string, specs []PayloadIndexSpec) error {
	for _, spec := range specs {
		if err := c.CreatePayloadIndex(ctx, collection, spec.Field, spec.Schema); err != nil {
			return err
		}
	}
	return nil
}
