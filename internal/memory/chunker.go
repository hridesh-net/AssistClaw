package memory

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// Chunk represents a segment of a document.
type Chunk struct {
	StartLine int
	EndLine   int
	Content   string
	Hash      string
}

// ChunkMarkdown splits markdown content into overlapping chunks.
// It uses a character-based heuristic where tokens are roughly 4 characters.
func ChunkMarkdown(content string, maxTokens int, overlapTokens int) []Chunk {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return nil
	}

	maxChars := maxTokens * 4
	if maxChars < 32 {
		maxChars = 32
	}
	overlapChars := overlapTokens * 4

	var chunks []Chunk
	var current []lineEntry
	currentChars := 0

	flush := func() {
		if len(current) == 0 {
			return
		}
		var sb strings.Builder
		for i, entry := range current {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(entry.line)
		}
		text := sb.String()
		chunks = append(chunks, Chunk{
			StartLine: current[0].lineNo,
			EndLine:   current[len(current)-1].lineNo,
			Content:   text,
			Hash:      fmt.Sprintf("%x", sha256.Sum256([]byte(text))),
		})
	}

	carryOverlap := func() {
		if overlapChars <= 0 || len(current) == 0 {
			current = nil
			currentChars = 0
			return
		}
		acc := 0
		var kept []lineEntry
		for i := len(current) - 1; i >= 0; i-- {
			entry := current[i]
			acc += len(entry.line) + 1
			kept = append([]lineEntry{entry}, kept...)
			if acc >= overlapChars {
				break
			}
		}
		current = kept
		currentChars = acc
	}

	for i, line := range lines {
		lineNo := i + 1

		// If a single line is longer than maxChars, we split it.
		segments := splitLine(line, maxChars)

		for _, segment := range segments {
			lineSize := len(segment) + 1
			if currentChars+lineSize > maxChars && len(current) > 0 {
				flush()
				carryOverlap()
			}
			current = append(current, lineEntry{line: segment, lineNo: lineNo})
			currentChars += lineSize
		}
	}

	flush()
	return chunks
}

type lineEntry struct {
	line   string
	lineNo int
}

func splitLine(line string, maxChars int) []string {
	if len(line) == 0 {
		return []string{""}
	}
	if len(line) <= maxChars {
		return []string{line}
	}
	var segments []string
	for start := 0; start < len(line); start += maxChars {
		end := start + maxChars
		if end > len(line) {
			end = len(line)
		}
		segments = append(segments, line[start:end])
	}
	return segments
}
