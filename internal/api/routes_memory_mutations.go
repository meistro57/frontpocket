package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
)

type mutationAuditLog struct {
	path string
	mu   sync.Mutex
}

func newMutationAuditLog(path string) *mutationAuditLog {
	return &mutationAuditLog{path: strings.TrimSpace(path)}
}

// Record appends one mutation to the audit log. A logging failure never blocks
// the mutation itself but is surfaced to the caller so it is not silent.
func (l *mutationAuditLog) Record(record memory.MutationRecord) error {
	if l == nil || l.path == "" {
		return nil
	}
	if record.Timestamp == "" {
		record.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if dir := filepath.Dir(l.path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	file, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return nil
}

type memoryPayloadPatcher interface {
	SetMemoryPayloadFields(ctx context.Context, memoryIDs []string, fields map[string]any) error
}

type memoryPointDeleter interface {
	DeleteMemoryPoints(ctx context.Context, memoryIDs []string) error
}

func (s *Server) memoryMutator() (memoryPayloadPatcher, *memory.ErrorBody) {
	patcher, ok := s.memoryStore.(memoryPayloadPatcher)
	if !ok {
		return nil, &memory.ErrorBody{
			Code:    "MUTATION_UNAVAILABLE",
			Message: "The configured memory store does not support mutations.",
		}
	}
	return patcher, nil
}

// requireAdminEndpoints reports whether mutation routes may run, writing a
// structured error when they are disabled.
func (s *Server) requireAdminEndpoints(w http.ResponseWriter) bool {
	if s.cfg.Security.EnableAdminEndpoints {
		return true
	}
	writeError(w, http.StatusForbidden, memory.ErrorBody{
		Code:    "ADMIN_ENDPOINTS_DISABLED",
		Message: "Mutation endpoints are disabled.",
		Detail:  "Set ENABLE_ADMIN_ENDPOINTS=true to allow pin, correct, forget, and export.",
	})
	return false
}

func (s *Server) handleMemoryPin(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminEndpoints(w) {
		return
	}
	var req memory.PinRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	ids := cleanStrings(req.MemoryIDs)
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{
			Code:    "VALIDATION_ERROR",
			Message: "memory_ids is required.",
		})
		return
	}

	patcher, errBody := s.memoryMutator()
	if errBody != nil {
		writeError(w, http.StatusNotImplemented, *errBody)
		return
	}

	pinned := true
	if req.Pinned != nil {
		pinned = *req.Pinned
	}
	now := time.Now().UTC().Format(time.RFC3339)
	fields := map[string]any{
		"pinned":    pinned,
		"pinned_at": now,
		"pinned_by": mutationActor(req.Actor),
	}
	if !pinned {
		fields["pinned_at"] = ""
	}

	if err := patcher.SetMemoryPayloadFields(r.Context(), ids, fields); err != nil {
		writeError(w, statusForStoreError(err), memory.ErrorBody{
			Code:    "PIN_FAILED",
			Message: "Could not update pin state.",
			Detail:  err.Error(),
		})
		return
	}

	s.recordMutation(memory.MutationRecord{
		Action:    mutationActionPinned,
		Actor:     mutationActor(req.Actor),
		MemoryIDs: ids,
		Fields:    []string{"pinned", "pinned_at", "pinned_by"},
		Reason:    req.Reason,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"updated_count": len(ids),
		"memory_ids":    ids,
		"pinned":        pinned,
	})
}

func (s *Server) handleMemoryCorrect(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminEndpoints(w) {
		return
	}
	var req memory.CorrectRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	memoryID := strings.TrimSpace(req.MemoryID)
	if memoryID == "" {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{
			Code:    "VALIDATION_ERROR",
			Message: "memory_id is required.",
		})
		return
	}

	patcher, errBody := s.memoryMutator()
	if errBody != nil {
		writeError(w, http.StatusNotImplemented, *errBody)
		return
	}

	fields := map[string]any{}
	touched := make([]string, 0, 8)
	setString := func(key string, value *string) {
		if value == nil {
			return
		}
		fields[key] = strings.TrimSpace(*value)
		touched = append(touched, key)
	}
	setString("text", req.Text)
	setString("summary", req.Summary)
	setString("source_quote", req.SourceQuote)
	setString("memory_kind", req.MemoryKind)
	setString("project", req.Project)
	if req.Tags != nil {
		fields["tags"] = cleanStrings(req.Tags)
		touched = append(touched, "tags")
	}

	if len(fields) == 0 {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{
			Code:    "VALIDATION_ERROR",
			Message: "At least one field to correct is required.",
			Detail:  "Provide text, summary, source_quote, memory_kind, project, or tags.",
		})
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	fields["corrected_at"] = now
	fields["corrected_by"] = mutationActor(req.Actor)
	touched = append(touched, "corrected_at", "corrected_by")

	if err := patcher.SetMemoryPayloadFields(r.Context(), []string{memoryID}, fields); err != nil {
		writeError(w, statusForStoreError(err), memory.ErrorBody{
			Code:    "CORRECTION_FAILED",
			Message: "Could not apply the correction.",
			Detail:  err.Error(),
		})
		return
	}

	s.recordMutation(memory.MutationRecord{
		Action:    mutationActionCorrected,
		Actor:     mutationActor(req.Actor),
		MemoryIDs: []string{memoryID},
		Fields:    touched,
		Reason:    req.Reason,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"memory_id":        memoryID,
		"corrected_at":     now,
		"corrected_fields": touched,
	})
}

