package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type transcriptData struct {
	File     string              `json:"file"`
	Engine   string              `json:"engine"`
	Model    string              `json:"model"`
	Duration float64             `json:"duration"`
	Segments []transcriptSegment `json:"segments"`
}

type transcriptSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// AudioToMessageRecords transcribes an audio or video file with Whisper and returns chunked records.
func AudioToMessageRecords(ctx context.Context, filePath string, opts FolderImportOptions) ([]MessageRecord, error) {
	if opts.NoAudio {
		return nil, nil
	}

	fi, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	baseName := filepath.Base(filePath)
	ext := strings.ToLower(filepath.Ext(baseName))
	docTitle := cleanTitle(strings.TrimSuffix(baseName, filepath.Ext(baseName)))
	convID := sanitizeID(docTitle)
	timestamp := fi.ModTime().UTC().Format(time.RFC3339)

	mimeType := "audio/mp4"
	sourceType := "audio_overview"
	category := "audio"
	speaker := "audio_overview"

	switch ext {
	case ".mp4":
		mimeType = "video/mp4"
		sourceType = "video_presentation"
		category = "video"
		speaker = "video_presentation"
	case ".mp3":
		mimeType = "audio/mpeg"
	case ".wav":
		mimeType = "audio/wav"
	}

	transcriptsDir := filepath.Join(opts.CacheDir, "transcripts")
	if err := os.MkdirAll(transcriptsDir, 0o755); err != nil {
		return nil, err
	}
	cachePath := filepath.Join(transcriptsDir, convID+".json")

	var data transcriptData
	cached := false
	if raw, readErr := os.ReadFile(cachePath); readErr == nil {
		if jsonErr := json.Unmarshal(raw, &data); jsonErr == nil && len(data.Segments) > 0 {
			cached = true
		}
	}

	if !cached {
		pythonBin := findPythonBinary(opts.PythonPath)
		if pythonBin == "" {
			return nil, fmt.Errorf("python interpreter with whisper/whisperx not found")
		}

		scriptPath := findTranscribeScript()
		if scriptPath == "" {
			return nil, fmt.Errorf("transcribe_audio.py script not found")
		}

		cmd := exec.CommandContext(ctx, pythonBin, scriptPath, "--input", filePath, "--output", cachePath)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if runErr := cmd.Run(); runErr != nil {
			return nil, fmt.Errorf("transcription failed for %s: %w (%s)", baseName, runErr, stderr.String())
		}

		raw, readErr := os.ReadFile(cachePath)
		if readErr != nil {
			return nil, fmt.Errorf("read transcript %s: %w", cachePath, readErr)
		}
		if jsonErr := json.Unmarshal(raw, &data); jsonErr != nil {
			return nil, fmt.Errorf("parse transcript %s: %w", cachePath, jsonErr)
		}
	}

	if len(data.Segments) == 0 {
		return nil, nil
	}

	// Group segments into logical chunks (~500-900 chars)
	turns := groupSegmentsIntoTurns(data.Segments, 700)
	var records []MessageRecord

	for idx, turn := range turns {
		heading := fmt.Sprintf("[%s - %s to %s]", docTitle, formatSeconds(turn.Start), formatSeconds(turn.End))
		content := fmt.Sprintf("%s\n\n%s", heading, turn.Text)

		records = append(records, MessageRecord{
			ConversationID:         convID,
			Timestamp:              timestamp,
			Speaker:                speaker,
			Text:                   content,
			SourceType:             sourceType,
			SourceTitle:            docTitle,
			Project:                opts.Project,
			MemoryKind:             KindResearchNote,
			Tags:                   []string{"audio", "transcript", category, fmt.Sprintf("chunk-%d", idx+1)},
			AttachmentFilename:     baseName,
			AttachmentMimeType:     mimeType,
			AttachmentCategory:     category,
			AttachmentSourceSystem: "folder_import",
			AIProvider:             opts.AIProvider,
		})
	}

	return records, nil
}

type audioTurn struct {
	Start float64
	End   float64
	Text  string
}

func groupSegmentsIntoTurns(segments []transcriptSegment, targetLen int) []audioTurn {
	var turns []audioTurn
	var currentText strings.Builder
	var currentStart, currentEnd float64
	started := false

	for _, seg := range segments {
		t := strings.TrimSpace(seg.Text)
		if t == "" {
			continue
		}

		if !started {
			currentStart = seg.Start
			currentEnd = seg.End
			currentText.WriteString(t)
			started = true
			continue
		}

		if currentText.Len()+len(t)+1 >= targetLen {
			turns = append(turns, audioTurn{
				Start: currentStart,
				End:   currentEnd,
				Text:  currentText.String(),
			})
			currentStart = seg.Start
			currentEnd = seg.End
			currentText.Reset()
			currentText.WriteString(t)
		} else {
			currentEnd = seg.End
			currentText.WriteString(" ")
			currentText.WriteString(t)
		}
	}

	if currentText.Len() > 0 {
		turns = append(turns, audioTurn{
			Start: currentStart,
			End:   currentEnd,
			Text:  currentText.String(),
		})
	}

	return turns
}

func formatSeconds(sec float64) string {
	totalSec := int(sec)
	m := totalSec / 60
	s := totalSec % 60
	h := m / 60
	m = m % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func findPythonBinary(explicit string) string {
	if explicit != "" {
		if _, err := os.Stat(explicit); err == nil {
			return explicit
		}
	}
	candidates := []string{
		"/home/dev/.openclaw/workspace/Transcripto/venv/bin/python",
		"/home/dev/frontpocket/venv/bin/python3",
		"/usr/bin/python3",
		"python3",
	}
	for _, c := range candidates {
		if path, err := exec.LookPath(c); err == nil {
			return path
		}
	}
	return ""
}

func findTranscribeScript() string {
	candidates := []string{
		"scripts/transcribe_audio.py",
		"../scripts/transcribe_audio.py",
		"../../scripts/transcribe_audio.py",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			if abs, err := filepath.Abs(c); err == nil {
				return abs
			}
			return c
		}
	}
	return ""
}
