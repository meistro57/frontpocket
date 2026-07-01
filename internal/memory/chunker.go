package memory

import (
	"strings"
	"unicode"
)

// Chunker splits text into overlapping windows sized for embedding.
//
// It prefers to break on a paragraph boundary, then a sentence ending, then
// plain whitespace, and only falls back to a hard character cut when none of
// those exist within the search window (e.g. unbroken text, code, or ids).
// This avoids the mid-word/mid-sentence fragments that a pure character-count
// slicer produces, which otherwise degrade embedding quality and confuse
// downstream reflection prompts.
type Chunker struct {
	Size    int
	Overlap int
	MinSize int
}

func (c Chunker) ChunkText(text string) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}

	runes := []rune(trimmed)
	if c.Size <= 0 || len(runes) <= c.Size {
		return []string{trimmed}
	}

	overlap := c.Overlap
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= c.Size {
		overlap = c.Size / 2
	}

	chunks := make([]string, 0, (len(runes)/(c.Size-overlap))+1)
	start := 0

	for start < len(runes) {
		end := start + c.Size
		if end >= len(runes) {
			end = len(runes)
		} else {
			end = findBreakPoint(runes, start, end)
			if end <= start {
				end = start + c.Size
				if end > len(runes) {
					end = len(runes)
				}
			}
		}

		piece := strings.TrimSpace(string(runes[start:end]))

		if piece != "" {
			if c.MinSize > 0 && len([]rune(piece)) < c.MinSize && len(chunks) > 0 {
				chunks[len(chunks)-1] = strings.TrimSpace(chunks[len(chunks)-1] + "\n" + piece)
			} else {
				chunks = append(chunks, piece)
			}
		}

		if end >= len(runes) {
			break
		}

		next := end - overlap
		if next <= start {
			next = end
		}
		start = next
	}

	return chunks
}

// findBreakPoint looks backward from end for a natural boundary: a paragraph
// break (\n\n) first, then a sentence ending followed by whitespace, checked
// across the whole [start, end) window so a sentence boundary is preferred
// even if it falls earlier than the last few characters. Plain whitespace is
// only considered within the last 20% of the window, to avoid producing an
// overly short chunk when no punctuation boundary exists nearby. Returns end
// unchanged (a hard cut) if nothing suitable exists.
func findBreakPoint(runes []rune, start, end int) int {
	for i := end - 1; i > start; i-- {
		if runes[i] == '\n' && i > start && runes[i-1] == '\n' {
			return i + 1
		}
	}
	for i := end - 1; i > start; i-- {
		if isSentenceEnder(runes[i]) && i+1 < len(runes) && unicode.IsSpace(runes[i+1]) {
			return i + 1
		}
	}

	searchWindow := (end - start) / 5
	if searchWindow < 1 {
		return end
	}
	minBound := end - searchWindow
	if minBound < start {
		minBound = start
	}
	for i := end - 1; i > minBound; i-- {
		if unicode.IsSpace(runes[i]) {
			return i
		}
	}
	return end
}

func isSentenceEnder(r rune) bool {
	return r == '.' || r == '!' || r == '?'
}
