package httpadapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSurvivalSkillBundle_OnboardingContract(t *testing.T) {
	root := filepath.Join("..", "..", "..", "apps", "web", "public", "skills")
	const expectedVersion = "2.6.3"

	indexRaw := readSkillAsset(t, filepath.Join(root, "index.json"))
	var index struct {
		Skills []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Path    string `json:"path"`
		} `json:"skills"`
	}
	if err := json.Unmarshal([]byte(indexRaw), &index); err != nil {
		t.Fatalf("unmarshal index.json: %v", err)
	}

	var survivalEntry *struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Path    string `json:"path"`
	}
	for i := range index.Skills {
		if index.Skills[i].Name == "survival" {
			survivalEntry = &index.Skills[i]
			break
		}
	}
	if survivalEntry == nil {
		t.Fatalf("index.json missing survival entry")
	}
	if survivalEntry.Version != expectedVersion {
		t.Fatalf("index survival version = %q, want %q", survivalEntry.Version, expectedVersion)
	}
	if survivalEntry.Path != "survival/skill.md" {
		t.Fatalf("index survival path = %q, want %q", survivalEntry.Path, "survival/skill.md")
	}

	pkgRaw := readSkillAsset(t, filepath.Join(root, "survival", "package.json"))
	var pkg struct {
		Version string `json:"version"`
		Clawvival struct {
			Requirements struct {
				Automation struct {
					AutonomousCyclesEnabled bool `json:"autonomousCyclesEnabled"`
				} `json:"automation"`
			} `json:"requirements"`
		} `json:"clawvival"`
	}
	if err := json.Unmarshal([]byte(pkgRaw), &pkg); err != nil {
		t.Fatalf("unmarshal package.json: %v", err)
	}
	if pkg.Version != expectedVersion {
		t.Fatalf("package version = %q, want %q", pkg.Version, expectedVersion)
	}
	if !pkg.Clawvival.Requirements.Automation.AutonomousCyclesEnabled {
		t.Fatalf("package.json must declare autonomous cycles in clawvival.requirements.automation")
	}

	skillDoc := readSkillAsset(t, filepath.Join(root, "survival", "skill.md"))
	for _, needle := range []string{
		"3-Minute Onboarding",
		"Newcomer Milestones",
		"FAQ",
		"continue",
		"Self-Generated Stage Goal Template",
	} {
		if !strings.Contains(skillDoc, needle) {
			t.Fatalf("skill.md missing onboarding marker %q", needle)
		}
	}

	heartbeatDoc := readSkillAsset(t, filepath.Join(root, "survival", "HEARTBEAT.md"))
	if !strings.Contains(heartbeatDoc, "Newcomer-First Execution Chain") {
		t.Fatalf("HEARTBEAT.md missing onboarding section")
	}

	messagingDoc := readSkillAsset(t, filepath.Join(root, "survival", "MESSAGING.md"))
	if !strings.Contains(messagingDoc, "First-Turn Onboarding Reply Template") {
		t.Fatalf("MESSAGING.md missing onboarding template section")
	}

	if !strings.Contains(indexRaw, "autonomous_cycles_enabled") {
		t.Fatalf("index.json missing autonomous cycle registry metadata")
	}
	if !strings.Contains(indexRaw, "credentials_required") {
		t.Fatalf("index.json missing local credential requirement metadata")
	}
	if !strings.Contains(skillDoc, "\"autonomous_cycles_enabled\":true") {
		t.Fatalf("skill.md frontmatter metadata missing autonomous cycle marker")
	}
}

func readSkillAsset(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
