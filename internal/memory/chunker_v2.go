package memory

import "strings"

// ConversationChunker groups adjacent conversation turns into context windows
// before splitting them for embedding, instead of chunking every message in
// total isolation the way the original per-message Chunker does.
//
// This fixes a problem that character-boundary fixes alone can't touch: a
// short message like "is there a resource monitor for node red?" carries
// almost no signal on its own — no evidence for classification, nothing for
// a reflection pass to work with. Paired with its neighboring turn(s) (e.g.
// the question plus the answer that follows it), the same content becomes a
// coherent, classifiable unit.
//
// This lives next to Chunker rather than replacing it: ConversationChunker
// handles turn-grouping, then delegates to a Chunker (the sentence/paragraph
// boundary-aware splitter) for any window still too large to embed as one
// piece. Point ingestion at a new QDRANT_COLLECTION value to test this
// without touching data produced by the original per-message path.
type ConversationChunker struct {
	// TextChunker splits an oversized window on sentence/paragraph
	// boundaries. Reuses the existing Chunker rather than reimplementing
	// boundary detection.
	TextChunker Chunker
	// WindowTurns is how many adjacent messages get grouped into one window
	// before splitting. 2 pairs a message with its immediate neighbor (e.g.
	// a user question with the assistant's answer). Must be >= 1; values
	// below 1 are treated as 1 (no grouping, same as per-message chunking).
	WindowTurns int
	// WindowOverlapTurns is how many turns from the end of one window repeat
	// at the start of the next, so context isn't lost at window edges.
	WindowOverlapTurns int
}

// ConversationTurn is one message in an ordered conversation, in the order it
// actually occurred.
type ConversationTurn struct {
	Speaker string
	Text    string
}

// TurnWindow is a chunked group of adjacent turns, ready for embedding.
type TurnWindow struct {
	// Text is speaker-labeled, e.g. "USER: ...\n\nASSISTANT: ...".
	Text string
	// Speakers lists distinct speakers represented, in first-appearance order.
	Speakers []string
	// TurnIndices are the original turn indices included, in order.
	TurnIndices []int
}

func (c ConversationChunker) ChunkConversation(turns []ConversationTurn) []TurnWindow {
	windowSize := c.WindowTurns
	if windowSize < 1 {
		windowSize = 1
	}
	overlap := c.WindowOverlapTurns
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= windowSize {
		overlap = windowSize - 1
	}
	step := windowSize - overlap
	if step < 1 {
		step = 1
	}

	windows := make([]TurnWindow, 0, (len(turns)/step)+1)

	for start := 0; start < len(turns); start += step {
		end := start + windowSize
		if end > len(turns) {
			end = len(turns)
		}
		group := turns[start:end]

		windowText, speakers, indices := assembleWindow(group, start)
		if windowText == "" {
			if end >= len(turns) {
				break
			}
			continue
		}

		for _, piece := range c.TextChunker.ChunkText(windowText) {
			windows = append(windows, TurnWindow{
				Text:        piece,
				Speakers:    speakers,
				TurnIndices: indices,
			})
		}

		if end >= len(turns) {
			break
		}
	}

	return windows
}

func assembleWindow(group []ConversationTurn, startIndex int) (string, []string, []int) {
	var sb strings.Builder
	speakers := make([]string, 0, len(group))
	seen := make(map[string]bool, len(group))
	indices := make([]int, 0, len(group))

	for i, turn := range group {
		text := strings.TrimSpace(turn.Text)
		if text == "" {
			continue
		}
		speaker := strings.TrimSpace(turn.Speaker)
		if speaker == "" {
			speaker = "unknown"
		}
		if !seen[speaker] {
			seen[speaker] = true
			speakers = append(speakers, speaker)
		}
		indices = append(indices, startIndex+i)

		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(strings.ToUpper(speaker))
		sb.WriteString(": ")
		sb.WriteString(text)
	}

	return strings.TrimSpace(sb.String()), speakers, indices
}
