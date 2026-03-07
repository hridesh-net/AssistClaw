package skills

import (
	"context"
	"encoding/json"
)

// Skill represents a loaded OpenClaw-compatible skill graph.
type Skill struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Version     string `yaml:"version,omitempty"`
	Author      string `yaml:"author,omitempty"`

	// Homepage/Documentation URL
	Homepage string `yaml:"homepage,omitempty"`

	// Metadata contains additional skill information like emojis or hardware requirements.
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

	// Nodes contains all content files (.md) within the skill directory.
	// The primary SKILL.md is typically the entry point node.
	Nodes map[string]*Node `yaml:"-"`

	// FilePath is the absolute path to the main SKILL.md file.
	FilePath string `yaml:"-"`

	// Tools defines any custom actions provided by the skill.
	Tools []SkillTool `yaml:"tools,omitempty"`
}

// Node represents an individual markdown file within a skill, acting as a graph node.
type Node struct {
	Name         string   `yaml:"name"`    // Simple filename (e.g. "risk-management")
	Summary      string   `yaml:"summary"` // Short description from frontmatter
	Instructions string   `yaml:"-"`       // Full body content (populated after parsing)
	FilePath     string   `yaml:"-"`       // Absolute path
	Links        []string `yaml:"-"`       // Names of other nodes linked via [[wikilinks]]
}

// SkillTool represents a callable tool within a skill directory.
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

	// Register adds (or replaces) a virtual skill in the registry.
	// Used by the MCP adapter to inject external server tools as skill nodes.
	Register(skill *Skill)

	// ReadSkillNode retrieves the content of a specific node within a skill.
	ReadSkillNode(skillName string, nodeName string) (*Node, bool)

	// BuildContext dynamically compiles a "Map of Content" for active skills.
	BuildContext(activeSkillNames []string) string

	// Discover lists available skill names in the directory without loading full body.
	Discover(dir string) ([]SkillInfo, error)

	// CheckRequirements verifies if the skill's dependencies are met.
	CheckRequirements(skill *Skill) (bool, []string)

	// InstallDependency attempts to install missing dependencies for a skill.
	InstallDependency(ctx context.Context, skill *Skill) error

	// RepairAllEnabled iterates through the given skill names and attempts to install missing dependencies for each.
	RepairAllEnabled(ctx context.Context, enabledNames []string) error

	// FindBridges looks for intermediate nodes connecting the given file paths.
	FindBridges(paths []string) []*Node
}
