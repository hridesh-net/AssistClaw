package skills

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type loader struct {
	skills map[string]*Skill
}

// NewRegistry creates a new Skill Registry.
func NewRegistry() Registry {
	return &loader{
		skills: make(map[string]*Skill),
	}
}

// LoadAll walks the given directory looking for skill directories and indexing all .md nodes.
func (l *loader) LoadAll(ctx context.Context, dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := filepath.Join(dir, entry.Name())
		skill, err := l.loadSkill(skillDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skills: failed to load skill in %s: %v\n", skillDir, err)
			continue
		}
		l.skills[skill.Name] = skill
	}
	return nil
}

func (l *loader) loadSkill(skillDir string) (*Skill, error) {
	mainFile := filepath.Join(skillDir, "SKILL.md")
	skill, err := l.parseSkillMetadata(mainFile)
	if err != nil {
		// If main metadata fails, we still try to treat the dir as a skill if it has md files
		skill = &Skill{
			Name: filepath.Base(skillDir),
		}
	}
	skill.Nodes = make(map[string]*Node)
	skill.FilePath = mainFile

	// Index all .md files as nodes
	err = filepath.Walk(skillDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".md" {
			node, err := l.parseNodeFile(path)
			if err != nil {
				return nil // Skip broken nodes
			}
			skill.Nodes[node.Name] = node
		}
		return nil
	})

	return skill, err
}

func (l *loader) Get(name string) (*Skill, bool) {
	s, ok := l.skills[name]
	return s, ok
}

func (l *loader) ReadSkillNode(skillName string, nodeName string) (*Node, bool) {
	skill, ok := l.skills[skillName]
	if !ok {
		return nil, false
	}
	node, ok := skill.Nodes[nodeName]
	return node, ok
}

func (l *loader) List() []Skill {
	var out []Skill
	for _, s := range l.skills {
		out = append(out, *s)
	}
	return out
}

func (l *loader) Discover(dir string) ([]SkillInfo, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}

	var out []SkillInfo
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := filepath.Join(dir, entry.Name())
		mainFile := filepath.Join(skillDir, "SKILL.md")
		skill, err := l.parseSkillMetadata(mainFile)
		if err != nil {
			continue
		}
		met, missing := l.CheckRequirements(skill)
		out = append(out, SkillInfo{
			Name:        skill.Name,
			Description: skill.Description,
			Emoji:       skill.Metadata.OpenClaw.Emoji,
			Eligible:    met,
			Missing:     missing,
		})
	}
	return out, nil
}

func (l *loader) BuildContext(activeSkillNames []string) string {
	if len(activeSkillNames) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n<skill_graph>\n")
	sb.WriteString("You have access to the following Skill Graphs. Each graph contains nodes you can read using the 'read_skill_node' tool.\n")

	for _, name := range activeSkillNames {
		s, ok := l.skills[name]
		if !ok {
			continue
		}

		met, missing := l.CheckRequirements(s)
		if !met {
			sb.WriteString(fmt.Sprintf("\n<skill name=\"%s\" status=\"error\" error=\"missing dependencies: %s\" />\n", s.Name, strings.Join(missing, ", ")))
			continue
		}

		sb.WriteString(fmt.Sprintf("\n<skill name=\"%s\" description=\"%s\">\n", s.Name, s.Description))
		sb.WriteString("  <nodes>\n")
		for _, node := range s.Nodes {
			sb.WriteString(fmt.Sprintf("    <node name=\"%s\" summary=\"%s\" />\n", node.Name, node.Summary))
		}
		sb.WriteString("  </nodes>\n")
		sb.WriteString("</skill>\n")
	}

	sb.WriteString("</skill_graph>\n")
	return sb.String()
}

func (l *loader) InstallDependency(ctx context.Context, skill *Skill) error {
	if skill.Metadata.OpenClaw.Install == nil {
		return fmt.Errorf("no installation instructions for skill %s", skill.Name)
	}

	var lastErr error
	for _, inst := range skill.Metadata.OpenClaw.Install {
		kind, _ := inst["kind"].(string)
		label, _ := inst["label"].(string)

		fmt.Printf("Attempting: %s...\n", label)

		var cmd *exec.Cmd
		switch kind {
		case "brew":
			formula, _ := inst["formula"].(string)
			cmd = exec.CommandContext(ctx, "brew", "install", formula)
		case "go":
			module, _ := inst["module"].(string)
			cmd = exec.CommandContext(ctx, "go", "install", module)
		case "apt":
			pkg, _ := inst["package"].(string)
			cmd = exec.CommandContext(ctx, "sudo", "apt-get", "install", "-y", pkg)
		case "python", "pip":
			pkg, _ := inst["package"].(string)
			cmd = exec.CommandContext(ctx, "pip", "install", pkg)
		default:
			continue
		}

		if cmd == nil {
			continue
		}

		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			lastErr = err
			fmt.Printf("Failed: %v\n", err)
			continue
		}

		if met, _ := l.CheckRequirements(skill); met {
			return nil
		}
	}

	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("failed to install dependencies for %s using any available method", skill.Name)
}

func (l *loader) CheckRequirements(skill *Skill) (bool, []string) {
	var missing []string

	// Check bins
	for _, bin := range skill.Metadata.OpenClaw.Requires.Bins {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, bin)
		}
	}

	// Check anyBins
	if len(skill.Metadata.OpenClaw.Requires.AnyBins) > 0 {
		found := false
		for _, bin := range skill.Metadata.OpenClaw.Requires.AnyBins {
			if _, err := exec.LookPath(bin); err == nil {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, fmt.Sprintf("one of (%s)", strings.Join(skill.Metadata.OpenClaw.Requires.AnyBins, ", ")))
		}
	}

	return len(missing) == 0, missing
}

func (l *loader) compactPath(p string) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

// parseSkillMetadata reads the frontmatter of the main SKILL.md.
func (l *loader) parseSkillMetadata(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	const separator = "---\n"
	if bytes.HasPrefix(data, []byte(separator)) {
		endIdx := bytes.Index(data[len(separator):], []byte(separator))
		if endIdx != -1 {
			frontmatter := data[len(separator) : len(separator)+endIdx]
			var skill Skill
			if err := yaml.Unmarshal(frontmatter, &skill); err != nil {
				return nil, fmt.Errorf("invalid frontmatter: %w", err)
			}
			if skill.Name == "" {
				skill.Name = filepath.Base(filepath.Dir(path))
			}
			return &skill, nil
		}
	}

	return &Skill{
		Name: filepath.Base(filepath.Dir(path)),
	}, nil
}

// parseNodeFile reads any .md file and extracts summary/instructions.
func (l *loader) parseNodeFile(path string) (*Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var node Node
	node.Name = strings.TrimSuffix(filepath.Base(path), ".md")
	node.FilePath = path

	const separator = "---\n"
	if bytes.HasPrefix(data, []byte(separator)) {
		endIdx := bytes.Index(data[len(separator):], []byte(separator))
		if endIdx != -1 {
			frontmatter := data[len(separator) : len(separator)+endIdx]
			_ = yaml.Unmarshal(frontmatter, &node)
			bodyStart := len(separator) + endIdx + len(separator)
			node.Instructions = strings.TrimSpace(string(data[bodyStart:]))
		}
	} else {
		node.Instructions = strings.TrimSpace(string(data))
	}

	return &node, nil
}
