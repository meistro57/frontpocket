package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meistro57/frontpocket/internal/config"
	"github.com/meistro57/frontpocket/internal/memory"
)

type mutatingStore struct {
	stubMemoryStore
	patchedIDs    []string
	patchedFields map[string]any
	deletedIDs    []string
	points        []memory.MemoryPoint
}

func (s *mutatingStore) SetMemoryPayloadFields(_ context.Context, ids []string, fields map[string]any) error {
	s.patchedIDs = append(s.patchedIDs, ids...)
	s.patchedFields = fields
	return nil
}

func (s *mutatingStore) DeleteMemoryPoints(_ context.Context, ids []string) error {
	s.deletedIDs = append(s.deletedIDs, ids...)
	return nil
}

func (s *mutatingStore) ScrollRaw(_ context.Context, _ int, _ string, _ memory.SearchFilters, _, _ time.Time, _ bool) ([]memory.MemoryPoint, string, error) {
	return s.points, "", nil
}

func newMutationTestServer(t *testing.T, store memory.MemoryStore, logPath string) *Server {
	t.Helper()
	return &Server{
		cfg: config.Config{
			Search:      config.SearchConfig{IncludeSourceQuote: true, IncludeFullText: true},
			ContextPack: config.ContextPackConfig{DefaultLimit: 5, MaxLimit: 10, MaxCharacters: 4000},
			Security:    config.SecurityConfig{EnableAdminEndpoints: true, MutationLogPath: logPath},
		},
		memoryStore:    store,
		searchCacheKey: "frontpocket-test",
		defaultSearch:  5,
		maxSearch:      20,
		mutationLog:    newMutationAuditLog(logPath),
	}
}

func TestMemoryPinPatchesPayloadAndAudits(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "mutations.jsonl")
	store := &mutatingStore{}
	s := newMutationTestServer(t, store, logPath)

	body := strings.NewReader(`{"memory_ids":["m1","m2"],"reason":"keep this","actor":"mark"}`)
	rr := httptest.NewRecorder()
	s.handleMemoryPin(rr, httptest.NewRequest(http.MethodPost, "/memory/pin", body))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(store.patchedIDs) != 2 {
		t.Fatalf("expected both memories patched, got %#v", store.patchedIDs)
	}
	if pinned, ok := store.patchedFields["pinned"].(bool); !ok || !pinned {
		t.Fatalf("expected pinned=true, got %#v", store.patchedFields["pinned"])
	}
	if store.patchedFields["pinned_by"] != "mark" {
		t.Fatalf("expected actor recorded, got %#v", store.patchedFields["pinned_by"])
	}

	records := readAuditRecords(t, logPath)
	if len(records) != 1 || records[0].Action != "pin" {
		t.Fatalf("expected one pin audit record, got %#v", records)
	}
	if records[0].Reason != "keep this" {
		t.Fatalf("expected reason recorded, got %q", records[0].Reason)
	}
}

func TestMemoryPinRequiresIDs(t *testing.T) {
	s := newMutationTestServer(t, &mutatingStore{}, filepath.Join(t.TempDir(), "mutations.jsonl"))

	rr := httptest.NewRecorder()
	s.handleMemoryPin(rr, httptest.NewRequest(http.MethodPost, "/memory/pin", strings.NewReader(`{"memory_ids":[]}`)))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestMemoryCorrectRejectsEmptyCorrection(t *testing.T) {
	s := newMutationTestServer(t, &mutatingStore{}, filepath.Join(t.TempDir(), "mutations.jsonl"))

	body := strings.NewReader(`{"memory_id":"m1"}`)
	rr := httptest.NewRecorder()
	s.handleMemoryCorrect(rr, httptest.NewRequest(http.MethodPost, "/memory/correct", body))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestMemoryCorrectAppliesFieldsAndAudits(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "mutations.jsonl")
	store := &mutatingStore{}
	s := newMutationTestServer(t, store, logPath)

	body := strings.NewReader(`{"memory_id":"m1","text":"corrected text","reason":"user clarified"}`)
	rr := httptest.NewRecorder()
	s.handleMemoryCorrect(rr, httptest.NewRequest(http.MethodPost, "/memory/correct", body))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if store.patchedFields["text"] != "corrected text" {
		t.Fatalf("expected corrected text, got %#v", store.patchedFields["text"])
	}
	if store.patchedFields["corrected_at"] == "" {
		t.Fatal("expected corrected_at stamp")
	}

	records := readAuditRecords(t, logPath)
	if len(records) != 1 || records[0].Action != "correct" {
		t.Fatalf("expected one correction record, got %#v", records)
	}
	if len(records[0].Fields) == 0 {
		t.Fatal("expected corrected field list in audit record")
	}
}

func TestMemoryForgetSoftDeletesByDefault(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "mutations.jsonl")
	store := &mutatingStore{}
	s := newMutationTestServer(t, store, logPath)

	body := strings.NewReader(`{"memory_ids":["m1"],"reason":"no longer true"}`)
	rr := httptest.NewRecorder()
	s.handleMemoryForget(rr, httptest.NewRequest(http.MethodDelete, "/memory/forget", body))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if store.patchedFields["status"] != memory.StatusDeleted {
		t.Fatalf("expected status=deleted, got %#v", store.patchedFields["status"])
	}
	if len(store.deletedIDs) != 0 {
		t.Fatalf("expected no hard delete, got %#v", store.deletedIDs)
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if resp["hard"] != false {
		t.Fatalf("expected hard=false in response, got %#v", resp["hard"])
	}
}

func TestMemoryForgetHardDeletesPoints(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "mutations.jsonl")
	store := &mutatingStore{}
	s := newMutationTestServer(t, store, logPath)

	body := strings.NewReader(`{"memory_ids":["m1","m2"],"hard":true}`)
	rr := httptest.NewRecorder()
	s.handleMemoryForget(rr, httptest.NewRequest(http.MethodDelete, "/memory/forget", body))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(store.deletedIDs) != 2 {
		t.Fatalf("expected both points deleted, got %#v", store.deletedIDs)
	}
	records := readAuditRecords(t, logPath)
	if len(records) != 1 || !records[0].Hard {
		t.Fatalf("expected a hard deletion record, got %#v", records)
	}
}

