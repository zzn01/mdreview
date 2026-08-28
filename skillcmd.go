package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed skill/SKILL.md
var skillMD []byte

// installSkill writes the embedded skill file under dir/review-plan/SKILL.md,
// creating directories as needed. dir is the skills root (e.g.
// ~/.claude/skills or ./.claude/skills). It overwrites any existing file,
// since re-running init is the upgrade path.
func installSkill(dir string) error {
	skillDir := filepath.Join(dir, "review-plan")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(path, skillMD, 0o644); err != nil {
		return err
	}
	return nil
}

// runInit implements the "mdreview init" subcommand: it installs the
// embedded review-plan skill to the user's Claude Code skills directory
// (~/.claude/skills), or to ./.claude/skills when --project is given.
func runInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	project := fs.Bool("project", false, "install to ./.claude/skills instead of the user-level ~/.claude/skills")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: mdreview init [--project]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	var dir string
	if *project {
		dir = filepath.Join(".", ".claude", "skills")
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "mdreview: %v\n", err)
			os.Exit(1)
		}
		dir = filepath.Join(home, ".claude", "skills")
	}

	if err := installSkill(dir); err != nil {
		fmt.Fprintf(os.Stderr, "mdreview: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "mdreview: installed skill to %s\n", filepath.Join(dir, "review-plan", "SKILL.md"))
}
