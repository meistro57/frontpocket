package memoryloop

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
)

func TestReviewQueueUpsertKeepsReviewedCandidates(t *testing.T) {
	queue := NewFileReviewQueue(filepath.Join(t.TempDir(), "proposed.json"))
	candidate := Candidate{
		ID:         "cand_reviewed",
		Summary:    "Mark prefers tea over coffee.",
		MemoryKind: memory.KindPreference,
		Status:     memory.StatusInferredFromSources,
		CreatedAt:  time.Now().UTC().Add(-time.Hour),
	}
	if err := queue.Upsert([]Candidate{candidate}); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	if _, err := queue.Reject(candidate.ID, "wrong person", "mark"); err != nil {
		t.Fatalf("reject failed: %v", err)
	}

	revived := candidate
	revived.Summary = "Mark prefers tea over coffee (regenerated)."
	revived.Status = memory.StatusInferredFromSources
	revived.CreatedAt = time.Time{}
	if err := queue.Upsert([]Candidate{revived}); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}

	got, found, err := queue.Get(candidate.ID)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if !found {
		t.Fatal("expected candidate to still exist")
	}
	if got.Status != memory.StatusRejected {
		t.Fatalf("expected rejection to survive loop rerun, got status %q", got.Status)
	}
	if got.RejectionReason != "wrong person" {
		t.Fatalf("expected rejection reason to survive, got %q", got.RejectionReason)
	}
	if got.ReviewedAt == nil || got.ReviewedBy != "mark" {
		t.Fatalf("expected review metadata to survive, got %#v", got)
	}
	if !got.CreatedAt.Equal(candidate.CreatedAt) {
		t.Fatalf("expected original created_at to survive, got %v", got.CreatedAt)
	}
}
