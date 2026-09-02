package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

type QdrantCollectionDescriptor struct {
	Name       string
	Exists     bool
	Points     int
	VectorSize int
}

type QdrantSearchHit struct {
	Score   float64
	Payload map[string]any
}

type QdrantSearchRequest struct {
	Collection string
	VectorName string
	Vector     []float32
	Limit      int
	Filter     map[string]any
}

type QdrantScrollRequest struct {
	Collection string
	Limit      int
	Offset     any
	Filter     map[string]any
}

func (c *QdrantClient) ListCollections(ctx context.Context) ([]string, error) {
	if c == nil || strings.TrimSpace(c.baseURL) == "" {
		return nil, fmt.Errorf("qdrant client is not configured")
	}

	var response struct {
		Result struct {
			Collections []struct {
				Name string `json:"name"`
			} `json:"collections"`
		} `json:"result"`
	}
	_, err := c.doJSON(ctx, http.MethodGet, "/collections", nil, &response)
	if err != nil {
		return nil, err
	}

	collections := make([]string, 0, len(response.Result.Collections))
	for _, item := range response.Result.Collections {
		name := strings.TrimSpace(item.Name)
		if name != "" {
			collections = append(collections, name)
		}
	}
	sort.Strings(collections)
	return collections, nil
}

func (c *QdrantClient) DescribeCollection(ctx context.Context, collection, vectorName string) (QdrantCollectionDescriptor, error) {
	trimmed := strings.TrimSpace(collection)
	if trimmed == "" {
		return QdrantCollectionDescriptor{}, fmt.Errorf("collection is required")
	}

	var response struct {
		Result struct {
			PointsCount int `json:"points_count"`
			Config      struct {
				Params struct {
					Vectors any `json:"vectors"`
				} `json:"params"`
			} `json:"config"`
		} `json:"result"`
	}
	status, err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/collections/%s", trimmed), nil, &response)
	if err != nil {
		if status == http.StatusNotFound {
			return QdrantCollectionDescriptor{Name: trimmed}, nil
		}
		return QdrantCollectionDescriptor{}, err
	}

	vectorRaw, err := json.Marshal(response.Result.Config.Params.Vectors)
	if err != nil {
		vectorRaw = nil
	}
	return QdrantCollectionDescriptor{
		Name:       trimmed,
		Exists:     true,
		Points:     response.Result.PointsCount,
		VectorSize: parseVectorSize(vectorRaw, strings.TrimSpace(vectorName)),
	}, nil
}

func (c *QdrantClient) SearchCollection(ctx context.Context, req QdrantSearchRequest) ([]QdrantSearchHit, error) {
	collection := strings.TrimSpace(req.Collection)
	if collection == "" {
		return nil, fmt.Errorf("collection is required")
	}
	if len(req.Vector) == 0 {
		return nil, fmt.Errorf("search vector is required")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	searchVector := any(req.Vector)
	if trimmedVectorName := strings.TrimSpace(req.VectorName); trimmedVectorName != "" {
		searchVector = map[string][]float32{trimmedVectorName: req.Vector}
	}

	body := map[string]any{
		"vector":       searchVector,
		"limit":        limit,
		"with_payload": true,
		"with_vector":  false,
	}
	if req.Filter != nil {
		body["filter"] = req.Filter
	}

	var response struct {
		Result []struct {
			Score   float64        `json:"score"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	_, err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/collections/%s/points/search", collection), body, &response)
	if err != nil {
		return nil, err
	}

	hits := make([]QdrantSearchHit, 0, len(response.Result))
	for _, item := range response.Result {
		hits = append(hits, QdrantSearchHit{Score: item.Score, Payload: item.Payload})
	}
	return hits, nil
}

func (c *QdrantClient) ScrollCollection(ctx context.Context, req QdrantScrollRequest) ([]map[string]any, any, error) {
	collection := strings.TrimSpace(req.Collection)
	if collection == "" {
		return nil, nil, fmt.Errorf("collection is required")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 128
	}

	body := map[string]any{
		"limit":        limit,
		"with_payload": true,
		"with_vector":  false,
	}
	if req.Filter != nil {
		body["filter"] = req.Filter
	}
	if req.Offset != nil {
		body["offset"] = req.Offset
	}

	var response struct {
		Result struct {
			Points []struct {
				Payload map[string]any `json:"payload"`
			} `json:"points"`
			NextPageOffset any `json:"next_page_offset"`
		} `json:"result"`
	}
	status, err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/collections/%s/points/scroll", collection), body, &response)
	if err != nil {
		if status == http.StatusNotFound {
			return []map[string]any{}, nil, nil
		}
		return nil, nil, err
	}

	payloads := make([]map[string]any, 0, len(response.Result.Points))
	for _, point := range response.Result.Points {
		payloads = append(payloads, point.Payload)
	}
	return payloads, response.Result.NextPageOffset, nil
}
