// Package subagents persists named specialist agents (delegated sub-agents).
package subagents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Agent is a registered sub-agent definition.
type Agent struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	ToolsProfile string    `json:"tools_profile"` // full | coding
	CreatedAt    time.Time `json:"created_at"`
}

type registryFile struct {
	Agents []Agent `json:"agents"`
}

// Store loads and saves ~/.assistclaw/subagents/registry.json.
type Store struct {
	root string
	mu   sync.Mutex
}

// NewStore uses stateDir as AssistClaw root (e.g. ~/.assistclaw).
func NewStore(stateDir string) *Store {
	return &Store{root: stateDir}
}

func (s *Store) registryPath() string {
	return filepath.Join(s.root, "subagents", "registry.json")
}

func (s *Store) agentDir(id string) string {
	return filepath.Join(s.root, "subagents", id)
}

// Load reads the registry from disk.
func (s *Store) Load() ([]Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.registryPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var rf registryFile
	if err := json.Unmarshal(data, &rf); err != nil {
		return nil, err
	}
	return rf.Agents, nil
}

func (s *Store) saveLocked(agents []Agent) error {
	if err := os.MkdirAll(filepath.Dir(s.registryPath()), 0o755); err != nil {
		return err
	}
	rf := registryFile{Agents: agents}
	data, err := json.MarshalIndent(rf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.registryPath(), data, 0o644)
}

// Create registers a sub-agent and writes its workspace SOUL.md.
func (s *Store) Create(name, instructions, toolsProfile string) (Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	instructions = strings.TrimSpace(instructions)
	if name == "" {
		return Agent{}, fmt.Errorf("subagents: name is required")
	}
	if instructions == "" {
		return Agent{}, fmt.Errorf("subagents: instructions are required")
	}
	if toolsProfile != "coding" {
		toolsProfile = "full"
	}
	id := uuid.New().String()[:8]
	dir := s.agentDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Agent{}, err
	}
	soul := "# Specialist sub-agent: " + name + "\n\n" + instructions + "\n\n" +
		"You are a **specialist sub-agent** invoked by the main AssistClaw agent. " +
		"Focus on the task given in each invocation. You do not create or manage other sub-agents."
	if err := os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte(soul), 0o644); err != nil {
		return Agent{}, err
	}
	agents, _ := s.loadLocked()
	a := Agent{
		ID: id, Name: name, Description: "", ToolsProfile: toolsProfile,
		CreatedAt: time.Now().UTC(),
	}
	agents = append(agents, a)
	if err := s.saveLocked(agents); err != nil {
		return Agent{}, err
	}
	return a, nil
}

func (s *Store) loadLocked() ([]Agent, error) {
	data, err := os.ReadFile(s.registryPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var rf registryFile
	if err := json.Unmarshal(data, &rf); err != nil {
		return nil, err
	}
	return rf.Agents, nil
}

// Get returns one agent by id, or nil.
func (s *Store) Get(id string) (*Agent, error) {
	agents, err := s.Load()
	if err != nil {
		return nil, err
	}
	for i := range agents {
		if agents[i].ID == id {
			return &agents[i], nil
		}
	}
	return nil, nil
}

// Remove deletes a sub-agent from the registry and optionally its workspace directory.
func (s *Store) Remove(id string, deleteFiles bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	agents, err := s.loadLocked()
	if err != nil {
		return err
	}
	var out []Agent
	found := false
	for _, a := range agents {
		if a.ID == id {
			found = true
			continue
		}
		out = append(out, a)
	}
	if !found {
		return fmt.Errorf("subagents: no agent with id %q", id)
	}
	if err := s.saveLocked(out); err != nil {
		return err
	}
	if deleteFiles {
		_ = os.RemoveAll(s.agentDir(id))
	}
	return nil
}

// WorkspaceDir returns the on-disk workspace for a sub-agent.
func (s *Store) WorkspaceDir(id string) string {
	return s.agentDir(id)
}
