package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSkillsRoot_UsesEnv(t *testing.T) {
	t.Setenv("SKILLS_ROOT", "/tmp/custom-skills")
	if got := resolveSkillsRoot(); got != "/tmp/custom-skills" {
		t.Fatalf("resolveSkillsRoot()=%q want %q", got, "/tmp/custom-skills")
	}
}

func TestResolveSkillsRoot_UsesRootSkillsWhenPresent(t *testing.T) {
	t.Setenv("SKILLS_ROOT", "")

	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "index.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write skills index: %v", err)
	}

	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(prevWD)
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if got := resolveSkillsRoot(); got != "./skills" {
		t.Fatalf("resolveSkillsRoot()=%q want %q", got, "./skills")
	}
}

func TestResolveSkillsRoot_FallsBackToWebPublic(t *testing.T) {
	t.Setenv("SKILLS_ROOT", "")

	dir := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(prevWD)
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if got := resolveSkillsRoot(); got != "./apps/web/public/skills" {
		t.Fatalf("resolveSkillsRoot()=%q want %q", got, "./apps/web/public/skills")
	}
}
