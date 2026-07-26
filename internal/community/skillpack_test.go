package community

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSkillpackLoadsSkillAndReferences(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..", ".agents", "skills", "sirens-echo-community")
	pack, err := LoadSkillpack([]string{root})
	if err != nil {
		t.Fatalf("LoadSkillpack: %v", err)
	}
	for _, expected := range []string{
		"Sirens Echo community host",
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
