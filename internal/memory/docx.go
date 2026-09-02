package memory

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DocxSection represents a logical section in a Word document.
type DocxSection struct {
	Heading string
	Text    string
}

// ParseDocxFile extracts structured text sections from a .docx file.
func ParseDocxFile(path string) ([]DocxSection, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open docx zip %s: %w", path, err)
	}
	defer r.Close()

	var docFile *zip.File
	for _, f := range r.File {
		if f.Name == "word/document.xml" {
			docFile = f
			break
		}
	}
	if docFile == nil {
		return nil, fmt.Errorf("word/document.xml not found in %s", path)
	}

	rc, err := docFile.Open()
	if err != nil {
		return nil, fmt.Errorf("open word/document.xml in %s: %w", path, err)
	}
	defer rc.Close()

	return parseDocxXML(rc)
}

func parseDocxXML(r io.Reader) ([]DocxSection, error) {
	decoder := xml.NewDecoder(r)

	type paragraph struct {
		isHeading bool
		text      strings.Builder
	}

	var paragraphs []paragraph
	var currentP *paragraph
	inText := false

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("parse docx xml: %w", err)
		}

		switch elem := token.(type) {
		case xml.StartElement:
			switch elem.Name.Local {
			case "p":
				currentP = &paragraph{}
			case "pStyle":
				if currentP != nil {
					for _, attr := range elem.Attr {
						if attr.Name.Local == "val" {
							val := strings.ToLower(attr.Value)
							if strings.Contains(val, "heading") || strings.Contains(val, "title") {
								currentP.isHeading = true
							}
						}
					}
				}
			case "t":
				inText = true
			case "tab":
				if currentP != nil {
					currentP.text.WriteString("\t")
				}
			case "br", "cr":
				if currentP != nil {
					currentP.text.WriteString("\n")
				}
			}

		case xml.CharData:
			if inText && currentP != nil {
				currentP.text.Write(elem)
			}

		case xml.EndElement:
			switch elem.Name.Local {
			case "t":
				inText = false
			case "p":
				if currentP != nil {
					trimmed := strings.TrimSpace(currentP.text.String())
					if trimmed != "" {
						currentP.text.Reset()
						currentP.text.WriteString(trimmed)
						paragraphs = append(paragraphs, *currentP)
					}
					currentP = nil
				}
			}
		}
	}

	if len(paragraphs) == 0 {
		return nil, nil
	}

	// Group into sections by headings
	var sections []DocxSection
	var currentSection *DocxSection

	for _, p := range paragraphs {
		text := p.text.String()
		if p.isHeading || isLikelyNumberedHeading(text) {
			if currentSection != nil && strings.TrimSpace(currentSection.Text) != "" {
				sections = append(sections, *currentSection)
			}
			currentSection = &DocxSection{
				Heading: text,
				Text:    text,
			}
		} else {
			if currentSection == nil {
				currentSection = &DocxSection{
					Heading: "Introduction",
					Text:    text,
				}
			} else {
				if currentSection.Text != "" {
					currentSection.Text += "\n\n" + text
				} else {
					currentSection.Text = text
				}
			}
		}
	}

	if currentSection != nil && strings.TrimSpace(currentSection.Text) != "" {
		sections = append(sections, *currentSection)
	}

	return sections, nil
}

// isLikelyNumberedHeading checks if a short paragraph starts with "1.", "2.1", etc.
func isLikelyNumberedHeading(s string) bool {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) > 120 {
		return false
	}
	if len(trimmed) < 3 {
		return false
	}
	// e.g. "1. Foundational Architecture" or "Section 2:"
	if strings.HasPrefix(trimmed, "Section ") || strings.HasPrefix(trimmed, "Chapter ") {
		return true
	}
	firstWord := strings.Fields(trimmed)[0]
	if strings.HasSuffix(firstWord, ".") && len(firstWord) <= 5 {
		// check if numeric
		isNum := true
		for _, r := range strings.TrimSuffix(firstWord, ".") {
			if (r < '0' || r > '9') && r != '.' {
				isNum = false
				break
			}
		}
		return isNum
	}
	return false
}

// DocxToMessageRecords converts a parsed .docx file into MessageRecords.
func DocxToMessageRecords(filePath, project, aiProvider string) ([]MessageRecord, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	sections, err := ParseDocxFile(filePath)
	if err != nil {
		return nil, err
	}

	baseName := filepath.Base(filePath)
	docTitle := strings.TrimSuffix(baseName, filepath.Ext(baseName))
	convID := sanitizeID(docTitle)
	timestamp := fi.ModTime().UTC().Format(time.RFC3339)

	var records []MessageRecord
	for i, sec := range sections {
		heading := sec.Heading
		if heading == "" {
			heading = fmt.Sprintf("Section %d", i+1)
		}
		content := sec.Text
		if !strings.HasPrefix(content, heading) {
			content = heading + "\n\n" + content
		}

		records = append(records, MessageRecord{
			ConversationID:         convID,
			Timestamp:              timestamp,
			Speaker:                "document",
			Text:                   content,
			SourceType:             "document",
			SourceTitle:            docTitle,
			Project:                project,
			MemoryKind:             KindResearchNote,
			Tags:                   []string{"document", "docx", "report"},
			AttachmentFilename:     baseName,
			AttachmentMimeType:     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			AttachmentCategory:     "file",
			AttachmentSourceSystem: "folder_import",
			AIProvider:             aiProvider,
		})
	}

	return records, nil
}

func sanitizeID(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			sb.WriteRune(r)
		} else if r == ' ' || r == '.' || r == '/' || r == ':' {
			sb.WriteRune('_')
		}
	}
	res := sb.String()
	for strings.Contains(res, "__") {
		res = strings.ReplaceAll(res, "__", "_")
	}
	res = strings.Trim(res, "_-")
	if res == "" {
		return "doc"
	}
	return res
}
