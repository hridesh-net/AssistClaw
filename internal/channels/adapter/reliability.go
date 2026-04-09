package adapter

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RetryPolicy configures exponential backoff behavior.
type RetryPolicy struct {
	MaxAttempts   int
	BaseDelay     time.Duration
	MaxDelay      time.Duration
	JitterPercent float64
}

// CircuitBreakerPolicy configures breaker state transitions.
type CircuitBreakerPolicy struct {
	FailureThreshold int
	Cooldown         time.Duration
}

// ReliabilityConfig groups outbound reliability controls.
type ReliabilityConfig struct {
	Retry   RetryPolicy
	Breaker CircuitBreakerPolicy
	DLQPath string
}

// DefaultReliabilityConfig returns sane defaults for production usage.
func DefaultReliabilityConfig(stateDir string) ReliabilityConfig {
	return ReliabilityConfig{
		Retry: RetryPolicy{
			MaxAttempts:   5,
			BaseDelay:     250 * time.Millisecond,
			MaxDelay:      10 * time.Second,
			JitterPercent: 0.2,
		},
		Breaker: CircuitBreakerPolicy{
			FailureThreshold: 5,
			Cooldown:         30 * time.Second,
		},
		DLQPath: filepath.Join(stateDir, "channels", "dlq.ndjson"),
	}
}

