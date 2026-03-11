package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/assistclaw/assistclaw/internal/provider"
)

// ProcessRegistry tracks background processes started by the agent.
var globalProcessRegistry = &processRegistry{
	procs: make(map[string]*managedProcess),
}
var processOnce sync.Once

type managedProcess struct {
	name    string
	cmd     string
	pid     int
	proc    *os.Process
	started time.Time
	logBuf  strings.Builder
	mu      sync.Mutex
}

type PersistedProcess struct {
	Name    string    `json:"name"`
	Cmd     string    `json:"cmd"`
	Pid     int       `json:"pid"`
	Started time.Time `json:"started"`
}

type processRegistry struct {
	mu              sync.Mutex
	procs           map[string]*managedProcess
	persistencePath string
}

func (r *processRegistry) init(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.persistencePath = path
	if path == "" {
		return
	}

	if data, err := os.ReadFile(path); err == nil {
		var persisted []PersistedProcess
		if err := json.Unmarshal(data, &persisted); err == nil {
			for _, p := range persisted {
				// Check if process still running
				proc, err := os.FindProcess(p.Pid)
				if err == nil {
					// On Unix, FindProcess always succeeds.
					// Use Signal(0) to check if it's actually alive.
					if err := proc.Signal(os.Signal(nil)); err == nil {
						r.procs[p.Name] = &managedProcess{
							name:    p.Name,
							cmd:     p.Cmd,
							pid:     p.Pid,
							proc:    proc,
							started: p.Started,
						}
					}
				}
			}
		}
	}
}

func (r *processRegistry) save() {
	if r.persistencePath == "" {
		return
	}
	var persisted []PersistedProcess
	for _, mp := range r.procs {
		persisted = append(persisted, PersistedProcess{
			Name:    mp.name,
			Cmd:     mp.cmd,
			Pid:     mp.pid,
			Started: mp.started,
		})
	}
	data, _ := json.MarshalIndent(persisted, "", "  ")
	_ = os.WriteFile(r.persistencePath, data, 0o644)
}

func (r *processRegistry) add(name string, mp *managedProcess) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.procs[name] = mp
	r.save()
}

func (r *processRegistry) get(name string) (*managedProcess, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	mp, ok := r.procs[name]
	return mp, ok
}

func (r *processRegistry) remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.procs, name)
	r.save()
}

func (r *processRegistry) list() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []map[string]any
	for name, mp := range r.procs {
		running := false
		if mp.proc != nil {
			if err := mp.proc.Signal(os.Signal(nil)); err == nil {
				running = true
			}
		}
		out = append(out, map[string]any{
			"name":    name,
			"pid":     mp.pid,
			"cmd":     mp.cmd,
			"started": mp.started.Format(time.RFC3339),
			"running": running,
		})
	}
	return out
}

// ProcessTool manages background processes: start, stop, status, logs.
type ProcessTool struct {
	PersistencePath string
}

func (ProcessTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name: "process",
		Description: `Start, stop, inspect, or list background processes.
Commands:
  start  — start a shell command in the background (returns immediately)
  stop   — kill a running process by name
  status — check if a named process is still running
  logs   — read recent output from a background process
  list   — list all background processes managed by the agent`,
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"command": map[string]any{"type": "string", "description": "One of: start, stop, status, logs, list"},
				"name":    map[string]any{"type": "string", "description": "Name to identify this process (required for start, stop, status, logs)"},
				"cmd":     map[string]any{"type": "string", "description": "Shell command to run (required for start)"},
				"dir":     map[string]any{"type": "string", "description": "Working directory (optional, for start)"},
			},
			Required: []string{"command"},
		},
	}
}

