package memory

import "strings"

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

	step := c.Size - overlap
	if step <= 0 {
		step = c.Size
	}

	chunks := make([]string, 0, (len(runes)/step)+1)
	for start := 0; start < len(runes); start += step {
		end := start + c.Size
		if end > len(runes) {
			end = len(runes)
		}

		piece := strings.TrimSpace(string(runes[start:end]))
		if piece == "" {
			continue
		}

		if c.MinSize > 0 && len([]rune(piece)) < c.MinSize {
			if len(chunks) == 0 {
				chunks = append(chunks, piece)
			} else {
				chunks[len(chunks)-1] = strings.TrimSpace(chunks[len(chunks)-1] + "\n" + piece)
			}
			continue
		}

		chunks = append(chunks, piece)
	}

	return chunks
}
