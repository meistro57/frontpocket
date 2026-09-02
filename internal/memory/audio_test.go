package memory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAudioToMessageRecordsWithCachedTranscript(t *testing.T) {
	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, "cache")
	transcriptsDir := filepath.Join(cacheDir, "transcripts")
	if err := os.MkdirAll(transcriptsDir, 0o755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}

	// Create a dummy audio file
	audioFile := filepath.Join(tempDir, "quantum_resonance.m4a")
	if err := os.WriteFile(audioFile, []byte("fake-audio-bytes"), 0o644); err != nil {
		t.Fatalf("failed to write dummy audio: %v", err)
	}

	// Pre-populate transcript cache
	cachedData := transcriptData{
		File:     audioFile,
		Engine:   "whisperx",
		Model:    "base",
		Duration: 35.0,
		Segments: []transcriptSegment{
			{Start: 0.0, End: 10.0, Text: "Welcome to the deep dive on discrete geometric resonance."},
			{Start: 10.5, End: 22.0, Text: "We are examining the E8 hyper-lattice and cycle clock theory."},
			{Start: 22.5, End: 35.0, Text: "Notice how vacuum energy suppression occurs at irrational harmonic angles."},
		},
	}
	cachedJSON, _ := json.Marshal(cachedData)
	convID := sanitizeID(cleanTitle("quantum_resonance"))
	cacheFile := filepath.Join(transcriptsDir, convID+".json")
	if err := os.WriteFile(cacheFile, cachedJSON, 0o644); err != nil {
		t.Fatalf("failed to write cached transcript: %v", err)
	}

	opts := FolderImportOptions{
		Project:    "UTMGR",
		CacheDir:   cacheDir,
		NoAudio:    false,
		AIProvider: "test",
	}

	records, err := AudioToMessageRecords(context.Background(), audioFile, opts)
	if err != nil {
		t.Fatalf("AudioToMessageRecords failed: %v", err)
	}

	if len(records) == 0 {
		t.Fatalf("expected records, got 0")
	}

	t.Logf("Generated %d audio turn records", len(records))
	for i, r := range records {
		t.Logf("Record %d: %s", i+1, r.Text)
		if r.SourceType != "audio_overview" {
			t.Errorf("expected source_type audio_overview, got %s", r.SourceType)
		}
		if r.Speaker != "audio_overview" {
			t.Errorf("expected speaker audio_overview, got %s", r.Speaker)
		}
	}
}
