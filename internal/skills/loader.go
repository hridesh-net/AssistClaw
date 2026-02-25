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

// LoadAll walks the given directory looking for SKILL.md files.
func (l *loader) LoadAll(ctx context.Context, dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil // No skills dir yet, perfectly fine
	}

	return filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Base(path) == "SKILL.md" {
			skill, err := l.parseSkillFile(path)
			if err != nil {
				// Log but continue
				fmt.Fprintf(os.Stderr, "skills: failed to load %s: %v\n", path, err)
				return nil
			}
			l.skills[skill.Name] = skill
		}
		return nil
	})
}

func (l *loader) Get(name string) (*Skill, bool) {
	s, ok := l.skills[name]
	return s, ok
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
	err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Base(path) == "SKILL.md" {
			skill, err := l.parseSkillFile(path)
			if err != nil {
				return nil
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
		return nil
	})
	return out, err
}

func (l *loader) BuildContext(activeSkillNames []string) string {
	if len(activeSkillNames) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n<active_skills>\n")
	sb.WriteString("You have the following skills active to assist you with your tasks:\n")

	for _, name := range activeSkillNames {
		s, ok := l.skills[name]
		if !ok {
			continue
		}

		// Check requirements
		met, missing := l.CheckRequirements(s)
		if !met {
			sb.WriteString(fmt.Sprintf("\n<skill name=\"%s\" status=\"error\" error=\"missing dependencies: %s\">\n", s.Name, strings.Join(missing, ", ")))
			sb.WriteString("This skill is missing required dependencies and cannot be used.")
			sb.WriteString("\n</skill>\n")
			continue
		}

		path := l.compactPath(s.FilePath)
		sb.WriteString(fmt.Sprintf("\n<skill name=\"%s\" file=\"%s\">\n", s.Name, path))
		sb.WriteString(s.Instructions)
		sb.WriteString("\n</skill>\n")
	}

	sb.WriteString("</active_skills>\n")
	return sb.String()
}

func (l *loader) InstallDependency(ctx context.Context, skill *Skill) error {
	if skill.Metadata.OpenClaw.Install == nil {
		return fmt.Errorf("no installation instructions for skill %s", skill.Name)
	}

	// Try each installation method until one succeeds or we run out
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

		// Double check if requirement is met now
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

	// Check anyBins (at least one must exist)
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

// parseSkillFile reads a SKILL.md file, extracting YAML frontmatter and the Markdown body.
func (l *loader) parseSkillFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var skill Skill

	// Find YAML frontmatter
	const separator = "---\n"
	if bytes.HasPrefix(data, []byte(separator)) {
		endIdx := bytes.Index(data[len(separator):], []byte(separator))
		if endIdx != -1 {
			frontmatter := data[len(separator) : len(separator)+endIdx]
			if err := yaml.Unmarshal(frontmatter, &skill); err != nil {
				return nil, fmt.Errorf("invalid frontmatter: %w", err)
			}
			// Body is everything after the second separator
			bodyStart := len(separator) + endIdx + len(separator)
			skill.Instructions = strings.TrimSpace(string(data[bodyStart:]))
		}
	} else {
		// No frontmatter, use directory name as skill name
		skill.Name = filepath.Base(filepath.Dir(path))
		skill.Instructions = strings.TrimSpace(string(data))
	}

	skill.FilePath = path
	if skill.Name == "" {
		skill.Name = filepath.Base(filepath.Dir(path))
	}

	return &skill, nil
}
