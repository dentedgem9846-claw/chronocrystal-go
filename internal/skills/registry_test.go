package skills

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry("/some/dir")
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if r.skillsDir != "/some/dir" {
		t.Errorf("skillsDir = %q, want /some/dir", r.skillsDir)
	}
	if len(r.skills) != 0 {
		t.Errorf("skills = %d, want 0", len(r.skills))
	}
}

func TestDiscoverEmptyDir(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)
	if err := r.Discover(context.Background()); err != nil {
		t.Fatalf("Discover on empty dir: %v", err)
	}
	if got := r.All(); len(got) != 0 {
		t.Errorf("All() = %d skills, want 0", len(got))
	}
}

func TestDiscoverNonexistentDir(t *testing.T) {
	r := NewRegistry("/no/such/directory")
	err := r.Discover(context.Background())
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

func TestDiscoverValidSkills(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "test-skill.md", `---
name: Test Skill
description: A test skill
trigger_keywords: [test, keyword]
---
This is the skill content.
`)

	r := NewRegistry(dir)
	if err := r.Discover(context.Background()); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	all := r.All()
	if len(all) != 1 {
		t.Fatalf("All() = %d skills, want 1", len(all))
	}
	s := all[0]
	if s.Name != "Test Skill" {
		t.Errorf("Name = %q, want %q", s.Name, "Test Skill")
	}
	if s.Description != "A test skill" {
		t.Errorf("Description = %q, want %q", s.Description, "A test skill")
	}
	if len(s.TriggerKeywords) != 2 {
		t.Fatalf("TriggerKeywords = %v, want 2 entries", s.TriggerKeywords)
	}
	if s.TriggerKeywords[0] != "test" || s.TriggerKeywords[1] != "keyword" {
		t.Errorf("TriggerKeywords = %v, want [test, keyword]", s.TriggerKeywords)
	}
	if s.Content != "This is the skill content." {
		t.Errorf("Content = %q, want %q", s.Content, "This is the skill content.")
	}
}

func TestDiscoverSkipsNonMarkdown(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "notes.txt", "not a skill")
	writeSkill(t, dir, "real.md", `---
name: Real
description: Real skill
---
Content.
`)

	r := NewRegistry(dir)
	if err := r.Discover(context.Background()); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	all := r.All()
	if len(all) != 1 || all[0].Name != "Real" {
		t.Errorf("All() = %v, want single skill named Real", all)
	}
}

func TestDiscoverSkipsDirs(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	// Put a .md inside the subdir — it should not be read since Discover
	// only scans the top-level directory.
	writeSkill(t, sub, "nested.md", `---
name: Nested
description: Should be ignored
---
Nested content.
`)

	r := NewRegistry(dir)
	if err := r.Discover(context.Background()); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(r.All()) != 0 {
		t.Errorf("All() = %d skills, want 0 (subdirs skipped)", len(r.All()))
	}
}

func TestDiscoverSkipsInvalidFrontmatter(t *testing.T) {
	dir := t.TempDir()
	// No frontmatter markers at all.
	writeSkill(t, dir, "bad.md", "Just some text without frontmatter.\n")

	// Frontmatter missing closing ---.
	writeSkill(t, dir, "unclosed.md", `---
name: Unclosed
description: Missing close
No closing marker here.
`)

	r := NewRegistry(dir)
	if err := r.Discover(context.Background()); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(r.All()) != 0 {
		t.Errorf("All() = %d, want 0 (invalid frontmatter skipped)", len(r.All()))
	}
}

func TestMatchKeywordHit(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "deploy.md", `---
name: Deploy
description: Deploy things
trigger_keywords: [deploy, ship]
---
Deploy content.
`)

	r := NewRegistry(dir)
	if err := r.Discover(context.Background()); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	matched := r.Match("please deploy the service")
	if len(matched) != 1 {
		t.Fatalf("Match = %d, want 1", len(matched))
	}
	if matched[0].Name != "Deploy" {
		t.Errorf("Match[0].Name = %q, want Deploy", matched[0].Name)
	}
}

