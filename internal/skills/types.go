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

	// Homepage/Documentation URL
	Homepage string `yaml:"homepage,omitempty"`

	// Metadata contains additional skill information like emojis or hardware requirements.
	// It mirrors OpenClaw's rich metadata structure.
	Metadata struct {
		OpenClaw struct {
			Emoji      string   `yaml:"emoji,omitempty"`
			Always     bool     `yaml:"always,omitempty"`
			PrimaryEnv string   `yaml:"primaryEnv,omitempty"`
			OS         []string `yaml:"os,omitempty"`
			Requires   struct {
				Bins    []string `yaml:"bins,omitempty"`
				AnyBins []string `yaml:"anyBins,omitempty"`
				Env     []string `yaml:"env,omitempty"`
				Config  []string `yaml:"config,omitempty"`
			} `yaml:"requires,omitempty"`
			Install []map[string]any `yaml:"install,omitempty"`
		} `yaml:"openclaw,omitempty"`
	} `yaml:"metadata,omitempty"`

	// Instructions is the Markdown body of the SKILL.md file
	// that will be injected into the system prompt when active.
	Instructions string `yaml:"-"`

	// FilePath is the absolute path to the SKILL.md file.
	FilePath string `yaml:"-"`

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

// SkillInfo used for discovery.
type SkillInfo struct {
	Name        string
	Description string
	Emoji       string
	Eligible    bool
	Missing     []string
}

// Registry manages loaded skills.
type Registry interface {
	LoadAll(ctx context.Context, dir string) error
	Get(name string) (*Skill, bool)
	List() []Skill

	// BuildContext dynamically compiles the active skills into a prompt string.
	BuildContext(activeSkillNames []string) string

	// Discover lists available skill names in the directory without loading full body.
	Discover(dir string) ([]SkillInfo, error)

	// CheckRequirements verifies if the skill's dependencies are met.
	CheckRequirements(skill *Skill) (bool, []string)
}
