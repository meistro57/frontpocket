package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LibraryFileEntry carries the subset of library_files.json metadata needed
// to resolve a "Library" persistent-file reference to real bytes on disk.
type LibraryFileEntry struct {
	FileID                    string
	FileName                  string
	FileExtension             string
	MimeType                  string
	Category                  string
	OriginationConversationID string
	OriginationMessageID      string
	OriginationThreadID       string
}

type libraryFileRecord struct {
	FileID                    string `json:"file_id"`
	FileName                  string `json:"file_name"`
	FileExtension             string `json:"file_extension"`
	MimeType                  string `json:"mime_type"`
	LibraryFileCategory       string `json:"library_file_category"`
	OriginationConversationID string `json:"origination_conversation_id"`
	OriginationMessageID      string `json:"origination_message_id"`
	OriginationThreadID       string `json:"origination_thread_id"`
}

// ResolvedAttachment is the outcome of resolving an attachment reference
// (e.g. "file-service://file-abc123" or "sediment://file_abc123") against
// the export's asset/library mapping files, plus locating the actual bytes
// on disk.
type ResolvedAttachment struct {
	Reference    string
	Filename     string
	MimeType     string
	Category     string
	DiskPath     string
	SourceSystem string
}

const (
	AttachmentSourceAssetFileNames = "asset_file_names"
	AttachmentSourceLibraryFiles   = "library_files"
	AttachmentSourceUnresolved     = "unresolved"
)

// LoadAssetFileNames reads conversation_asset_file_names.json and returns a
// map from reference ID (e.g. "file-15GcoL75t6eDcSQs6aLGnE.dat", the key
// shape the export actually uses) to the resolved original filename. Returns
// an empty map, no error, if the file doesn't exist — this mapping file is
// optional, same pattern as Phase 1's LoadShareSignals/LoadFeedbackSignals.
func LoadAssetFileNames(rootDir string) (map[string]string, error) {
	path := filepath.Join(rootDir, "conversation_asset_file_names.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, err
	}

	var raw map[string]string
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("parse conversation_asset_file_names.json: %w", err)
	}

	out := make(map[string]string, len(raw))
	for key, value := range raw {
		trimmedKey := strings.TrimSpace(key)
		trimmedValue := strings.TrimSpace(value)
		if trimmedKey == "" || trimmedValue == "" {
			continue
		}
		out[trimmedKey] = trimmedValue
	}
	return out, nil
}

// LoadLibraryFiles reads library_files.json and returns a map from file ID
// to a LibraryFileEntry with mime type, category, and origination metadata.
// Same optional-file handling as LoadAssetFileNames.
func LoadLibraryFiles(rootDir string) (map[string]LibraryFileEntry, error) {
	path := filepath.Join(rootDir, "library_files.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]LibraryFileEntry{}, nil
		}
		return nil, err
	}

	var rows []libraryFileRecord
	if err := json.Unmarshal(payload, &rows); err != nil {
		return nil, fmt.Errorf("parse library_files.json: %w", err)
	}

	out := make(map[string]LibraryFileEntry, len(rows))
	for _, row := range rows {
		fileID := strings.TrimSpace(row.FileID)
		if fileID == "" {
			continue
		}
		out[fileID] = LibraryFileEntry{
			FileID:                    fileID,
			FileName:                  strings.TrimSpace(row.FileName),
			FileExtension:             strings.TrimSpace(row.FileExtension),
			MimeType:                  strings.TrimSpace(row.MimeType),
			Category:                  strings.TrimSpace(row.LibraryFileCategory),
			OriginationConversationID: strings.TrimSpace(row.OriginationConversationID),
			OriginationMessageID:      strings.TrimSpace(row.OriginationMessageID),
			OriginationThreadID:       strings.TrimSpace(row.OriginationThreadID),
		}
	}
	return out, nil
}

// normalizeAttachmentRef strips a "scheme://" prefix (e.g. "file-service://"
// for inline uploads, "sediment://" for Library files) so the bare
// reference ID can be looked up against either mapping file.
func normalizeAttachmentRef(ref string) string {
	trimmed := strings.TrimSpace(ref)
	if idx := strings.Index(trimmed, "://"); idx >= 0 {
		trimmed = trimmed[idx+3:]
	}
	return trimmed
}

// ResolveAttachment checks a reference ID against both mapping files (the
// dash-prefixed vs underscore-prefixed shape is an inference about which
// system a ref belongs to, not a guarantee, so both maps are checked
// regardless of ref shape) and returns the resolved filename, mime type,
// category, and full path to the .dat file on disk. Returns (nil, false) if
// unresolvable in either map, or if the .dat file doesn't actually exist at
// the expected path.
func ResolveAttachment(ref string, rootDir string, assetNames map[string]string, libraryFiles map[string]LibraryFileEntry) (*ResolvedAttachment, bool) {
	bare := normalizeAttachmentRef(ref)
	if bare == "" {
		return nil, false
	}

	if filename, ok := lookupAssetFileName(bare, assetNames); ok {
		diskPath := filepath.Join(rootDir, bare+".dat")
		if !fileExists(diskPath) {
			diskPath = filepath.Join(rootDir, bare)
			if !fileExists(diskPath) {
				return nil, false
			}
		}
		return &ResolvedAttachment{
			Reference:    ref,
			Filename:     filename,
			MimeType:     mimeTypeFromFilename(filename),
			DiskPath:     diskPath,
			SourceSystem: AttachmentSourceAssetFileNames,
		}, true
	}

	if entry, ok := libraryFiles[bare]; ok {
		filename := entry.FileName
		if filename == "" {
			filename = entry.FileID
			if entry.FileExtension != "" {
				filename = entry.FileID + "." + strings.TrimPrefix(entry.FileExtension, ".")
			}
		}
		diskPath := filepath.Join(rootDir, bare+".dat")
		if !fileExists(diskPath) {
			diskPath = filepath.Join(rootDir, bare)
			if !fileExists(diskPath) {
				return nil, false
			}
		}
		mimeType := entry.MimeType
		if mimeType == "" {
			mimeType = mimeTypeFromFilename(filename)
		}
		return &ResolvedAttachment{
			Reference:    ref,
			Filename:     filename,
			MimeType:     mimeType,
			Category:     entry.Category,
			DiskPath:     diskPath,
			SourceSystem: AttachmentSourceLibraryFiles,
		}, true
	}

	return nil, false
}

func lookupAssetFileName(bare string, assetNames map[string]string) (string, bool) {
	if filename, ok := assetNames[bare]; ok {
		return filename, true
	}
	if filename, ok := assetNames[bare+".dat"]; ok {
		return filename, true
	}
	return "", false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func mimeTypeFromFilename(filename string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
	switch ext {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "heic":
		return "image/heic"
	case "dng":
		return "image/x-adobe-dng"
	case "pdf":
		return "application/pdf"
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "mp4":
		return "video/mp4"
	default:
		return ""
	}
}