func TestMemoryMutationsBlockedWhenAdminDisabled(t *testing.T) {
	s := newMutationTestServer(t, &mutatingStore{}, "")
	s.cfg.Security.EnableAdminEndpoints = false

	for name, handler := range map[string]http.HandlerFunc{
		"pin":     s.handleMemoryPin,
		"correct": s.handleMemoryCorrect,
		"forget":  s.handleMemoryForget,
		"export":  s.handleMemoryExport,
	} {
		rr := httptest.NewRecorder()
		handler(rr, httptest.NewRequest(http.MethodPost, "/memory/"+name, strings.NewReader(`{}`)))
		if rr.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for %s when admin endpoints are disabled, got %d", name, rr.Code)
		}
		var envelope memory.ErrorEnvelope
		if err := json.NewDecoder(rr.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode error body failed: %v", err)
		}
		if envelope.Error.Code != "ADMIN_ENDPOINTS_DISABLED" {
			t.Fatalf("expected ADMIN_ENDPOINTS_DISABLED for %s, got %q", name, envelope.Error.Code)
		}
	}
}

func TestMemoryExportStreamsNDJSON(t *testing.T) {
	store := &mutatingStore{points: []memory.MemoryPoint{
		{MemoryID: "m1", Text: "first", SourceQuote: "first", Timestamp: time.Now().UTC()},
		{MemoryID: "m2", Text: "second", SourceQuote: "second", Timestamp: time.Now().UTC()},
	}}
	s := newMutationTestServer(t, store, "")

	rr := httptest.NewRecorder()
	s.handleMemoryExport(rr, httptest.NewRequest(http.MethodGet, "/memory/export?limit=10", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if contentType := rr.Header().Get("Content-Type"); contentType != "application/x-ndjson" {
		t.Fatalf("expected ndjson content type, got %q", contentType)
	}
	lines := strings.Split(strings.TrimSpace(rr.Body.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two exported lines, got %d", len(lines))
	}
	var first memory.MemoryPoint
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("decode exported record failed: %v", err)
	}
	if first.MemoryID != "m1" {
		t.Fatalf("expected m1 first, got %q", first.MemoryID)
	}
}

func TestMemoryPinUnsupportedStoreReturnsNotImplemented(t *testing.T) {
	s := newMutationTestServer(t, &stubMemoryStore{}, "")

	body := strings.NewReader(`{"memory_ids":["m1"]}`)
	rr := httptest.NewRecorder()
	s.handleMemoryPin(rr, httptest.NewRequest(http.MethodPost, "/memory/pin", body))

	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 for a store without payload patching, got %d", rr.Code)
	}
}

func readAuditRecords(t *testing.T, path string) []memory.MutationRecord {
	t.Helper()
	logger := newMutationAuditLog(path)
	if logger.path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read audit log: %v", err)
	}
	records := make([]memory.MutationRecord, 0, 1)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record memory.MutationRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("could not decode audit record %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}