func (t ProcessTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Command string `json:"command"`
		Name    string `json:"name"`
		Cmd     string `json:"cmd"`
		Dir     string `json:"dir"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}

	processOnce.Do(func() {
		globalProcessRegistry.init(t.PersistencePath)
	})

	switch strings.ToLower(args.Command) {
	case "start":
		return processStart(args.Name, args.Cmd, args.Dir)
	case "stop":
		return processStop(args.Name)
	case "status":
		return processStatus(args.Name)
	case "logs":
		return processLogs(args.Name)
	case "list":
		items := globalProcessRegistry.list()
		if len(items) == 0 {
			return "No background processes currently tracked.", nil
		}
		b, _ := json.MarshalIndent(items, "", "  ")
		return string(b), nil
	default:
		return fmt.Sprintf("Unknown process command %q. Use: start, stop, status, logs, list", args.Command), nil
	}
}

func processStart(name, cmd, dir string) (string, error) {
	if name == "" {
		return "process start: 'name' is required", nil
	}
	if cmd == "" {
		return "process start: 'cmd' is required", nil
	}

	// If name already in use and running, refuse
	if mp, ok := globalProcessRegistry.get(name); ok {
		if mp.proc != nil {
			if err := mp.proc.Signal(os.Signal(nil)); err == nil {
				return fmt.Sprintf("Process %q is already running (PID %d). Stop it first.", name, mp.pid), nil
			}
		}
	}

	mp := &managedProcess{name: name, cmd: cmd, started: time.Now()}
	c := exec.Command("bash", "-c", cmd)
	c.Env = os.Environ()
	if dir != "" {
		c.Dir = dir
	}

	// Capture stdout+stderr into log buffer
	c.Stdout = &mp.logBuf
	c.Stderr = &mp.logBuf

	if err := c.Start(); err != nil {
		return fmt.Sprintf("process start: failed to start %q: %v", name, err), nil
	}

	mp.proc = c.Process
	mp.pid = c.Process.Pid
	globalProcessRegistry.add(name, mp)

	// Reap the process asynchronously to avoid zombies
	go func() {
		_ = c.Wait()
	}()

	return fmt.Sprintf("✔ Process %q started (PID %d): %s", name, mp.pid, cmd), nil
}

func processStop(name string) (string, error) {
	if name == "" {
		return "process stop: 'name' is required", nil
	}
	mp, ok := globalProcessRegistry.get(name)
	if !ok {
		return fmt.Sprintf("No process named %q is tracked.", name), nil
	}
	if mp.proc != nil {
		_ = mp.proc.Kill()
	}
	globalProcessRegistry.remove(name)
	return fmt.Sprintf("✔ Process %q (PID %d) stopped.", name, mp.pid), nil
}

func processStatus(name string) (string, error) {
	if name == "" {
		return "process status: 'name' is required", nil
	}
	mp, ok := globalProcessRegistry.get(name)
	if !ok {
		return fmt.Sprintf("No process named %q is tracked.", name), nil
	}
	running := false
	if mp.proc != nil {
		if err := mp.proc.Signal(os.Signal(nil)); err == nil {
			running = true
		}
	}
	status := "stopped"
	if running {
		status = "running"
	}
	uptime := time.Since(mp.started).Round(time.Second)
	return fmt.Sprintf("Process %q: PID=%d status=%s uptime=%s cmd=%q",
		name, mp.pid, status, uptime, mp.cmd), nil
}

func processLogs(name string) (string, error) {
	if name == "" {
		return "process logs: 'name' is required", nil
	}
	mp, ok := globalProcessRegistry.get(name)
	if !ok {
		return fmt.Sprintf("No process named %q is tracked.", name), nil
	}
	mp.mu.Lock()
	logs := mp.logBuf.String()
	mp.mu.Unlock()
	if logs == "" {
		return fmt.Sprintf("Process %q has produced no output yet.", name), nil
	}
	// Return last 4000 chars to stay within token budget
	if len(logs) > 4000 {
		logs = "...(truncated)...\n" + logs[len(logs)-4000:]
	}
	pid := strconv.Itoa(mp.pid)
	return fmt.Sprintf("=== Logs for process %q (PID %s) ===\n%s", name, pid, logs), nil
}
