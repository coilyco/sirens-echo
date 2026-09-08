package community

import (
	"os"
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
		"Sirens Echo response rules",
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
		"CoilyCo general-purpose response rules",
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

// A composed agent-compose source ships COMPOSED.md rather than SKILL.md, and
// a role bundle is unreadable without it.
func TestLoadSkillpackReadsComposedSources(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	composed := filepath.Join(root, "coilyco-favorites")
	if err := os.MkdirAll(filepath.Join(composed, "references"), 0o755); err != nil {
		t.Fatalf("prepare composed root: %v", err)
	}
	write := func(path, body string) {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	write(
		filepath.Join(composed, "COMPOSED.md"),
		"---\nname: coilyco-favorites\ndescription: House colours.\n---\n\n"+
			"# Favorites\n\nPurple and black.\n",
	)
	write(filepath.Join(composed, "references", "detail.md"), "Reference detail.\n")

	ordinary := filepath.Join(root, "ordinary")
	if err := os.MkdirAll(ordinary, 0o755); err != nil {
		t.Fatalf("prepare ordinary root: %v", err)
	}
	write(filepath.Join(ordinary, "SKILL.md"), "# Ordinary\n\nOrdinary body.\n")

	pack, err := LoadSkillpack([]string{composed, ordinary})
	if err != nil {
		t.Fatalf("LoadSkillpack: %v", err)
	}
	// #971 inverted this. An entrypoint is indexed by its own description and
	// its body is fetched, so the pack carries the summary and not the text.
	if !strings.Contains(pack, "House colours.") {
		t.Fatalf("skillpack missing the composed source's description:\n%s", pack)
	}
	for _, body := range []string{"Purple and black.", "Ordinary body."} {
		if strings.Contains(pack, body) {
			t.Fatalf("skillpack inlined an entrypoint body %q:\n%s", body, pack)
		}
	}
	// An entrypoint with no description still has to be findable, so it falls
	// back to its first heading rather than vanishing from the index.
	if !strings.Contains(pack, "Ordinary") {
		t.Fatalf("an entrypoint without a description left no index line:\n%s", pack)
	}
	// The reference is fetchable rather than inlined, and the pack says so, or
	// the model has no way to know it exists. See sirens-echo#859.
	if strings.Contains(pack, "Reference detail.") {
		t.Errorf("a reference was inlined:\n%s", pack)
	}
	if !strings.Contains(pack, "references/detail.md") {
		t.Errorf("the pack does not index the reference:\n%s", pack)
	}
	references, err := LoadSkillReferences([]string{composed, ordinary})
	if err != nil {
		t.Fatalf("LoadSkillReferences: %v", err)
	}
	// Three now, not one: both entrypoints joined the reference alongside the
	// reference itself, which is what makes an entrypoint fetchable at all.
	if len(references) != 3 {
		t.Fatalf("references = %#v", references)
	}
	served := map[string]string{}
	for _, reference := range references {
		served[filepath.Base(filepath.Dir(reference.Path))+"/"+filepath.Base(reference.Path)] =
			reference.Title
	}
	for path, title := range map[string]string{
		"coilyco-favorites/COMPOSED.md": "House colours.",
		"references/detail.md":          "reference material",
		"ordinary/SKILL.md":             "Ordinary",
	} {
		if served[path] != title {
			t.Errorf("%s is indexed as %q, want %q", path, served[path], title)
		}
	}
	if strings.Contains(pack, "name: coilyco-favorites") {
		t.Fatal("composed frontmatter reached the model context")
	}
}
