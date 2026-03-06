package channels

import (
	"strings"
	"sync"
	"time"
)

// StreamingBuffer wraps a StreamingReplyFunc to group small tokens into larger messages.
// It flushes on sentence boundaries (punc + space) or after a short inactivity period.
type StreamingBuffer struct {
	target       StreamingReplyFunc
	buffer       strings.Builder
	mu           sync.Mutex
	flushTimer   *time.Timer
	flushTimeout time.Duration
}

func NewStreamingBuffer(target StreamingReplyFunc, timeout time.Duration) *StreamingBuffer {
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	return &StreamingBuffer{
		target:       target,
		flushTimeout: timeout,
	}
}

func (b *StreamingBuffer) Push(token string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.buffer.WriteString(token)

	// Check for "natural" breakpoints: end of sentence, paragraph, or code block.
	content := b.buffer.String()
	if b.shouldFlush(content) {
		return b.flushLocked()
	}

	// Set timer for eventual flush if no more tokens come and one isn't already pending.
	// This prevents "starvation" when tokens arrive faster than the timeout.
	if b.flushTimer == nil {
		b.flushTimer = time.AfterFunc(b.flushTimeout, func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			_ = b.flushLocked()
		})
	}

	return nil
}

func (b *StreamingBuffer) shouldFlush(s string) bool {
	if s == "" {
		return false
	}

	// Flush on double newline (end of paragraph)
	if strings.HasSuffix(s, "\n\n") {
		return true
	}

	// Flush on sentence ending punctuation followed by newline or space
	// We check for common sentence endings: . ! ?
	if len(s) > 1 {
		last := s[len(s)-1]
		if last == '\n' || last == ' ' {
			prev := s[len(s)-2]
			if prev == '.' || prev == '!' || prev == '?' {
				return true
			}
		}
	}

	// Avoid extremely long messages too
	if len(s) > 1000 {
		return true
	}

	return false
}

func (b *StreamingBuffer) flushLocked() error {
	content := b.buffer.String()
	if content == "" {
		return nil
	}
	b.buffer.Reset()
	if b.flushTimer != nil {
		b.flushTimer.Stop()
		b.flushTimer = nil
	}
	return b.target(content)
}

// Done signals that no more tokens will be pushed.
func (b *StreamingBuffer) Done() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.flushLocked()
}
