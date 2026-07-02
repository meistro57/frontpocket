package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
)

type scrollResponse struct {
	Result struct {
		Points []struct {
			ID      any            `json:"id"`
			Payload map[string]any `json:"payload"`
		} `json:"points"`
		NextPageOffset any `json:"next_page_offset"`
	} `json:"result"`
}

func main() {
	qdrantURL := flag.String("qdrant-url", envOrDefault("QDRANT_URL", "http://localhost:6333"), "Qdrant base URL")
	collection := flag.String("collection", envOrDefault("QDRANT_COLLECTION", "frontpocket_memory"), "Qdrant collection")
	provider := flag.String("provider", "chatgpt", "ai_provider value to set")
	batchSize := flag.Int("batch-size", 256, "set_payload batch size")
	dryRun := flag.Bool("dry-run", true, "report-only mode; do not write payload updates")
	flag.Parse()

	if strings.TrimSpace(*collection) == "" {
		exitf("collection is required")
	}
	if strings.TrimSpace(*provider) == "" {
		exitf("provider is required")
	}
	if *batchSize <= 0 {
		exitf("batch-size must be > 0")
	}

	baseURL := strings.TrimRight(strings.TrimSpace(*qdrantURL), "/")
	client := &http.Client{}

	missingIDs := make([]string, 0, 4096)
	seenMissing := make(map[string]struct{}, 4096)
	offset := any(nil)
	scanned := 0

	for {
		body := map[string]any{
			"limit":        512,
			"with_payload": true,
			"with_vector":  false,
		}
		if offset != nil {
			body["offset"] = offset
		}

		var resp scrollResponse
		if err := postJSON(client, baseURL+"/collections/"+strings.TrimSpace(*collection)+"/points/scroll", body, &resp); err != nil {
			exitf("scroll failed: %v", err)
		}

		for _, point := range resp.Result.Points {
			scanned++
			if hasAIProvider(point.Payload) {
				continue
			}
			id := strings.TrimSpace(fmt.Sprintf("%v", point.ID))
			if id == "" {
				continue
			}
			if _, exists := seenMissing[id]; exists {
				continue
			}
			seenMissing[id] = struct{}{}
			missingIDs = append(missingIDs, id)
		}

		nextOffset := resp.Result.NextPageOffset
		if nextOffset == nil {
			break
		}
		nextOffsetRaw := strings.TrimSpace(fmt.Sprintf("%v", nextOffset))
		if nextOffsetRaw == "" || (offset != nil && nextOffsetRaw == fmt.Sprintf("%v", offset)) {
			break
		}
		offset = nextOffset
		if len(resp.Result.Points) == 0 {
			break
		}
	}

	sort.Strings(missingIDs)
	fmt.Printf("collection: %s\n", strings.TrimSpace(*collection))
	fmt.Printf("scanned_points: %d\n", scanned)
	fmt.Printf("missing_ai_provider: %d\n", len(missingIDs))
	fmt.Printf("dry_run: %t\n", *dryRun)
	if len(missingIDs) > 0 {
		preview := missingIDs
		if len(preview) > 5 {
			preview = preview[:5]
		}
		fmt.Printf("sample_missing_ids: %s\n", strings.Join(preview, ", "))
	}
	if *dryRun || len(missingIDs) == 0 {
		return
	}

	updated := 0
	for start := 0; start < len(missingIDs); start += *batchSize {
		end := start + *batchSize
		if end > len(missingIDs) {
			end = len(missingIDs)
		}
		batch := missingIDs[start:end]
		body := map[string]any{
			"payload": map[string]any{"ai_provider": strings.TrimSpace(*provider)},
			"points":  batch,
		}
		if err := postJSON(client, baseURL+"/collections/"+strings.TrimSpace(*collection)+"/points/set_payload?wait=true", body, nil); err != nil {
			exitf("set_payload failed at batch starting %d: %v", start, err)
		}
		updated += len(batch)
		fmt.Printf("updated: %d/%d\n", updated, len(missingIDs))
	}

	fmt.Printf("backfill_complete: updated=%d target=%d\n", updated, len(missingIDs))
}

func hasAIProvider(payload map[string]any) bool {
	if payload == nil {
		return false
	}
	value, ok := payload["ai_provider"]
	if !ok {
		return false
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value)) != ""
}

func postJSON(client *http.Client, url string, body any, out any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return err
	}
	if response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("status %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	if out != nil && len(payload) > 0 {
		if err := json.Unmarshal(payload, out); err != nil {
			return err
		}
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
