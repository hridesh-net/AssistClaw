package skills

import (
	"context"
	"encoding/json"
)

// Skill represents an loaded OpenClaw-compatible skill.
type Skill struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Version     string `yaml:"version,omitempty"`
	Author      string `yaml:"author,omitempty"`

	// Instructions is the Markdown body of the SKILL.md file
	// that will be injected into the system prompt when active.
	Instructions string `yaml:"-"`

	// Tools defines any custom actions provided by the skill.
	Tools []SkillTool `yaml:"tools,omitempty"`
}

// SkillTool represents a callable tool within a skill directory.
// Skills often provide Python scripts or executables alongside the SKILL.md.
type SkillTool struct {
	Name        string          `yaml:"name"`
	Description string          `yaml:"description"`
	Command     string          `yaml:"command"`          // e.g. "python3 script.py"
	Schema      json.RawMessage `yaml:"schema,omitempty"` // JSON schema for parameters
}

// Registry manages loaded skills.
type Registry interface {
	LoadAll(ctx context.Context, dir string) error
	Get(name string) (*Skill, bool)
	List() []Skill

	// BuildContext dynamically compiles the active skills into a prompt string.
	BuildContext(activeSkillNames []string) string
}
