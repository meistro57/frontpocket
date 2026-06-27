package store

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/meistro57/frontpocket/internal/embed"
	"github.com/meistro57/frontpocket/internal/memory"
)

type QdrantClient struct {
	baseURL        string
	http           *http.Client
	logger         *slog.Logger
	requestTimeout time.Duration
}

func NewQdrantClient(url string) *QdrantClient {
	return &QdrantClient{
		baseURL: strings.TrimRight(url, "/"),
		http: &http.Client{},
		logger: slog.Default(),
		requestTimeout: 30 * time.Second,
	}
}

func (c *QdrantClient) Health(ctx context.Context) error {
	if c == nil || c.baseURL == "" {
		return fmt.Errorf("qdrant client is not configured")
	}

	_, err := c.doJSON(ctx, http.MethodGet, "/collections", nil, nil)
	return err
}

func (c *QdrantClient) doJSON(ctx context.Context, method, path string, payload any, out any) (int, error) {
	if c == nil || c.baseURL == "" {
		return 0, fmt.Errorf("qdrant client is not configured")
	}

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return 0, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	callCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline && c.requestTimeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, c.requestTimeout)
		defer cancel()
		req = req.WithContext(callCtx)
	}

	started := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		if isTimeoutError(err) {
			if c.logger != nil {
				c.logger.Warn("qdrant request timed out", "method", method, "path", path, "timeout", c.requestTimeout.String(), "elapsed_ms", time.Since(started).Milliseconds())
			}
			return 0, fmt.Errorf("qdrant request timed out for %s %s: %w", method, path, err)
		}
		if c.logger != nil {
			c.logger.Warn("qdrant request failed", "method", method, "path", path, "error", err.Error(), "elapsed_ms", time.Since(started).Milliseconds())
		}
		return 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		message := strings.TrimSpace(string(respBody))
		if c.logger != nil {
			c.logger.Warn("qdrant responded with error status", "method", method, "path", path, "status", resp.StatusCode, "body", message, "elapsed_ms", time.Since(started).Milliseconds())
		}
		return resp.StatusCode, fmt.Errorf("qdrant responded with status %d: %s", resp.StatusCode, message)
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return resp.StatusCode, err
		}
	}

	return resp.StatusCode, nil
}

type DimensionMismatchError struct {
	CollectionSize int
	EmbeddingSize  int
}

func (e *DimensionMismatchError) Error() string {
	return fmt.Sprintf("Qdrant collection vector size is %d, but current embedding model returned %d dimensions. Use a different collection or recreate the collection.", e.CollectionSize, e.EmbeddingSize)
}

type QdrantMemoryStore struct {
	client     *QdrantClient
	embedder   embed.Embedder
	collection string
	vectorName string
	distance   string
	fallback   memory.MemoryStore
}

func NewQdrantMemoryStore(client *QdrantClient, embedder embed.Embedder, collection, vectorName, distance string, fallback memory.MemoryStore) *QdrantMemoryStore {
	dist := strings.TrimSpace(distance)
	if dist == "" {
		dist = "Cosine"
	}
	return &QdrantMemoryStore{
		client:     client,
		embedder:   embedder,
		collection: strings.TrimSpace(collection),
		vectorName: strings.TrimSpace(vectorName),
		distance:   dist,
		fallback:   fallback,
	}
}

