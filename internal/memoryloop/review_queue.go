package memoryloop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/meistro57/frontpocket/internal/memory"
)

type CandidateFilter struct {
	Status     string
	MemoryKind string
	Project    string
	Speaker    string
	Reviewed   *bool
	Confidence string
}

type FileReviewQueue struct {
	path string
	mu   sync.Mutex
}

func NewFileReviewQueue(path string) *FileReviewQueue {
	return &FileReviewQueue{path: strings.TrimSpace(path)}
}

func (q *FileReviewQueue) List(filter CandidateFilter) ([]Candidate, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	records, err := q.load()
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(records))
	for _, record := range records {
		if !matchesCandidateFilter(record, filter) {
			continue
		}
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (q *FileReviewQueue) Get(id string) (Candidate, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	records, err := q.load()
	if err != nil {
		return Candidate{}, false, err
	}
	for _, record := range records {
		if record.ID == id {
			return record, true, nil
		}
	}
	return Candidate{}, false, nil
}

func (q *FileReviewQueue) Upsert(candidates []Candidate) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	records, err := q.load()
	if err != nil {
		return err
	}
	index := make(map[string]int, len(records))
	for idx, record := range records {
		index[record.ID] = idx
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.ID) == "" {
			continue
		}
		now := time.Now().UTC()
		if candidate.CreatedAt.IsZero() {
			candidate.CreatedAt = now
		}
		candidate.UpdatedAt = now
		if idx, ok := index[candidate.ID]; ok {
			records[idx] = candidate
			continue
		}
		records = append(records, candidate)
	}
	return q.save(records)
}

func (q *FileReviewQueue) Reject(id, reason, reviewedBy string) (Candidate, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	records, err := q.load()
	if err != nil {
		return Candidate{}, err
	}
	for idx, record := range records {
		if record.ID != id {
			continue
		}
		now := time.Now().UTC()
		record.Status = memory.StatusRejected
		record.RejectionReason = strings.TrimSpace(reason)
		record.ReviewedBy = strings.TrimSpace(reviewedBy)
		record.ReviewedAt = &now
		record.UpdatedAt = now
		records[idx] = record
		if err := q.save(records); err != nil {
			return Candidate{}, err
		}
		return record, nil
	}
	return Candidate{}, fmt.Errorf("candidate %q not found", id)
}

func (q *FileReviewQueue) Approve(ctx context.Context, id, reviewedBy string, canonicalStore memory.MemoryStore, edited *Candidate) (Candidate, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	records, err := q.load()
	if err != nil {
		return Candidate{}, err
	}
	for idx, record := range records {
		if record.ID != id {
			continue
		}
		if edited != nil {
			record = *edited
			record.ID = id
		}
		now := time.Now().UTC()
		record.Canonical = true
		if record.Status != memory.StatusDirectUserStatement {
			record.Status = memory.StatusApprovedByUser
		}
		record.ReviewedBy = strings.TrimSpace(reviewedBy)
		record.ReviewedAt = &now
		record.UpdatedAt = now
		if canonicalStore != nil {
			point := candidateToMemoryPoint(record)
			if err := canonicalStore.Upsert(ctx, []memory.MemoryPoint{point}); err != nil {
				return Candidate{}, err
			}
		}
		records[idx] = record
		if err := q.save(records); err != nil {
			return Candidate{}, err
		}
		return record, nil
	}
	return Candidate{}, fmt.Errorf("candidate %q not found", id)
}

func (q *FileReviewQueue) Merge(id, targetID, reviewedBy string) (Candidate, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	records, err := q.load()
	if err != nil {
		return Candidate{}, err
	}
	for idx, record := range records {
		if record.ID != id {
			continue
		}
		now := time.Now().UTC()
		record.MergeTargetID = strings.TrimSpace(targetID)
		record.MergedFrom = unique(append(record.MergedFrom, id))
		record.ReviewedBy = strings.TrimSpace(reviewedBy)
		record.ReviewedAt = &now
		record.Status = memory.StatusNeedsReview
		record.UpdatedAt = now
		records[idx] = record
		if err := q.save(records); err != nil {
			return Candidate{}, err
		}
		return record, nil
	}
	return Candidate{}, fmt.Errorf("candidate %q not found", id)
}

