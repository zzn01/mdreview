package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallSkill_FreshInstall(t *testing.T) {
	dir := t.TempDir()
	skillsRoot := filepath.Join(dir, ".claude", "skills")

	if err := installSkill(skillsRoot); err != nil {
		t.Fatalf("installSkill: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(skillsRoot, "mdreview", "SKILL.md"))
	if err != nil {
		t.Fatalf("reading installed skill: %v", err)
	}
	if !bytes.Equal(got, skillMD) {
		t.Errorf("installed content does not match embedded skill.md")
	}
}

func TestInstallSkill_Overwrites(t *testing.T) {
	dir := t.TempDir()
	skillsRoot := filepath.Join(dir, ".claude", "skills")

	if err := installSkill(skillsRoot); err != nil {
		t.Fatalf("first installSkill: %v", err)
	}

	// Simulate a stale/edited copy on disk.
	path := filepath.Join(skillsRoot, "mdreview", "SKILL.md")
	if err := os.WriteFile(path, []byte("stale content"), 0o644); err != nil {
		t.Fatalf("writing stale content: %v", err)
	}

	if err := installSkill(skillsRoot); err != nil {
		t.Fatalf("second installSkill: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading installed skill: %v", err)
	}
	if !bytes.Equal(got, skillMD) {
		t.Errorf("second install did not overwrite stale content with embedded skill.md")
	}
}

func TestResolveSkillsDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	tests := []struct {
		name    string
		project bool
		dir     string
		want    string
	}{
		{"default is user-level claude skills", false, "", filepath.Join(home, ".claude", "skills")},
		{"project installs to local claude skills", true, "", filepath.Join(".", ".claude", "skills")},
		{"explicit dir wins", false, "/some/agent/skills", "/some/agent/skills"},
		{"explicit dir wins over project", true, "/some/agent/skills", "/some/agent/skills"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSkillsDir(tt.project, tt.dir)
			if err != nil {
				t.Fatalf("resolveSkillsDir: %v", err)
			}
			if got != tt.want {
				t.Errorf("resolveSkillsDir(%v, %q) = %q, want %q", tt.project, tt.dir, got, tt.want)
			}
		})
	}
}
