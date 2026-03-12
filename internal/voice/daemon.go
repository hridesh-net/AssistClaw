package voice

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/assistclaw/assistclaw/internal/config"
)

type Daemon struct {
	cfg    config.VoiceConfig
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

func NewDaemon(cfg config.VoiceConfig) *Daemon {
	return &Daemon{cfg: cfg}
}

// Start launches the internal microservice.
func (d *Daemon) Start(ctx context.Context) error {
	if !d.cfg.Enabled {
		return nil
	}

	stateDir := d.cfg.VenvPath
	if stateDir == "" {
		stateDir = filepath.Join(os.Getenv("HOME"), ".assistclaw")
	}
	
	venvPath := filepath.Join(stateDir, "voice_env")
	pythonPath := filepath.Join(venvPath, "bin", "python")
	if os.PathSeparator == '\\' {
		pythonPath = filepath.Join(venvPath, "Scripts", "python.exe")
	}

	// Check if venv exists
	if _, err := os.Stat(pythonPath); os.IsNotExist(err) {
		log.Printf("voice: venv not found at %s. Running setup...", venvPath)
		setupCmd := exec.Command("python3", "scripts/setup_voice.py", stateDir)
		setupCmd.Stdout = os.Stdout
		setupCmd.Stderr = os.Stderr
		if err := setupCmd.Run(); err != nil {
			return fmt.Errorf("failed to setup voice venv: %w", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel

	d.cmd = exec.CommandContext(ctx, pythonPath, "scripts/voice_server.py")
	d.cmd.Env = append(os.Environ(), fmt.Sprintf("VOICE_PORT=%d", d.cfg.ServicePort))
	d.cmd.Stdout = os.Stdout
	d.cmd.Stderr = os.Stderr

	go func() {
		if err := d.cmd.Run(); err != nil {
			if ctx.Err() == nil {
				log.Printf("voice: server exited unexpectedly: %v", err)
			}
		}
	}()

	log.Printf("voice: internal microservice starting on port %d", d.cfg.ServicePort)
	
	// Wait for health check
	return d.waitForReady()
}

func (d *Daemon) waitForReady() error {
	url := fmt.Sprintf("http://127.0.0.1:%d/", d.cfg.ServicePort)
	client := http.Client{Timeout: 1 * time.Second}
	
	for i := 0; i < 30; i++ {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("voice: server timed out starting")
}

func (d *Daemon) Stop() error {
	if d.cancel != nil {
		d.cancel()
	}
	return nil
}