type dlqRecord struct {
	FailedAt       time.Time `json:"failed_at"`
	Channel        string    `json:"channel"`
	SessionID      string    `json:"session_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Text           string    `json:"text"`
	Reason         string    `json:"reason"`
	Attempts       int       `json:"attempts"`
}

// ReliableSender wraps adapter outbound sends with retry/backoff, breaker, and DLQ.
type ReliableSender struct {
	channelName string
	sender      OutboundSender
	cfg         ReliabilityConfig

	mu              sync.Mutex
	consecutiveFail int
	breakerOpenTill time.Time
	halfOpenProbe   bool

	// test seams
	nowFn   func() time.Time
	sleepFn func(context.Context, time.Duration) error
	randFn  func() float64
}

// NewReliableSender creates a reliability wrapper around an adapter outbound sender.
func NewReliableSender(channelName string, sender OutboundSender, cfg ReliabilityConfig) *ReliableSender {
	if cfg.Retry.MaxAttempts <= 0 {
		cfg.Retry.MaxAttempts = 1
	}
	if cfg.Retry.BaseDelay <= 0 {
		cfg.Retry.BaseDelay = 100 * time.Millisecond
	}
	if cfg.Retry.MaxDelay < cfg.Retry.BaseDelay {
		cfg.Retry.MaxDelay = cfg.Retry.BaseDelay
	}
	if cfg.Breaker.FailureThreshold <= 0 {
		cfg.Breaker.FailureThreshold = 1
	}
	if cfg.Breaker.Cooldown <= 0 {
		cfg.Breaker.Cooldown = 5 * time.Second
	}
	return &ReliableSender{
		channelName: channelName,
		sender:      sender,
		cfg:         cfg,
		nowFn:       time.Now,
		sleepFn: func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		},
		randFn: rand.Float64,
	}
}

// Send executes the wrapped send with reliability controls.
func (r *ReliableSender) Send(ctx context.Context, msg OutboundMessage) (*DeliveryReceipt, error) {
	if msg.IdempotencyKey == "" {
		msg.IdempotencyKey = ensureIdempotencyKey(msg)
	}
	if caps, ok := CapabilitiesFor(r.channelName); ok {
		if !caps.Threading && (msg.ThreadRef != nil || msg.ReplyToID != "") {
			log.Printf("channels/outbound[%s]: threading unsupported, sending without thread/reply context", r.channelName)
			msg.ThreadRef = nil
			msg.ReplyToID = ""
		}
	}
	if allowed := r.beforeAttempt(); !allowed {
		err := NewChannelError(ErrorKindRetryable, "circuit breaker open", nil)
		_ = r.writeDLQ(msg, err, 0)
		return nil, err
	}

	var lastErr error
	for attempt := 1; attempt <= r.cfg.Retry.MaxAttempts; attempt++ {
		rec, err := r.sender.Send(ctx, msg)
		if err == nil {
			r.onSuccess()
			return rec, nil
		}
		lastErr = err

		if IsPermanent(err) {
			r.onFailure()
			_ = r.writeDLQ(msg, err, attempt)
			return nil, err
		}

		r.onFailure()
		if attempt >= r.cfg.Retry.MaxAttempts {
			break
		}
		delay := r.backoff(attempt)
		if sleepErr := r.sleepFn(ctx, delay); sleepErr != nil {
			return nil, NewChannelError(ErrorKindRetryable, "retry cancelled", sleepErr)
		}
	}

	_ = r.writeDLQ(msg, lastErr, r.cfg.Retry.MaxAttempts)
	return nil, lastErr
}

func ensureIdempotencyKey(msg OutboundMessage) string {
	h := sha1.Sum([]byte(msg.SessionID + "\n" + msg.Text + "\n" + time.Now().UTC().Format(time.RFC3339Nano)))
	return "ac-" + hex.EncodeToString(h[:8])
}

func (r *ReliableSender) backoff(attempt int) time.Duration {
	n := float64(r.cfg.Retry.BaseDelay) * math.Pow(2, float64(attempt-1))
	if n > float64(r.cfg.Retry.MaxDelay) {
		n = float64(r.cfg.Retry.MaxDelay)
	}
	delay := time.Duration(n)
	j := r.cfg.Retry.JitterPercent
	if j <= 0 {
		return delay
	}
	factor := (1 - j) + (2*j)*r.randFn() // [1-j, 1+j]
	out := time.Duration(float64(delay) * factor)
	if out < 0 {
		return 0
	}
	return out
}

func (r *ReliableSender) beforeAttempt() bool {
	now := r.nowFn()
	r.mu.Lock()
	defer r.mu.Unlock()
	if now.Before(r.breakerOpenTill) {
		return false
	}
	// Transition to half-open after cooldown: allow a single probe.
	if !r.breakerOpenTill.IsZero() && now.After(r.breakerOpenTill) {
		if r.halfOpenProbe {
			return false
		}
		r.halfOpenProbe = true
		log.Printf("channels/outbound[%s]: circuit breaker HALF-OPEN", r.channelName)
	}
	return true
}

func (r *ReliableSender) onSuccess() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.breakerOpenTill.IsZero() && r.consecutiveFail == 0 && !r.halfOpenProbe {
		return
	}
	if r.halfOpenProbe || !r.breakerOpenTill.IsZero() {
		log.Printf("channels/outbound[%s]: circuit breaker CLOSED", r.channelName)
	}
	r.consecutiveFail = 0
	r.breakerOpenTill = time.Time{}
	r.halfOpenProbe = false
}

func (r *ReliableSender) onFailure() {
	now := r.nowFn()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.consecutiveFail++
	// In half-open, a single failure re-opens immediately.
	if r.halfOpenProbe {
		r.breakerOpenTill = now.Add(r.cfg.Breaker.Cooldown)
		r.halfOpenProbe = false
		log.Printf("channels/outbound[%s]: circuit breaker OPEN (half-open probe failed)", r.channelName)
		return
	}
	if r.consecutiveFail >= r.cfg.Breaker.FailureThreshold {
		r.breakerOpenTill = now.Add(r.cfg.Breaker.Cooldown)
		r.consecutiveFail = 0
		log.Printf("channels/outbound[%s]: circuit breaker OPEN", r.channelName)
	}
}

func (r *ReliableSender) writeDLQ(msg OutboundMessage, err error, attempts int) error {
	if r.cfg.DLQPath == "" {
		return nil
	}
	rec := dlqRecord{
		FailedAt:       r.nowFn().UTC(),
		Channel:        r.channelName,
		SessionID:      msg.SessionID,
		IdempotencyKey: msg.IdempotencyKey,
		Text:           msg.Text,
		Reason:         fmt.Sprintf("%v", err),
		Attempts:       attempts,
	}
	line, mErr := json.Marshal(rec)
	if mErr != nil {
		return mErr
	}
	if mkErr := os.MkdirAll(filepath.Dir(r.cfg.DLQPath), 0o755); mkErr != nil {
		return mkErr
	}
	f, oErr := os.OpenFile(r.cfg.DLQPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if oErr != nil {
		return oErr
	}
	defer f.Close()
	if _, wErr := f.Write(append(line, '\n')); wErr != nil {
		return wErr
	}
	return nil
}
