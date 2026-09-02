package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type FolderImportOptions struct {
	Path            string
	Project         string
	CacheDir        string
	NoAudio         bool
	NoVision        bool
	Limit           int
	AIProvider      string
	VisionCaptioner Captioner
	CaptionCache    *CaptionCache
	PythonPath      string
	ProgressFn      func(FolderProgressEvent)
}

type FolderProgressEvent struct {
	CurrentFile  string
	FileType     string
	FilesDone    int
	FilesTotal   int
	RecordsSoFar int
	Message      string
}

type FolderImportResult struct {
	SourcePath        string
	Project           string
	FilesScanned      int
	FilesProcessed    int
	FilesSkipped      int
	DocumentsParsed   int
	SlidesParsed      int
	ImagesParsed      int
	AudiosTranscribed int
	TotalRecords      int
	Records           []MessageRecord
}

// ParseFolder scans and ingests all supported documents, slides, diagrams, and audio files in a folder.
func ParseFolder(ctx context.Context, folderPath string, opts FolderImportOptions) (*FolderImportResult, error) {
	absPath, err := filepath.Abs(folderPath)
	if err != nil {
		return nil, fmt.Errorf("resolve folder path: %w", err)
	}

	fi, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("stat folder: %w", err)
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", absPath)
	}

	if opts.CacheDir == "" {
		opts.CacheDir = ".frontpocket_cache"
	}
	if err := os.MkdirAll(opts.CacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create cache dir %s: %w", opts.CacheDir, err)
	}

	if opts.Project == "" {
		opts.Project = cleanTitle(filepath.Base(absPath))
	}
	if opts.AIProvider == "" {
		opts.AIProvider = "frontpocket"
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	var targetFiles []string
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if isSupportedExtension(ext) {
			targetFiles = append(targetFiles, filepath.Join(absPath, e.Name()))
		}
	}
	sort.Strings(targetFiles)

	result := &FolderImportResult{
		SourcePath:   absPath,
		Project:      opts.Project,
		FilesScanned: len(entries),
	}

	if opts.Limit > 0 && len(targetFiles) > opts.Limit {
		targetFiles = targetFiles[:opts.Limit]
	}

	totalTargets := len(targetFiles)
	for i, file := range targetFiles {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		baseName := filepath.Base(file)
		ext := strings.ToLower(filepath.Ext(baseName))
		fileType := describeFileType(ext)

		if opts.ProgressFn != nil {
			opts.ProgressFn(FolderProgressEvent{
				CurrentFile:  baseName,
				FileType:     fileType,
				FilesDone:    i,
				FilesTotal:   totalTargets,
				RecordsSoFar: len(result.Records),
				Message:      fmt.Sprintf("Processing %s (%s)", baseName, fileType),
			})
		}

		var fileRecords []MessageRecord
		var parseErr error

		switch ext {
		case ".docx":
			fileRecords, parseErr = DocxToMessageRecords(file, opts.Project, opts.AIProvider)
			if parseErr == nil {
				result.DocumentsParsed++
			}
		case ".pdf":
			fileRecords, parseErr = PDFToMessageRecords(ctx, file, opts)
			if parseErr == nil {
				if opts.NoVision {
					result.DocumentsParsed++
				} else {
					result.SlidesParsed += len(fileRecords)
				}
			}
		case ".png", ".jpg", ".jpeg", ".webp":
			fileRecords, parseErr = ImageToMessageRecords(ctx, file, opts)
			if parseErr == nil {
				result.ImagesParsed++
			}
		case ".m4a", ".mp3", ".wav", ".mp4":
			if opts.NoAudio {
				result.FilesSkipped++
				continue
			}
			fileRecords, parseErr = AudioToMessageRecords(ctx, file, opts)
			if parseErr == nil {
				result.AudiosTranscribed++
			}
		case ".md", ".txt":
			fileRecords, parseErr = textFileToMessageRecords(file, opts)
			if parseErr == nil {
				result.DocumentsParsed++
			}
		case ".jsonl":
			data, readErr := os.ReadFile(file)
			if readErr == nil {
				fileRecords, parseErr = ParseJSONL(string(data))
				if parseErr == nil {
					result.DocumentsParsed++
				}
			} else {
				parseErr = readErr
			}
		default:
			result.FilesSkipped++
			continue
		}

		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to parse %s: %v\n", baseName, parseErr)
			result.FilesSkipped++
			continue
		}

		result.FilesProcessed++
		result.Records = append(result.Records, fileRecords...)
	}

	result.TotalRecords = len(result.Records)

	if opts.ProgressFn != nil {
		opts.ProgressFn(FolderProgressEvent{
			CurrentFile:  "",
			FileType:     "",
			FilesDone:    totalTargets,
			FilesTotal:   totalTargets,
			RecordsSoFar: result.TotalRecords,
			Message:      "Scan and parsing complete",
		})
	}

	return result, nil
}

func textFileToMessageRecords(filePath string, opts FolderImportOptions) ([]MessageRecord, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	content := strings.TrimSpace(string(contentBytes))
	if content == "" {
		return nil, nil
	}

	baseName := filepath.Base(filePath)
	docTitle := cleanTitle(strings.TrimSuffix(baseName, filepath.Ext(baseName)))
	convID := sanitizeID(docTitle)
	timestamp := fi.ModTime().UTC().Format(time.RFC3339)

	return []MessageRecord{
		{
			ConversationID:         convID,
			Timestamp:              timestamp,
			Speaker:                "document",
			Text:                   fmt.Sprintf("[%s]\n\n%s", docTitle, content),
			SourceType:             "document",
			SourceTitle:            docTitle,
			Project:                opts.Project,
			MemoryKind:             KindResearchNote,
			Tags:                   []string{"document", "text"},
			AttachmentFilename:     baseName,
			AttachmentMimeType:     "text/plain",
			AttachmentCategory:     "file",
			AttachmentSourceSystem: "folder_import",
			AIProvider:             opts.AIProvider,
		},
	}, nil
}

func isSupportedExtension(ext string) bool {
	switch ext {
	case ".docx", ".pdf", ".png", ".jpg", ".jpeg", ".webp", ".m4a", ".mp3", ".wav", ".mp4", ".md", ".txt", ".jsonl":
		return true
	default:
		return false
	}
}

func describeFileType(ext string) string {
	switch ext {
	case ".docx":
		return "Word Document"
	case ".pdf":
		return "PDF Document / Presentation"
	case ".png", ".jpg", ".jpeg", ".webp":
		return "Image / Diagram"
	case ".m4a", ".mp3", ".wav":
		return "Audio Overview"
	case ".mp4":
		return "Video Presentation"
	case ".md", ".txt":
		return "Text Note"
	case ".jsonl":
		return "JSONL Memory"
	default:
		return "File"
	}
}
