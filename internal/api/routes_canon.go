package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/meistro57/frontpocket/internal/memory"
	"github.com/meistro57/frontpocket/internal/memoryloop"
)

type rejectRequest struct {
	Reason     string `json:"reason"`
	ReviewedBy string `json:"reviewed_by"`
}

type mergeRequest struct {
	TargetID   string `json:"target_id"`
	ReviewedBy string `json:"reviewed_by"`
}

type approveRequest struct {
	ReviewedBy string                `json:"reviewed_by"`
	Edited     *memoryloop.Candidate `json:"edited,omitempty"`
}

func (s *Server) handleProposedCanonList(w http.ResponseWriter, r *http.Request) {
	reviewed := parseReviewedFilter(r.URL.Query().Get("reviewed"))
	items, err := s.reviewQueue.List(memoryloop.CandidateFilter{
		Status:     strings.TrimSpace(r.URL.Query().Get("status")),
		MemoryKind: strings.TrimSpace(r.URL.Query().Get("memory_kind")),
		Project:    strings.TrimSpace(r.URL.Query().Get("project")),
		Speaker:    strings.TrimSpace(r.URL.Query().Get("speaker")),
		Confidence: strings.TrimSpace(r.URL.Query().Get("confidence")),
		Reviewed:   reviewed,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, memory.ErrorBody{Code: "CANON_LIST_FAILED", Message: "Could not list proposed canon.", Detail: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "count": len(items)})
}

func (s *Server) handleProposedCanonGet(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{Code: "VALIDATION_ERROR", Message: "candidate id is required."})
		return
	}
	item, found, err := s.reviewQueue.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, memory.ErrorBody{Code: "CANON_GET_FAILED", Message: "Could not load proposed canon.", Detail: err.Error()})
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, memory.ErrorBody{Code: "NOT_FOUND", Message: "Candidate not found."})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleProposedCanonApprove(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{Code: "VALIDATION_ERROR", Message: "candidate id is required."})
		return
	}
	var req approveRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, memory.ErrorBody{Code: "INVALID_REQUEST", Message: "Request body must be valid JSON.", Detail: err.Error()})
			return
		}
	}
	item, err := s.reviewQueue.Approve(r.Context(), id, strings.TrimSpace(req.ReviewedBy), s.memoryStore, req.Edited)
	if err != nil {
		writeError(w, http.StatusInternalServerError, memory.ErrorBody{Code: "CANON_APPROVE_FAILED", Message: "Could not approve proposed canon.", Detail: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleProposedCanonReject(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{Code: "VALIDATION_ERROR", Message: "candidate id is required."})
		return
	}
	var req rejectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{Code: "INVALID_REQUEST", Message: "Request body must be valid JSON.", Detail: err.Error()})
		return
	}
	item, err := s.reviewQueue.Reject(id, strings.TrimSpace(req.Reason), strings.TrimSpace(req.ReviewedBy))
	if err != nil {
		writeError(w, http.StatusInternalServerError, memory.ErrorBody{Code: "CANON_REJECT_FAILED", Message: "Could not reject proposed canon.", Detail: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleProposedCanonMerge(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{Code: "VALIDATION_ERROR", Message: "candidate id is required."})
		return
	}
	var req mergeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{Code: "INVALID_REQUEST", Message: "Request body must be valid JSON.", Detail: err.Error()})
		return
	}
	if strings.TrimSpace(req.TargetID) == "" {
		writeError(w, http.StatusBadRequest, memory.ErrorBody{Code: "VALIDATION_ERROR", Message: "target_id is required."})
		return
	}
	item, err := s.reviewQueue.Merge(id, strings.TrimSpace(req.TargetID), strings.TrimSpace(req.ReviewedBy))
	if err != nil {
		writeError(w, http.StatusInternalServerError, memory.ErrorBody{Code: "CANON_MERGE_FAILED", Message: "Could not merge proposed canon.", Detail: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func parseReviewedFilter(raw string) *bool {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	switch trimmed {
	case "true", "1", "yes", "y":
		v := true
		return &v
	case "false", "0", "no", "n":
		v := false
		return &v
	default:
		return nil
	}
}
