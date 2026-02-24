package memory

import (
	"testing"
)

func TestChunkMarkdown(t *testing.T) {
	content := `Line 1
Line 2
Line 3
Line 4
Line 5`

	// Max tokens = 2 (approx 8 chars), overlap = 1 (approx 4 chars)
	// "Line 1\n" is 7 chars.
	// Chunk 1: Line 1 (7 chars)
	// Chunk 2: Line 1 (overlap), Line 2
	chunks := ChunkMarkdown(content, 2, 1)

	if len(chunks) == 0 {
		t.Fatal("expected chunks, got none")
	}

	for i, c := range chunks {
		t.Logf("Chunk %d: lines %d-%d: %q", i, c.StartLine, c.EndLine, c.Content)
	}

	// Basic property checks
	if chunks[0].StartLine != 1 {
		t.Errorf("expected chunk 0 to start at line 1, got %d", chunks[0].StartLine)
	}
}

func TestChunkMarkdownLarge(t *testing.T) {
	content := `### Header
This is a paragraph with some content.
It spans multiple lines.

* List item 1
* List item 2`

	chunks := ChunkMarkdown(content, 10, 2)
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}

	for _, c := range chunks {
		if c.Content == "" {
			t.Error("chunk content should not be empty")
		}
		if c.StartLine <= 0 || c.EndLine < c.StartLine {
			t.Errorf("invalid line range: %d-%d", c.StartLine, c.EndLine)
		}
	}
}
