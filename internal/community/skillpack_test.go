package community

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSkillpackLoadsSkillAndReferences(t *testing.T) {
	t.Parallel()
	communityRoot := filepath.Join("..", "..", ".agents", "skills", "sirens-echo-community")
	knowledgeRoot := filepath.Join("..", "..", ".agents", "skills", "sirens-echo-knowledge")
	pack, err := LoadSkillpack([]string{communityRoot, knowledgeRoot})
	if err != nil {
		t.Fatalf("LoadSkillpack: %v", err)
	}
	for _, expected := range []string{
		"Sirens Echo response policy",
		"Keep responses neutral",
		"Sirens community knowledge",
		"#bots",
	} {
		if !strings.Contains(pack, expected) {
			t.Fatalf("skillpack missing %q", expected)
		}
	}
	if strings.Contains(pack, "name: sirens-echo-community") {
		t.Fatal("skillpack leaked YAML frontmatter")
	}
	if strings.Contains(pack, "openai.yaml") {
		t.Fatal("skillpack loaded UI metadata")
	}
}

func TestLoadSkillpackLoadsSocialPolicySeparately(t *testing.T) {
	t.Parallel()
	socialRoot := filepath.Join("..", "..", ".agents", "skills", "sirens-echo-social")
	knowledgeRoot := filepath.Join("..", "..", ".agents", "skills", "sirens-echo-knowledge")
	pack, err := LoadSkillpack([]string{knowledgeRoot, socialRoot})
	if err != nil {
		t.Fatalf("LoadSkillpack: %v", err)
	}
	for _, expected := range []string{
		"Sirens Echo social response policy",
		"Sound like a real community host",
		"Sirens community knowledge",
	} {
		if !strings.Contains(pack, expected) {
			t.Fatalf("social skillpack missing %q", expected)
		}
	}
	if strings.Contains(pack, "Keep responses neutral") {
		t.Fatal("social skillpack loaded the neutral response policy")
	}
}
