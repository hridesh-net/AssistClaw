package cron

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"github.com/assistclaw/assistclaw/internal/agent"
	"github.com/assistclaw/assistclaw/internal/memory"
	"github.com/assistclaw/assistclaw/internal/provider"
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

	// Dependencies for spinning up agents
	provider provider.Provider
	toolReg  *agent.ToolRegistry
	memMgr   *memory.Manager
	agentCfg agent.Config
	log      *zap.Logger

	mu sync.Mutex
}

func NewDaemon(
	jobs []Job,
	p provider.Provider,
	tools *agent.ToolRegistry,
	mem *memory.Manager,
	cfg agent.Config,
	logger *zap.Logger,
) *Daemon {
	return &Daemon{
		cron:     cron.New(cron.WithSeconds()), // Standard cron with seconds precision optional
		jobs:     jobs,
		provider: p,
		toolReg:  tools,
		memMgr:   mem,
		agentCfg: cfg,
		log:      logger,
	}
}

func (d *Daemon) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, j := range d.jobs {
		job := j // Capture variable for closure
		_, err := d.cron.AddFunc(job.Schedule, func() {
			log.Printf("cron: executing job %q", job.ID)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			runner := agent.NewRunner(d.agentCfg, d.provider, d.toolReg, d.memMgr, d.log)

			// Run in background without streaming to UI
			res, err := runner.Run(ctx, job.Prompt)
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
