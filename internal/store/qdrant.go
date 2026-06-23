package store

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type QdrantClient struct {
	baseURL string
	http    *http.Client
}

func NewQdrantClient(url string) *QdrantClient {
	return &QdrantClient{
		baseURL: strings.TrimRight(url, "/"),
		http: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

func (c *QdrantClient) Health(ctx context.Context) error {
	if c == nil || c.baseURL == "" {
		return fmt.Errorf("qdrant client is not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/collections", nil)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("qdrant responded with status %d", resp.StatusCode)
	}

	return nil
}
