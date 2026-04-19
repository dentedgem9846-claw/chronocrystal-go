package skills

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Skill represents a discovered skill loaded from a markdown file with YAML frontmatter.
type Skill struct {
	Name            string
	Description     string
	TriggerKeywords []string
	Content         string
}

// Registry discovers and holds skills loaded from a directory of markdown files.
type Registry struct {
	skillsDir string
	skills    []Skill
}

// NewRegistry creates a Registry rooted at skillsDir.
func NewRegistry(skillsDir string) *Registry {
	return &Registry{skillsDir: skillsDir}
}

// Discover scans skillsDir for .md files and parses YAML frontmatter from each.
// Returns an error only if the directory does not exist. Individual file errors are skipped.
func (r *Registry) Discover(_ context.Context) error {
	entries, err := os.ReadDir(r.skillsDir)
	if err != nil {
		return fmt.Errorf("read skills directory %s: %w", r.skillsDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(r.skillsDir, entry.Name())
		skill, ok := parseSkillFile(path)
		if !ok {
			continue
		}
		r.skills = append(r.skills, skill)
	}
	return nil
}

// Match returns all skills whose trigger keywords appear in the message (case-insensitive).
func (r *Registry) Match(message string) []Skill {
	lower := strings.ToLower(message)
	var matched []Skill
	for i := range r.skills {
		if skillMatches(r.skills[i], lower) {
			matched = append(matched, r.skills[i])
		}
	}
	return matched
}

// All returns every discovered skill.
func (r *Registry) All() []Skill {
	return r.skills
}

// InstructionsFor formats matching skills for prompt injection.
// Returns an empty string if no skills match.
func (r *Registry) InstructionsFor(message string) string {
	matched := r.Match(message)
	if len(matched) == 0 {
		return ""
	}
	var b strings.Builder
	for i, s := range matched {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "## Skill: %s\n%s\n%s", s.Name, s.Description, s.Content)
	}
	return b.String()
}

// skillMatches checks whether any trigger keyword of s appears in lowerMessage.
func skillMatches(s Skill, lowerMessage string) bool {
	for _, kw := range s.TriggerKeywords {
		if strings.Contains(lowerMessage, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// parseSkillFile reads a markdown file and extracts frontmatter fields.
// Returns the Skill and true on success, or zero Skill and false if the file
// lacks valid frontmatter or cannot be read.
func parseSkillFile(path string) (Skill, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Skill{}, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// First non-empty line must be "---".
	if !scanToNonEmpty(scanner) {
		return Skill{}, false
	}
	if scanner.Text() != "---" {
		return Skill{}, false
	}

	// Collect lines until the closing "---".
	var frontmatterLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			break
		}
		frontmatterLines = append(frontmatterLines, line)
	}
	if len(frontmatterLines) == 0 {
		return Skill{}, false
	}

	// Remaining lines are the content.
	var contentBuilder strings.Builder
	for scanner.Scan() {
		contentBuilder.WriteString(scanner.Text())
		contentBuilder.WriteByte('\n')
	}
	content := strings.TrimRight(contentBuilder.String(), "\n")

	skill := parseFrontmatter(frontmatterLines)
	skill.Content = content
	return skill, true
}

// scanToNonEmpty advances the scanner to the first non-empty line.
// Returns false if EOF is reached.
func scanToNonEmpty(scanner *bufio.Scanner) bool {
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			return true
		}
	}
	return false
}

// parseFrontmatter parses simple key: value pairs from frontmatter lines.
func parseFrontmatter(lines []string) Skill {
	var s Skill
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := splitKeyValue(line)
		if !ok {
			continue
		}
		switch key {
		case "name":
			s.Name = value
		case "description":
			s.Description = value
		case "trigger_keywords":
			s.TriggerKeywords = parseBracketList(value)
		}
	}
	return s
}

// splitKeyValue splits "key: value" at the first colon.
func splitKeyValue(line string) (string, string, bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:idx])
	value := strings.TrimSpace(line[idx+1:])
	return key, value, true
}

// parseBracketList parses "[keyword1, keyword2, keyword3]" into a string slice.
// Returns nil for unparseable input.
func parseBracketList(s string) []string {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return nil
	}
	inner := s[1 : len(s)-1]
	if strings.TrimSpace(inner) == "" {
		return nil
	}
	parts := strings.Split(inner, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}