func (s *QdrantMemoryStore) Upsert(ctx context.Context, points []memory.MemoryPoint) error {
	if len(points) == 0 {
		return nil
	}

	if err := s.populateVectors(ctx, points); err != nil {
		if s.fallback != nil {
			return s.fallback.Upsert(ctx, points)
		}
		return err
	}

	dims := inferEmbeddingDimensions(points)
	if err := s.ensureCollection(ctx, dims); err != nil {
		if _, ok := err.(*DimensionMismatchError); ok {
			return err
		}
		if s.fallback != nil {
			return s.fallback.Upsert(ctx, points)
		}
		return err
	}

	type qdrantPoint struct {
		ID      string         `json:"id"`
		Vector  any            `json:"vector"`
		Payload map[string]any `json:"payload"`
	}

	payload := struct {
		Points []qdrantPoint `json:"points"`
	}{
		Points: make([]qdrantPoint, 0, len(points)),
	}

	for _, point := range points {
		vector := any(point.Vector)
		if s.vectorName != "" {
			vector = map[string][]float32{s.vectorName: point.Vector}
		}

		payload.Points = append(payload.Points, qdrantPoint{
			ID:      qdrantPointID(point.MemoryID),
			Vector:  vector,
			Payload: toQdrantPayload(point),
		})
	}

	_, err := s.client.doJSON(ctx, http.MethodPut, fmt.Sprintf("/collections/%s/points?wait=true", s.collection), payload, nil)
	if err != nil {
		if s.fallback != nil {
			if fallbackErr := s.fallback.Upsert(ctx, points); fallbackErr == nil {
				return nil
			}
		}
		return err
	}

	if s.fallback != nil {
		_ = s.fallback.Upsert(ctx, points)
	}
	return nil
}

func (s *QdrantMemoryStore) populateVectors(ctx context.Context, points []memory.MemoryPoint) error {
	for i := range points {
		if len(points[i].Vector) > 0 {
			if points[i].EmbeddingDimensions <= 0 {
				points[i].EmbeddingDimensions = len(points[i].Vector)
			}
			continue
		}
		if s.embedder == nil {
			return fmt.Errorf("memory point %q has no vector and embedder is not configured", strings.TrimSpace(points[i].MemoryID))
		}

		text := strings.TrimSpace(points[i].Text)
		if text == "" {
			text = strings.TrimSpace(points[i].Summary)
		}
		if text == "" {
			text = strings.TrimSpace(points[i].SourceQuote)
		}
		if text == "" {
			return fmt.Errorf("memory point %q has no vectorizable text", strings.TrimSpace(points[i].MemoryID))
		}

		vector, err := s.embedder.EmbedText(ctx, text)
		if err != nil {
			return err
		}
		points[i].Vector = vector
		points[i].EmbeddingDimensions = len(vector)
	}
	return nil
}

func inferEmbeddingDimensions(points []memory.MemoryPoint) int {
	for _, point := range points {
		if point.EmbeddingDimensions > 0 {
			return point.EmbeddingDimensions
		}
		if len(point.Vector) > 0 {
			return len(point.Vector)
		}
	}
	return 0
}

