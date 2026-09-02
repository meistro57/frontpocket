package memory

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const SlideVisionPrompt = "Extract all readable text, titles, subtitles, bullet points, and key conceptual diagrams from this presentation slide. Provide a clear, detailed transcription and description."

// PDFToMessageRecords extracts text or visual slide transcripts from a PDF file.
func PDFToMessageRecords(ctx context.Context, filePath string, opts FolderImportOptions) ([]MessageRecord, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	baseName := filepath.Base(filePath)
	docTitle := cleanTitle(strings.TrimSuffix(baseName, filepath.Ext(baseName)))
	convID := sanitizeID(docTitle)
	timestamp := fi.ModTime().UTC().Format(time.RFC3339)

	// 1. Check if PDF has embedded text
	text, _ := extractPDFText(filePath)
	if len(strings.TrimSpace(text)) >= 100 {
		return pdfTextToRecords(text, baseName, docTitle, convID, timestamp, opts), nil
	}

	// 2. Otherwise, treat as visual slide presentation
	if opts.NoVision || opts.VisionCaptioner == nil {
		return []MessageRecord{
			{
				ConversationID:         convID,
				Timestamp:              timestamp,
				Speaker:                "presentation",
				Text:                   fmt.Sprintf("[Presentation: %s (Visual PDF slides - vision captioning skipped)]", docTitle),
				SourceType:             "presentation",
				SourceTitle:            docTitle,
				Project:                opts.Project,
				MemoryKind:             KindProjectContext,
				Tags:                   []string{"presentation", "pdf", "visual_slides"},
				AttachmentFilename:     baseName,
				AttachmentMimeType:     "application/pdf",
				AttachmentCategory:     "presentation",
				AttachmentSourceSystem: "folder_import",
				AIProvider:             opts.AIProvider,
			},
		}, nil
	}

	// Render slide pages to cache
	slidesDir := filepath.Join(opts.CacheDir, "slides", convID)
	slideImages, err := renderPDFSlides(filePath, slidesDir)
	if err != nil {
		return nil, fmt.Errorf("render slides for %s: %w", baseName, err)
	}

	var records []MessageRecord
	for idx, imgPath := range slideImages {
		pageNum := idx + 1
		slideName := fmt.Sprintf("%s - Slide %d", docTitle, pageNum)

		var caption string
		if pc, ok := opts.VisionCaptioner.(PromptCaptioner); ok {
			caption, err = pc.CaptionImageWithPrompt(ctx, ResolvedAttachment{
				DiskPath: imgPath,
				MimeType: "image/png",
				Filename: filepath.Base(imgPath),
			}, SlideVisionPrompt)
		} else {
			caption, err = opts.VisionCaptioner.CaptionImage(ctx, ResolvedAttachment{
				DiskPath: imgPath,
				MimeType: "image/png",
				Filename: filepath.Base(imgPath),
			})
		}
		if err != nil {
			caption = fmt.Sprintf("[Slide %d: vision extraction failed: %v]", pageNum, err)
		}

		content := fmt.Sprintf("[%s]\n\n%s", slideName, strings.TrimSpace(caption))

		records = append(records, MessageRecord{
			ConversationID:         convID,
			Timestamp:              timestamp,
			Speaker:                "presentation",
			Text:                   content,
			SourceType:             "presentation",
			SourceTitle:            docTitle,
			Project:                opts.Project,
			MemoryKind:             KindProjectContext,
			Tags:                   []string{"presentation", "slide", fmt.Sprintf("slide-%d", pageNum)},
			AttachmentFilename:     baseName,
			AttachmentMimeType:     "application/pdf",
			AttachmentCategory:     "presentation",
			AttachmentSourceSystem: "folder_import",
			AIProvider:             opts.AIProvider,
		})
	}

	return records, nil
}

func extractPDFText(filePath string) (string, error) {
	cmd := exec.Command("pdftotext", filePath, "-")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftotext: %w (%s)", err, stderr.String())
	}
	return stdout.String(), nil
}

func pdfTextToRecords(fullText, baseName, docTitle, convID, timestamp string, opts FolderImportOptions) []MessageRecord {
	pages := strings.Split(fullText, "\x0c") // form feed delimiter between pages
	var records []MessageRecord

	for i, p := range pages {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		pageNum := i + 1
		content := fmt.Sprintf("[%s - Page %d]\n\n%s", docTitle, pageNum, trimmed)
		records = append(records, MessageRecord{
			ConversationID:         convID,
			Timestamp:              timestamp,
			Speaker:                "document",
			Text:                   content,
			SourceType:             "document",
			SourceTitle:            docTitle,
			Project:                opts.Project,
			MemoryKind:             KindResearchNote,
			Tags:                   []string{"document", "pdf", fmt.Sprintf("page-%d", pageNum)},
			AttachmentFilename:     baseName,
			AttachmentMimeType:     "application/pdf",
			AttachmentCategory:     "file",
			AttachmentSourceSystem: "folder_import",
			AIProvider:             opts.AIProvider,
		})
	}

	if len(records) == 0 && strings.TrimSpace(fullText) != "" {
		records = append(records, MessageRecord{
			ConversationID:         convID,
			Timestamp:              timestamp,
			Speaker:                "document",
			Text:                   fmt.Sprintf("[%s]\n\n%s", docTitle, strings.TrimSpace(fullText)),
			SourceType:             "document",
			SourceTitle:            docTitle,
			Project:                opts.Project,
			MemoryKind:             KindResearchNote,
			Tags:                   []string{"document", "pdf"},
			AttachmentFilename:     baseName,
			AttachmentMimeType:     "application/pdf",
			AttachmentCategory:     "file",
			AttachmentSourceSystem: "folder_import",
			AIProvider:             opts.AIProvider,
		})
	}

	return records
}

func renderPDFSlides(filePath, outputDir string) ([]string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, err
	}

	// Check if already rendered
	existing, err := filepath.Glob(filepath.Join(outputDir, "slide-*.png"))
	if err == nil && len(existing) > 0 {
		sortSlideFiles(existing)
		return existing, nil
	}

	prefix := filepath.Join(outputDir, "slide")
	cmd := exec.Command("pdftoppm", "-png", "-r", "72", filePath, prefix)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pdftoppm failed: %w (%s)", err, stderr.String())
	}

	rendered, err := filepath.Glob(filepath.Join(outputDir, "slide-*.png"))
	if err != nil {
		return nil, err
	}
	sortSlideFiles(rendered)
	return rendered, nil
}

func sortSlideFiles(files []string) {
	sort.Slice(files, func(i, j int) bool {
		numI := extractTrailingNumber(files[i])
		numJ := extractTrailingNumber(files[j])
		if numI != numJ {
			return numI < numJ
		}
		return files[i] < files[j]
	})
}

func extractTrailingNumber(path string) int {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	parts := strings.Split(base, "-")
	if len(parts) > 1 {
		if n, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
			return n
		}
	}
	return 0
}

func cleanTitle(name string) string {
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.TrimSpace(name)
	return name
}