func (q *FileReviewQueue) load() ([]Candidate, error) {
	if q.path == "" {
		return []Candidate{}, nil
	}
	data, err := os.ReadFile(q.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Candidate{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return []Candidate{}, nil
	}
	var records []Candidate
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func (q *FileReviewQueue) save(records []Candidate) error {
	if q.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(q.path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(q.path, payload, 0o644)
}

func candidateToMemoryPoint(candidate Candidate) memory.MemoryPoint {
	memoryID := strings.TrimSpace(candidate.ID)
	if memoryID == "" {
		memoryID = candidateID("approved", candidate.Summary)
	}
	now := time.Now().UTC()
	timestamp := now
	if candidate.ApproximateDate != "" {
		if parsed, err := time.Parse("2006-01-02", candidate.ApproximateDate); err == nil {
			timestamp = parsed
		}
	}
	summary := strings.TrimSpace(candidate.Summary)
	if summary == "" {
		summary = strings.TrimSpace(candidate.Text)
	}
	return memory.MemoryPoint{
		MemoryID:        memoryID,
		ConversationID:  "canon_review",
		SourceType:      "canonical_review",
		SourceTitle:     "Canonical Review Queue",
		Timestamp:       timestamp,
		Speaker:         strings.TrimSpace(candidate.Speaker),
		Project:         strings.TrimSpace(candidate.Project),
		Tags:            append([]string(nil), candidate.Tags...),
		MemoryKind:      strings.TrimSpace(candidate.MemoryKind),
		Text:            summary,
		SourceQuote:     firstOf(candidate.SourceQuotes, summary),
		Summary:         summary,
		Canonical:       true,
		Confidence:      strings.TrimSpace(candidate.Confidence),
		Status:          strings.TrimSpace(candidate.Status),
		SourceMemoryIDs: append([]string(nil), candidate.SourceMemoryIDs...),
		SourceQuotes:    append([]string(nil), candidate.SourceQuotes...),
		ReviewedAt:      candidate.ReviewedAt,
		ReviewedBy:      strings.TrimSpace(candidate.ReviewedBy),
		CreatedByLoop:   candidate.CreatedByLoop,
		Supersedes:      append([]string(nil), candidate.Supersedes...),
		MergedFrom:      append([]string(nil), candidate.MergedFrom...),
		ApproximateDate: strings.TrimSpace(candidate.ApproximateDate),
		DateBasis:       strings.TrimSpace(candidate.DateBasis),
		RejectionReason: strings.TrimSpace(candidate.RejectionReason),
		MergeTargetID:   strings.TrimSpace(candidate.MergeTargetID),
	}
}

func matchesCandidateFilter(record Candidate, filter CandidateFilter) bool {
	if filter.Status != "" && !strings.EqualFold(record.Status, filter.Status) {
		return false
	}
	if filter.MemoryKind != "" && !strings.EqualFold(record.MemoryKind, filter.MemoryKind) {
		return false
	}
	if filter.Project != "" && !strings.EqualFold(record.Project, filter.Project) {
		return false
	}
	if filter.Speaker != "" && !strings.EqualFold(record.Speaker, filter.Speaker) {
		return false
	}
	if filter.Confidence != "" && !strings.EqualFold(record.Confidence, filter.Confidence) {
		return false
	}
	if filter.Reviewed != nil {
		reviewed := record.ReviewedAt != nil
		if reviewed != *filter.Reviewed {
			return false
		}
	}
	return true
}

func firstOf(values []string, fallback string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return fallback
}
