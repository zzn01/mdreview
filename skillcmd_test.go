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

	got, err := os.ReadFile(filepath.Join(skillsRoot, "review-plan", "SKILL.md"))
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
	path := filepath.Join(skillsRoot, "review-plan", "SKILL.md")
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
