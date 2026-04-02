package cron

import (
	"context"
	"encoding/json"
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
}

// Daemon handles background scheduled execution of agent loops.
type Daemon struct {
	cron *cron.Cron
	jobs []Job

	// template is the fully configured gateway runner (catalog, security, skills).
	template        *agent.Runner
	log             *zap.Logger
	persistencePath string

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
	}
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
		job := j // Capture variable for closure
		_, err := d.cron.AddFunc(job.Schedule, func() {
			log.Printf("cron: executing job %q", job.ID)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			if d.template == nil {
				d.log.Error("cron: no runner template", zap.String("id", job.ID))
				return
			}
			sid := "cron:" + job.ID
			runner := d.template.WithSession(sid)

			// Run in background without streaming to UI
			res, err := runner.Run(ctx, memory.Message{
				ID:        uuid.New().String(),
				SessionID: sid,
				Role:      memory.RoleUser,
				Content:   job.Prompt,
				CreatedAt: time.Now(),
			})
			if err != nil {
				d.log.Error("cron: job failed", zap.String("id", job.ID), zap.Error(err))
			} else {
				d.log.Info("cron: job finished", zap.String("id", job.ID), zap.Int("iterations", res.Iterations))
			}
		})
		if err != nil {
			d.log.Error("cron: failed to schedule job", zap.String("id", job.ID), zap.Error(err))
		} else {
			d.log.Info("cron: scheduled job", zap.String("id", job.ID), zap.String("schedule", job.Schedule))
		}
	}

	d.cron.Start()
	return nil
}

func (d *Daemon) Stop() {
	d.cron.Stop()
}
