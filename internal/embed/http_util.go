package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxEmbeddingResponseBytes int64 = 32 << 20

func newHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

func postJSON(ctx context.Context, client *http.Client, endpoint string, headers map[string]string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		if strings.TrimSpace(v) != "" {
			req.Header.Set(k, v)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	limited := &io.LimitedReader{R: resp.Body, N: maxEmbeddingResponseBytes + 1}
	respBody, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if int64(len(respBody)) > maxEmbeddingResponseBytes {
		return fmt.Errorf("embedding provider response exceeded %d bytes", maxEmbeddingResponseBytes)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("embedding provider returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if out == nil {
		return nil
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("failed to decode embedding response: %w", err)
	}
	return nil
}

func toFloat32Slice(values []float64) []float32 {
	out := make([]float32, len(values))
	for i, v := range values {
		out[i] = float32(v)
	}
	return out
}

func enforceDimensions(vector []float32, expected int, provider, model string) error {
	if expected <= 0 {
		return nil
	}
	if len(vector) != expected {
		return fmt.Errorf("%s model %s returned %d dimensions, expected %d", provider, model, len(vector), expected)
	}
	return nil
}