func (s *QdrantMemoryStore) Search(ctx context.Context, req memory.SearchRequest) ([]memory.SearchResult, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return []memory.SearchResult{}, nil
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 5
	}

	vector, err := s.embedder.EmbedText(ctx, query)
	if err != nil {
		if s.fallback != nil {
			return s.fallback.Search(ctx, req)
		}
		return nil, err
	}
	if err := s.ensureCollection(ctx, len(vector)); err != nil {
		if _, ok := err.(*DimensionMismatchError); ok {
			return nil, err
		}
		if s.fallback != nil {
			return s.fallback.Search(ctx, req)
		}
		return nil, err
	}

	searchVector := any(vector)
	if s.vectorName != "" {
		searchVector = map[string][]float32{s.vectorName: vector}
	}

	body := map[string]any{
		"vector":       searchVector,
		"limit":        limit,
		"with_payload": true,
		"with_vector":  false,
	}
	if filter := toQdrantFilter(req.Filters); filter != nil {
		body["filter"] = filter
	}

	var response struct {
		Result []struct {
			ID      any            `json:"id"`
			Score   float64        `json:"score"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	_, err = s.client.doJSON(ctx, http.MethodPost, fmt.Sprintf("/collections/%s/points/search", s.collection), body, &response)
	if err != nil {
		if s.fallback != nil {
			return s.fallback.Search(ctx, req)
		}
		return nil, err
	}

	results := make([]memory.SearchResult, 0, len(response.Result))
	for _, point := range response.Result {
		item := fromQdrantPayload(point.Payload, point.Score)
		if shouldExcludeByStatus(req.IncludeRejected, item.Status) {
			continue
		}
		item.Score = applyCanonicalBoost(item.Score, item.Canonical, item.Status)
		results = append(results, item)
	}
	if len(results) == 0 && s.fallback != nil {
		return s.fallback.Search(ctx, req)
	}
	return results, nil
}

func (s *QdrantMemoryStore) Stats(ctx context.Context, filters memory.SearchFilters) (memory.MemoryStats, error) {
	if strings.TrimSpace(s.collection) == "" {
		if s.fallback != nil {
			return s.fallback.Stats(ctx, filters)
		}
		return memory.MemoryStats{}, fmt.Errorf("QDRANT_COLLECTION is required")
	}

	stats := memory.MemoryStats{
		ByKind:    make(map[string]int),
		BySpeaker: make(map[string]int),
		ByProject: make(map[string]int),
	}

	var offset any
	lastOffset := ""
	for {
		body := map[string]any{
			"limit":        256,
			"with_payload": true,
			"with_vector":  false,
		}
		if filter := toQdrantFilter(filters); filter != nil {
			body["filter"] = filter
		}
		if offset != nil {
			body["offset"] = offset
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
				if s.fallback != nil {
					return s.fallback.Stats(ctx, filters)
				}
				return memory.MemoryStats{}, nil
			}
			if s.fallback != nil {
				return s.fallback.Stats(ctx, filters)
			}
			return memory.MemoryStats{}, err
		}

		for _, point := range response.Result.Points {
			stats.Total++
			if kind := strings.TrimSpace(asString(point.Payload["memory_kind"])); kind != "" {
				stats.ByKind[kind]++
			}
			if speaker := strings.TrimSpace(asString(point.Payload["speaker"])); speaker != "" {
				stats.BySpeaker[speaker]++
			}
			if project := strings.TrimSpace(asString(point.Payload["project"])); project != "" {
				stats.ByProject[project]++
			}
		}

		nextOffset := response.Result.NextPageOffset
		if nextOffset == nil {
			break
		}
		nextOffsetRaw := fmt.Sprintf("%v", nextOffset)
		if nextOffsetRaw == "" || nextOffsetRaw == lastOffset {
			break
		}
		lastOffset = nextOffsetRaw
		offset = nextOffset
		if len(response.Result.Points) == 0 {
			break
		}
	}

	if stats.Total == 0 && s.fallback != nil {
		return s.fallback.Stats(ctx, filters)
	}

	return stats, nil
}

func (s *QdrantMemoryStore) DeleteByFilters(ctx context.Context, filters memory.SearchFilters) error {
	if strings.TrimSpace(s.collection) == "" {
		return fmt.Errorf("QDRANT_COLLECTION is required")
	}
	filter := toQdrantFilter(filters)
	if filter == nil {
		return nil
	}

	_, err := s.client.doJSON(ctx, http.MethodPost, fmt.Sprintf("/collections/%s/points/delete?wait=true", s.collection), map[string]any{"filter": filter}, nil)
	if err != nil {
		return err
	}
	if fallbackDelete, ok := s.fallback.(interface {
		DeleteByFilters(memory.SearchFilters) error
	}); ok {
		_ = fallbackDelete.DeleteByFilters(filters)
	}
	return nil
}

func (s *QdrantMemoryStore) ensureCollection(ctx context.Context, dims int) error {
	if s.collection == "" {
		return fmt.Errorf("QDRANT_COLLECTION is required")
	}
	if dims <= 0 {
		return fmt.Errorf("embedding dimensions must be greater than zero")
	}

	var response struct {
		Result struct {
			Config struct {
				Params struct {
					Vectors json.RawMessage `json:"vectors"`
				} `json:"params"`
			} `json:"config"`
		} `json:"result"`
	}

	status, err := s.client.doJSON(ctx, http.MethodGet, fmt.Sprintf("/collections/%s", s.collection), nil, &response)
	if err != nil {
		if status != http.StatusNotFound {
			return err
		}
		createBody := map[string]any{}
		if s.vectorName == "" {
			createBody["vectors"] = map[string]any{
				"size":     dims,
				"distance": s.distance,
			}
		} else {
			createBody["vectors"] = map[string]any{
				s.vectorName: map[string]any{
					"size":     dims,
					"distance": s.distance,
				},
			}
		}

		_, createErr := s.client.doJSON(ctx, http.MethodPut, fmt.Sprintf("/collections/%s", s.collection), createBody, nil)
		return createErr
	}

	existingSize := parseVectorSize(response.Result.Config.Params.Vectors, s.vectorName)
	if existingSize > 0 && existingSize != dims {
		return &DimensionMismatchError{CollectionSize: existingSize, EmbeddingSize: dims}
	}
	return nil
}

func parseVectorSize(raw json.RawMessage, vectorName string) int {
	if len(raw) == 0 {
		return 0
	}

	var unnamed struct {
		Size int `json:"size"`
	}
	if err := json.Unmarshal(raw, &unnamed); err == nil && unnamed.Size > 0 {
		return unnamed.Size
	}

	var named map[string]struct {
		Size int `json:"size"`
	}
	if err := json.Unmarshal(raw, &named); err != nil {
		return 0
	}
	if len(named) == 0 {
		return 0
	}
	if vectorName != "" {
		if entry, ok := named[vectorName]; ok {
			return entry.Size
		}
		return 0
	}
	for _, entry := range named {
		if entry.Size > 0 {
			return entry.Size
		}
	}
	return 0
}

func qdrantPointID(memoryID string) string {
	trimmed := strings.TrimSpace(memoryID)
	if isUUID(trimmed) {
		return strings.ToLower(trimmed)
	}

	hash := md5.Sum([]byte(trimmed))
	hash[6] = (hash[6] & 0x0f) | 0x50
	hash[8] = (hash[8] & 0x3f) | 0x80
	return formatUUID(hash)
}

func formatUUID(b [16]byte) string {
	hexValue := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexValue[0:8], hexValue[8:12], hexValue[12:16], hexValue[16:20], hexValue[20:32])
}

func isUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for idx, r := range value {
		switch idx {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !isHexDigit(r) {
				return false
			}
		}
	}
	return true
}

func isHexDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func toQdrantFilter(filters memory.SearchFilters) map[string]any {
	must := make([]map[string]any, 0)
	appendMatch := func(key, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		must = append(must, map[string]any{
			"key": key,
			"match": map[string]any{
				"value": value,
			},
		})
	}

	appendMatch("project", filters.Project)
	appendMatch("memory_kind", filters.MemoryKind)
	appendMatch("speaker", filters.Speaker)
	appendMatch("source_type", filters.SourceType)
	appendMatch("conversation_id", filters.ConversationID)
	for _, tag := range filters.Tags {
		appendMatch("tags", tag)
	}

	if len(must) == 0 {
		return nil
	}
	return map[string]any{"must": must}
}

func toQdrantPayload(point memory.MemoryPoint) map[string]any {
	payload := map[string]any{
		"memory_id":            point.MemoryID,
		"conversation_id":      point.ConversationID,
		"session_id":           point.SessionID,
		"source_type":          point.SourceType,
		"source_title":         point.SourceTitle,
		"timestamp":            point.Timestamp.Format(time.RFC3339),
		"speaker":              point.Speaker,
		"project":              point.Project,
		"tags":                 point.Tags,
		"memory_kind":          point.MemoryKind,
		"importance":           point.Importance,
		"text":                 point.Text,
		"source_quote":         point.SourceQuote,
		"summary":              point.Summary,
		"used_memory_ids":      point.UsedMemoryIDs,
		"embedding_provider":   point.EmbeddingProvider,
		"embedding_model":      point.EmbeddingModel,
		"embedding_dimensions": point.EmbeddingDimensions,
		"canonical":            point.Canonical,
		"confidence":           point.Confidence,
		"status":               point.Status,
		"source_memory_ids":    point.SourceMemoryIDs,
		"source_quotes":        point.SourceQuotes,
		"reviewed_by":          point.ReviewedBy,
		"created_by_loop":      point.CreatedByLoop,
		"supersedes":           point.Supersedes,
		"merged_from":          point.MergedFrom,
		"approximate_date":     point.ApproximateDate,
		"date_basis":           point.DateBasis,
		"rejection_reason":     point.RejectionReason,
		"merge_target_id":      point.MergeTargetID,
	}
	if point.ReviewedAt != nil && !point.ReviewedAt.IsZero() {
		payload["reviewed_at"] = point.ReviewedAt.Format(time.RFC3339)
	}
	return payload
}

func fromQdrantPayload(payload map[string]any, score float64) memory.SearchResult {
	return memory.SearchResult{
		MemoryID:            asString(payload["memory_id"]),
		ConversationID:      asString(payload["conversation_id"]),
		SessionID:           asString(payload["session_id"]),
		SourceTitle:         asString(payload["source_title"]),
		SourceType:          asString(payload["source_type"]),
		Timestamp:           asTime(payload["timestamp"]),
		Speaker:             asString(payload["speaker"]),
		Project:             asString(payload["project"]),
		MemoryKind:          asString(payload["memory_kind"]),
		Tags:                asStringSlice(payload["tags"]),
		Summary:             asString(payload["summary"]),
		SourceQuote:         asString(payload["source_quote"]),
		Text:                asString(payload["text"]),
		UsedMemoryIDs:       asStringSlice(payload["used_memory_ids"]),
		Score:               score,
		EmbeddingProvider:   asString(payload["embedding_provider"]),
		EmbeddingModel:      asString(payload["embedding_model"]),
		EmbeddingDimensions: asInt(payload["embedding_dimensions"]),
		Canonical:           asBool(payload["canonical"]),
		Confidence:          asString(payload["confidence"]),
		Status:              asString(payload["status"]),
		SourceMemoryIDs:     asStringSlice(payload["source_memory_ids"]),
		SourceQuotes:        asStringSlice(payload["source_quotes"]),
		ReviewedAt:          asTimePtr(payload["reviewed_at"]),
		ReviewedBy:          asString(payload["reviewed_by"]),
		CreatedByLoop:       asBool(payload["created_by_loop"]),
		Supersedes:          asStringSlice(payload["supersedes"]),
		MergedFrom:          asStringSlice(payload["merged_from"]),
		ApproximateDate:     asString(payload["approximate_date"]),
		DateBasis:           asString(payload["date_basis"]),
	}
}

func asString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func asStringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, asString(item))
		}
		return out
	default:
		return nil
	}
}

func asInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	default:
		return 0
	}
}

func asBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(v))
		return parsed
	default:
		return false
	}
}

func asTime(value any) time.Time {
	raw := strings.TrimSpace(asString(value))
	if raw == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return ts
}

func asTimePtr(value any) *time.Time {
	ts := asTime(value)
	if ts.IsZero() {
		return nil
	}
	return &ts
}

func applyCanonicalBoost(score float64, canonical bool, status string) float64 {
	boost := 1.0
	if canonical {
		boost *= 1.25
	}
	switch strings.TrimSpace(status) {
	case memory.StatusApprovedByUser, memory.StatusDirectUserStatement:
		boost *= 1.2
	case memory.StatusInferredFromSources:
		boost *= 1.08
	case memory.StatusNeedsReview:
		boost *= 0.98
	}
	return score * boost
}

func shouldExcludeByStatus(includeRejected bool, status string) bool {
	status = strings.TrimSpace(status)
	if status == "" || includeRejected {
		return false
	}
	switch status {
	case memory.StatusRejected, memory.StatusContradicted, memory.StatusOutdated:
		return true
	default:
		return false
	}
}