func TestMatchNoKeywords(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "deploy.md", `---
name: Deploy
description: Deploy things
trigger_keywords: [deploy, ship]
---
Deploy content.
`)

	r := NewRegistry(dir)
	if err := r.Discover(context.Background()); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	matched := r.Match("tell me about the weather")
	if len(matched) != 0 {
		t.Errorf("Match = %d, want 0", len(matched))
	}
}

func TestMatchCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "deploy.md", `---
name: Deploy
description: Deploy things
trigger_keywords: [Deploy, SHIP]
---
Deploy content.
`)

	r := NewRegistry(dir)
	if err := r.Discover(context.Background()); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	matched := r.Match("please deploy and ship it")
	if len(matched) != 1 {
		t.Fatalf("Match = %d, want 1 (case-insensitive)", len(matched))
	}
}

func TestAllReturnsAllSkills(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "a.md", `---
name: A
description: Skill A
trigger_keywords: [a]
---
A content.
`)
	writeSkill(t, dir, "b.md", `---
name: B
description: Skill B
trigger_keywords: [b]
---
B content.
`)

	r := NewRegistry(dir)
	if err := r.Discover(context.Background()); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	all := r.All()
	if len(all) != 2 {
		t.Fatalf("All() = %d, want 2", len(all))
	}
	names := map[string]bool{}
	for _, s := range all {
		names[s.Name] = true
	}
	if !names["A"] || !names["B"] {
		t.Errorf("All() names = %v, want A and B", names)
	}
}

func TestInstructionsForMatchedSkills(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "deploy.md", `---
name: Deploy
description: Deploy things
trigger_keywords: [deploy]
---
Deploy content here.
`)

	r := NewRegistry(dir)
	if err := r.Discover(context.Background()); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	got := r.InstructionsFor("let's deploy")
	if !strings.Contains(got, "## Skill: Deploy") {
		t.Errorf("InstructionsFor missing skill header, got:\n%s", got)
	}
	if !strings.Contains(got, "Deploy things") {
		t.Errorf("InstructionsFor missing description, got:\n%s", got)
	}
	if !strings.Contains(got, "Deploy content here.") {
		t.Errorf("InstructionsFor missing content, got:\n%s", got)
	}
}

func TestInstructionsForNoMatch(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "deploy.md", `---
name: Deploy
description: Deploy things
trigger_keywords: [deploy]
---
Deploy content.
`)

	r := NewRegistry(dir)
	if err := r.Discover(context.Background()); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	got := r.InstructionsFor("something unrelated")
	if got != "" {
		t.Errorf("InstructionsFor = %q, want empty string", got)
	}
}

func TestParseBracketList(t *testing.T) {
	got := parseBracketList("[keyword1, keyword2]")
	if len(got) != 2 || got[0] != "keyword1" || got[1] != "keyword2" {
		t.Errorf("parseBracketList = %v, want [keyword1, keyword2]", got)
	}
}

func TestParseBracketListEmpty(t *testing.T) {
	got := parseBracketList("[]")
	if got != nil {
		t.Errorf("parseBracketList = %v, want nil", got)
	}
}

func TestParseBracketListInvalid(t *testing.T) {
	got := parseBracketList("no brackets")
	if got != nil {
		t.Errorf("parseBracketList = %v, want nil", got)
	}
}

func TestSplitKeyValue(t *testing.T) {
	key, value, ok := splitKeyValue("name: Test Skill")
	if !ok {
		t.Fatal("splitKeyValue returned false")
	}
	if key != "name" {
		t.Errorf("key = %q, want %q", key, "name")
	}
	if value != "Test Skill" {
		t.Errorf("value = %q, want %q", value, "Test Skill")
	}
}

func TestSplitKeyValueNoColon(t *testing.T) {
	_, _, ok := splitKeyValue("no colon here")
	if ok {
		t.Error("splitKeyValue returned true for line without colon")
	}
}

func TestScanToNonEmpty(t *testing.T) {
	input := "\n\n  \nfirst line\nsecond line\n"
	scanner := bufio.NewScanner(strings.NewReader(input))
	if !scanToNonEmpty(scanner) {
		t.Fatal("scanToNonEmpty returned false")
	}
	if scanner.Text() != "first line" {
		t.Errorf("scanner.Text() = %q, want %q", scanner.Text(), "first line")
	}
}

// writeSkill is a test helper that writes a skill file to dir.
func writeSkill(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}