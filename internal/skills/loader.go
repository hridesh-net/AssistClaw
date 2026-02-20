package skills

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
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
		sb.WriteString(fmt.Sprintf("\n<skill name=\"%s\">\n", s.Name))
		sb.WriteString(s.Instructions)
		sb.WriteString("\n</skill>\n")
	}

	sb.WriteString("</active_skills>\n")
	return sb.String()
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

	if skill.Name == "" {
		skill.Name = filepath.Base(filepath.Dir(path))
	}

	return &skill, nil
}
