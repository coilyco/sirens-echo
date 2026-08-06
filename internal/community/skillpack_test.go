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

func TestLoadSkillpackLoadsCoilyCoPolicySeparately(t *testing.T) {
	t.Parallel()
	generalRoot := filepath.Join("..", "..", ".agents", "skills", "coilyco-general")
	pack, err := LoadSkillpack([]string{generalRoot})
	if err != nil {
		t.Fatalf("LoadSkillpack: %v", err)
	}
	for _, expected := range []string{
		"CoilyCo general-purpose response policy",
		"Start from the request",
		"General-purpose means topic-neutral and extensible",
	} {
		if !strings.Contains(pack, expected) {
			t.Fatalf("general skillpack missing %q", expected)
		}
	}
	for _, forbidden := range []string{"Sirens community knowledge", "current Eco"} {
		if strings.Contains(pack, forbidden) {
			t.Fatalf("general skillpack retained %q", forbidden)
		}
	}
}
