package adapter

import (
	"errors"
	"fmt"
)

// ErrorKind classifies channel adapter errors for runners, retries, and metrics.
// Classify with errors.Is against [ChannelError] or the helpers below — do not match on [error].Error() strings.
type ErrorKind int

const (
	// ErrorKindUnknown is used when the kind cannot be determined.
	ErrorKindUnknown ErrorKind = iota
	// ErrorKindRetryable indicates a transient failure (network blip, 5xx, timeout).
	ErrorKindRetryable
	// ErrorKindPermanent indicates a failure that should not be retried without changing input or config.
	ErrorKindPermanent
	// ErrorKindRateLimited indicates the remote side asked the client to back off (429, flood wait).
	ErrorKindRateLimited
)

// ChannelError is the canonical adapter error type. Use [NewChannelError] or sentinels
// [ErrRetryable], [ErrPermanent], [ErrRateLimited] with fmt.Errorf wrapping.
type ChannelError struct {
	Kind ErrorKind
	Msg  string
	Err  error
}

func (e *ChannelError) Error() string {
	if e == nil {
		return ""
	}
	if e.Msg != "" && e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Msg, e.Err)
	}
	if e.Msg != "" {
		return e.Msg
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "channel adapter error"
}

func (e *ChannelError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// NewChannelError builds a classified adapter error.
func NewChannelError(kind ErrorKind, msg string, err error) *ChannelError {
	return &ChannelError{Kind: kind, Msg: msg, Err: err}
}

// Sentinel errors for wrapping with fmt.Errorf("%w", ...).
var (
	ErrRetryable   = NewChannelError(ErrorKindRetryable, "retryable", nil)
	ErrPermanent   = NewChannelError(ErrorKindPermanent, "permanent", nil)
	ErrRateLimited = NewChannelError(ErrorKindRateLimited, "rate limited", nil)
)

// KindOf returns the [ErrorKind] for err, or [ErrorKindUnknown].
func KindOf(err error) ErrorKind {
	if err == nil {
		return ErrorKindUnknown
	}
	var ce *ChannelError
	if errors.As(err, &ce) {
		return ce.Kind
	}
	return ErrorKindUnknown
}

// IsRetryable reports whether err should be retried with backoff.
func IsRetryable(err error) bool {
	return KindOf(err) == ErrorKindRetryable
}

// IsPermanent reports whether err should not be retried without changing state.
func IsPermanent(err error) bool {
	return KindOf(err) == ErrorKindPermanent
}

// IsRateLimited reports whether err indicates rate limiting / backoff required.
func IsRateLimited(err error) bool {
	return KindOf(err) == ErrorKindRateLimited
}
