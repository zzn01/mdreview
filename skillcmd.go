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

// resolveSkillsDir picks the skills root to install into: an explicit dir
// if given (any agent supporting the SKILL.md standard), else the local
// ./.claude/skills when project is set, else the user-level ~/.claude/skills.
func resolveSkillsDir(project bool, dir string) (string, error) {
	if dir != "" {
		return dir, nil
	}
	if project {
		return filepath.Join(".", ".claude", "skills"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "skills"), nil
}

// runInit implements the "mdreview init" subcommand: it installs the
// embedded review-plan skill to a skills directory chosen by
// resolveSkillsDir.
func runInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	project := fs.Bool("project", false, "install to ./.claude/skills instead of the user-level ~/.claude/skills")
	dirFlag := fs.String("dir", "", "install to this skills directory (for agents other than Claude Code)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: mdreview init [--project] [--dir <skills-dir>]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	dir, err := resolveSkillsDir(*project, *dirFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdreview: %v\n", err)
		os.Exit(1)
	}

	if err := installSkill(dir); err != nil {
		fmt.Fprintf(os.Stderr, "mdreview: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "mdreview: installed skill to %s\n", filepath.Join(dir, "review-plan", "SKILL.md"))
}
