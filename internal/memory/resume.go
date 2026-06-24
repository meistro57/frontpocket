package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// journalVersion is the on-disk schema version for a resume journal. Bump it
// when the persisted shape changes incompatibly.
const journalVersion = 1

// JournalMeta identifies the import a journal belongs to. A resume is only
// valid when the meta matches the journal on disk; otherwise the journal is for
// a different source/model/collection and must not be reused.
type JournalMeta struct {
	Source         string `json:"source"`
	Collection     string `json:"collection"`
	EmbeddingModel string `json:"embedding_model"`
}

// journalState is the JSON document persisted to disk after each batch.
type journalState struct {
	Version         int    `json:"version"`
	Source          string `json:"source"`
	Collection      string `json:"collection"`
	EmbeddingModel  string `json:"embedding_model"`
	TotalRecords    int    `json:"total_records"`
	LastRecordIndex int    `json:"last_record_index"`
	ChunksEmbedded  int    `json:"chunks_embedded"`
	UpdatedAt       string `json:"updated_at"`
}

// FileJournal is a ResumeJournal backed by a single JSON file. Each Commit
// rewrites the file atomically (temp file + rename) so a crash mid-write cannot
// corrupt the checkpoint.
type FileJournal struct {
	path  string
	state journalState
}

// OpenFileJournal opens (or creates) a journal at path for the import described
// by meta. If a journal already exists it is loaded and validated against meta;
// a mismatch returns an error so a stale journal can't silently skip records of
// a different import. The returned bool reports whether existing progress was
// found (true) versus a fresh journal (false).
func OpenFileJournal(path string, meta JournalMeta) (*FileJournal, bool, error) {
	j := &FileJournal{
		path: path,
		state: journalState{
			Version:         journalVersion,
			Source:          meta.Source,
			Collection:      meta.Collection,
			EmbeddingModel:  meta.EmbeddingModel,
			LastRecordIndex: -1,
		},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return j, false, nil
		}
		return nil, false, fmt.Errorf("read resume journal: %w", err)
	}

	var existing journalState
	if err := json.Unmarshal(data, &existing); err != nil {
		return nil, false, fmt.Errorf("parse resume journal %s: %w", path, err)
	}
	if existing.Version != journalVersion {
		return nil, false, fmt.Errorf("resume journal %s has unsupported version %d (want %d)", path, existing.Version, journalVersion)
	}
	if existing.Source != meta.Source || existing.Collection != meta.Collection || existing.EmbeddingModel != meta.EmbeddingModel {
		return nil, false, fmt.Errorf(
			"resume journal %s is for a different import (source=%q collection=%q model=%q); delete it to start over",
			path, existing.Source, existing.Collection, existing.EmbeddingModel,
		)
	}

	j.state = existing
	return j, true, nil
}

// LastRecordIndex returns the last committed record index, or -1.
func (j *FileJournal) LastRecordIndex() int {
	if j == nil {
		return -1
	}
	return j.state.LastRecordIndex
}

// ChunksEmbedded returns the total chunks recorded as embedded so far.
func (j *FileJournal) ChunksEmbedded() int {
	if j == nil {
		return 0
	}
	return j.state.ChunksEmbedded
}

// Commit persists cp atomically. It never moves the checkpoint backwards.
func (j *FileJournal) Commit(cp Checkpoint) error {
	if cp.LastRecordIndex < j.state.LastRecordIndex {
		return nil
	}
	j.state.LastRecordIndex = cp.LastRecordIndex
	j.state.TotalRecords = cp.TotalRecords
	j.state.ChunksEmbedded = cp.ChunksEmbedded
	j.state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	data, err := json.MarshalIndent(j.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode resume journal: %w", err)
	}

	tmp := j.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write resume journal: %w", err)
	}
	if err := os.Rename(tmp, j.path); err != nil {
		return fmt.Errorf("commit resume journal: %w", err)
	}
	return nil
}

// Remove deletes the journal file. Call it after a fully successful import so a
// later run of the same source starts fresh.
func (j *FileJournal) Remove() error {
	if j == nil {
		return nil
	}
	if err := os.Remove(j.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
