package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DiagramVisionPrompt = "Provide a comprehensive, detailed transcription and description of this diagram or image, including all visible labels, titles, formulas, node connections, flow, and conceptual meaning. Be concrete and specific."

// ImageToMessageRecords converts an image or diagram into a MessageRecord with vision transcription.
func ImageToMessageRecords(ctx context.Context, filePath string, opts FolderImportOptions) ([]MessageRecord, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	baseName := filepath.Base(filePath)
	ext := strings.ToLower(filepath.Ext(baseName))
	docTitle := cleanTitle(strings.TrimSuffix(baseName, filepath.Ext(baseName)))
	convID := sanitizeID(docTitle)
	timestamp := fi.ModTime().UTC().Format(time.RFC3339)

	mimeType := "image/png"
	switch ext {
	case ".jpg", ".jpeg":
		mimeType = "image/jpeg"
	case ".webp":
		mimeType = "image/webp"
	case ".gif":
		mimeType = "image/gif"
	}

	if opts.NoVision || opts.VisionCaptioner == nil {
		return []MessageRecord{
			{
				ConversationID:         convID,
				Timestamp:              timestamp,
				Speaker:                "diagram",
				Text:                   fmt.Sprintf("[Diagram: %s (Vision captioning skipped)]", docTitle),
				SourceType:             "diagram",
				SourceTitle:            docTitle,
				Project:                opts.Project,
				MemoryKind:             KindCreativeArtifact,
				Tags:                   []string{"diagram", "image", strings.TrimPrefix(ext, ".")},
				AttachmentFilename:     baseName,
				AttachmentMimeType:     mimeType,
				AttachmentCategory:     "image",
				AttachmentSourceSystem: "folder_import",
				AIProvider:             opts.AIProvider,
			},
		}, nil
	}

	var caption string
	if pc, ok := opts.VisionCaptioner.(PromptCaptioner); ok {
		caption, err = pc.CaptionImageWithPrompt(ctx, ResolvedAttachment{
			DiskPath: filePath,
			MimeType: mimeType,
			Filename: baseName,
		}, DiagramVisionPrompt)
	} else {
		caption, err = opts.VisionCaptioner.CaptionImage(ctx, ResolvedAttachment{
			DiskPath: filePath,
			MimeType: mimeType,
			Filename: baseName,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("caption image %s: %w", baseName, err)
	}

	content := fmt.Sprintf("[Diagram: %s]\n\n%s", docTitle, strings.TrimSpace(caption))

	return []MessageRecord{
		{
			ConversationID:         convID,
			Timestamp:              timestamp,
			Speaker:                "diagram",
			Text:                   content,
			SourceType:             "diagram",
			SourceTitle:            docTitle,
			Project:                opts.Project,
			MemoryKind:             KindCreativeArtifact,
			Tags:                   []string{"diagram", "image", "architecture"},
			AttachmentFilename:     baseName,
			AttachmentMimeType:     mimeType,
			AttachmentCategory:     "image",
			AttachmentSourceSystem: "folder_import",
			AIProvider:             opts.AIProvider,
		},
	}, nil
}
