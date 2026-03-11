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
	provider        provider.Provider
	toolReg         *agent.ToolRegistry
	memMgr          *memory.Manager
	agentCfg        agent.Config
	log             *zap.Logger
	workspaceDir    string
	persistencePath string

	mu sync.Mutex
}

func NewDaemon(
	jobs []Job,
	p provider.Provider,
	tools *agent.ToolRegistry,
	mem *memory.Manager,
	cfg agent.Config,
	logger *zap.Logger,
	workspaceDir string,
	persistencePath string,
) *Daemon {
	return &Daemon{
		cron:            cron.New(cron.WithSeconds()), // Standard cron with seconds precision optional
		jobs:            jobs,
		provider:        p,
		toolReg:         tools,
		memMgr:          mem,
		agentCfg:        cfg,
		log:             logger,
		workspaceDir:    workspaceDir,
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

			runner := agent.NewRunner(d.agentCfg, d.provider, d.toolReg, d.memMgr, d.log, d.workspaceDir)

			// Run in background without streaming to UI
			res, err := runner.Run(ctx, memory.Message{
				ID:        uuid.New().String(),
				SessionID: "cron:" + job.ID,
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