func (s *Server) handleMemoryForget(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminEndpoints(w) {
		return
	}
	var req memory.ForgetRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	ids := cleanStrings(req.MemoryIDs)
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{
			Code:    "VALIDATION_ERROR",
			Message: "memory_ids is required.",
		})
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	actor := mutationActor(req.Actor)

	if req.Hard {
		deleter, ok := s.memoryStore.(memoryPointDeleter)
		if !ok {
			writeError(w, http.StatusNotImplemented, memory.ErrorBody{
				Code:    "MUTATION_UNAVAILABLE",
				Message: "The configured memory store does not support deletion.",
			})
			return
		}
		if err := deleter.DeleteMemoryPoints(r.Context(), ids); err != nil {
			writeError(w, statusForStoreError(err), memory.ErrorBody{
				Code:    "FORGET_FAILED",
				Message: "Could not delete memories.",
				Detail:  err.Error(),
			})
			return
		}
		s.recordMutation(memory.MutationRecord{
			Action:    mutationActionForgotten,
			Actor:     actor,
			MemoryIDs: ids,
			Reason:    req.Reason,
			Hard:      true,
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"deleted_count": len(ids),
			"memory_ids":    ids,
			"hard":          true,
		})
		return
	}

	patcher, errBody := s.memoryMutator()
	if errBody != nil {
		writeError(w, http.StatusNotImplemented, *errBody)
		return
	}
	fields := map[string]any{
		"status":     memory.StatusDeleted,
		"deleted_at": now,
		"deleted_by": actor,
	}
	if err := patcher.SetMemoryPayloadFields(r.Context(), ids, fields); err != nil {
		writeError(w, statusForStoreError(err), memory.ErrorBody{
			Code:    "FORGET_FAILED",
			Message: "Could not mark memories as deleted.",
			Detail:  err.Error(),
		})
		return
	}
	s.recordMutation(memory.MutationRecord{
		Action:    mutationActionForgotten,
		Actor:     actor,
		MemoryIDs: ids,
		Reason:    req.Reason,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"deleted_count": len(ids),
		"memory_ids":    ids,
		"hard":          false,
	})
}

func (s *Server) handleMemoryExport(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminEndpoints(w) {
		return
	}
	scroller, ok := s.memoryStore.(interface {
		ScrollRaw(ctx context.Context, limit int, offset string, filters memory.SearchFilters, since, until time.Time, includeCanonical bool) ([]memory.MemoryPoint, string, error)
	})
	if !ok {
		writeError(w, http.StatusNotImplemented, memory.ErrorBody{
			Code:    "EXPORT_UNAVAILABLE",
			Message: "The configured memory store does not support export.",
		})
		return
	}

	query := r.URL.Query()
	limit := parsePositiveIntOrDefault(query.Get("limit"), 500)
	if limit > 2000 {
		limit = 2000
	}
	points, nextOffset, err := scroller.ScrollRaw(
		r.Context(),
		limit,
		strings.TrimSpace(query.Get("offset")),
		statsFiltersFromQuery(r),
		parseTimeQuery(query.Get("since")),
		parseTimeQuery(query.Get("until")),
		parseBoolQuery(query.Get("include_canonical"), false),
	)
	if err != nil {
		writeError(w, statusForStoreError(err), memory.ErrorBody{
			Code:    "EXPORT_FAILED",
			Message: "Memory export failed.",
			Detail:  err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Next-Offset", nextOffset)
	w.WriteHeader(http.StatusOK)
	encoder := json.NewEncoder(w)
	for _, point := range points {
		if err := encoder.Encode(point); err != nil {
			return
		}
	}
}

func (s *Server) recordMutation(record memory.MutationRecord) {
	if s.mutationLog == nil {
		return
	}
	if err := s.mutationLog.Record(record); err != nil {
		s.logger.Warn("could not append memory mutation record", "action", record.Action, "error", err)
	}
}

func mutationActor(requested string) string {
	trimmed := strings.TrimSpace(requested)
	if trimmed != "" {
		return trimmed
	}
	return "api"
}

const (
	mutationActionPinned    = "pin"
	mutationActionCorrected = "correct"
	mutationActionForgotten = "forget"
)
