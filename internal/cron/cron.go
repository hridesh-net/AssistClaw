package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"github.com/assistclaw/assistclaw/internal/agent"
	"github.com/assistclaw/assistclaw/internal/memory"
)

// Job defines a cron task.
type Job struct {
	ID       string `yaml:"id"`
	Schedule string `yaml:"schedule"`
	Prompt   string `yaml:"prompt"`
	// MaxRetries is the number of times the daemon will retry a transient
	// failure with exponential backoff (default 0 = no retry). Retries are
	// suppressed when the context deadline is hit.
	MaxRetries int `yaml:"max_retries,omitempty"`
}

// FailureNotifier delivers a cron-job failure summary to the user. The
// caller wires this to Telegram/Discord/Slack/Email or a no-op for tests.
type FailureNotifier func(ctx context.Context, jobID, summary string)

// Daemon handles background scheduled execution of agent loops.
type Daemon struct {
	cron *cron.Cron
	jobs []Job

	// template is the fully configured gateway runner (catalog, security, skills).
	template        *agent.Runner
	log             *zap.Logger
	persistencePath string

	// onFailure is called for every job that exhausts its retries. Nil =
	// log-only (the existing pre-13.10 behaviour).
	onFailure FailureNotifier

	// timeout is the per-job hard deadline. Defaults to 10 minutes.
	timeout time.Duration

	mu sync.Mutex
}

// NewDaemon schedules jobs using runners cloned from template so cron jobs get the
// same tool catalog, guardrail, and audit behavior as interactive sessions.
func NewDaemon(
	jobs []Job,
	template *agent.Runner,
	logger *zap.Logger,
	persistencePath string,
) *Daemon {
	return &Daemon{
		cron:            cron.New(cron.WithSeconds()),
		jobs:            jobs,
		template:        template,
		log:             logger,
		persistencePath: persistencePath,
		timeout:         10 * time.Minute,
	}
}

// WithFailureNotifier attaches a notifier that fires when a job's final
// attempt (after retries) fails. Returns the Daemon for chaining.
func (d *Daemon) WithFailureNotifier(n FailureNotifier) *Daemon {
	d.onFailure = n
	return d
}

// WithTimeout overrides the per-job context deadline. Zero or negative
// values are ignored.
func (d *Daemon) WithTimeout(t time.Duration) *Daemon {
	if t > 0 {
		d.timeout = t
	}
	return d
}

func (d *Daemon) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	allJobs := append([]Job{}, d.jobs...)

	// Load persistent jobs
	if d.persistencePath != "" {
		if data, err := os.ReadFile(d.persistencePath); err == nil {
			var persisted []Job
			if err := json.Unmarshal(data, &persisted); err == nil {
				allJobs = append(allJobs, persisted...)
			}
		}
	}

	for _, j := range allJobs {
		job := j // capture for closure
		_, err := d.cron.AddFunc(job.Schedule, func() { d.runJob(job) })
		if err != nil {
			d.log.Error("cron: failed to schedule job", zap.String("id", job.ID), zap.Error(err))
		} else {
			d.log.Info("cron: scheduled job", zap.String("id", job.ID), zap.String("schedule", job.Schedule))
		}
	}

	d.cron.Start()
	return nil
}

// runJob executes a single cron job with retry/backoff and failure
// notification. It runs inside a goroutine spawned by robfig/cron.
func (d *Daemon) runJob(job Job) {
	log.Printf("cron: executing job %q", job.ID)
	if d.template == nil {
		d.log.Error("cron: no runner template", zap.String("id", job.ID))
		return
	}

	maxAttempts := job.MaxRetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error
	var lastIter int
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
		sid := "cron:" + job.ID
		runner := d.template.WithSession(sid)
		res, err := runner.Run(ctx, memory.Message{
			ID:        uuid.New().String(),
			SessionID: sid,
			Role:      memory.RoleUser,
			Content:   job.Prompt,
			CreatedAt: time.Now(),
		})
		cancel()
		if err == nil {
			d.log.Info("cron: job finished",
				zap.String("id", job.ID),
				zap.Int("iterations", res.Iterations),
				zap.Int("attempt", attempt),
			)
			return
		}

		lastErr = err
		if res != nil {
			lastIter = res.Iterations
		}
		// Don't retry past deadlines — retrying immediately would just hit
		// the same timeout. Same for an explicit ctx cancel from the host.
		if ctx.Err() == context.DeadlineExceeded || ctx.Err() == context.Canceled {
			break
		}
		if attempt == maxAttempts {
			break
		}
		// Exponential backoff: 1s, 2s, 4s, 8s, …, capped at 60s.
		delay := time.Duration(1<<uint(attempt-1)) * time.Second
		if delay > 60*time.Second {
			delay = 60 * time.Second
		}
		d.log.Warn("cron: job attempt failed, will retry",
			zap.String("id", job.ID),
			zap.Int("attempt", attempt),
			zap.Duration("retry_in", delay),
			zap.Error(err),
		)
		time.Sleep(delay)
	}

	d.log.Error("cron: job failed after retries",
		zap.String("id", job.ID),
		zap.Int("attempts", maxAttempts),
		zap.Int("iterations_last_attempt", lastIter),
		zap.Error(lastErr),
	)
	if d.onFailure != nil {
		summary := fmt.Sprintf("cron job %q failed after %d attempt(s): %v",
			job.ID, maxAttempts, lastErr)
		notifyCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d.onFailure(notifyCtx, job.ID, summary)
	}
}

func (d *Daemon) Stop() {
	d.cron.Stop()
}
