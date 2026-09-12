package community

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func trackedTaxonomy(t *testing.T) ContentTaxonomy {
	t.Helper()
	taxonomy, err := LoadContentTaxonomy(
		filepath.Join("..", "..", "agent", "content-classes.yaml"),
	)
	if err != nil {
		t.Fatalf("LoadContentTaxonomy: %v", err)
	}
	return taxonomy
}

// Kai's requirement is that the list cover every content type theoretically
// possible to communicate. That needs allowed classes and a catch-all.
func TestTrackedTaxonomyIsActuallyClosed(t *testing.T) {
	t.Parallel()
	taxonomy := trackedTaxonomy(t)
	catchAll, known := taxonomy.Lookup(ContentClassOther)
	if !known || catchAll.Deny {
		t.Fatal("the catch-all is missing or denies, so the set is not closed")
	}
	allowed := 0
	for _, class := range taxonomy.Classes {
		if !class.Deny {
			allowed++
		}
	}
	if allowed < 2 {
		t.Fatalf("only %d allowed classes, so an ordinary request has nowhere to land", allowed)
	}
}

// The tie rule from the recorded decision. A bedtime story trips creative
// long-form and minor suspicion, and naming the ordinary one leaks the signal.
func TestSensitiveWinsTies(t *testing.T) {
	t.Parallel()
	taxonomy := trackedTaxonomy(t)
	for _, order := range [][]string{
		{"creative-long-form", "minor-suspected"},
		{"minor-suspected", "creative-long-form"},
	} {
		class, blocked, err := taxonomy.Verdict(order)
		if err != nil || !blocked {
			t.Fatalf("Verdict(%v) = %v, blocked %v, err %v", order, class.ID, blocked, err)
		}
		if !class.Sensitive || class.ID != "minor-suspected" {
			t.Fatalf("Verdict(%v) resolved to %s, which names an ordinary category", order, class.ID)
		}
	}
}

// Recorded as an ordinary category, so the reason may be named. Pairing it with
// a sensitive class is where the sensitive branch has to win.
func TestAnOrdinaryDenialIsNamedAndLosesToSensitive(t *testing.T) {
	t.Parallel()
	taxonomy := trackedTaxonomy(t)
	class, blocked, err := taxonomy.Verdict([]string{"medical-legal-advice"})
	if err != nil || !blocked {
		t.Fatalf("Verdict = %v, blocked %v, err %v", class.ID, blocked, err)
	}
	if class.Sensitive {
		t.Fatal("medical-legal-advice is sensitive, so a decline could not name its reason")
	}
	paired, blocked, err := taxonomy.Verdict([]string{"medical-legal-advice", "minor-suspected"})
	if err != nil || !blocked || paired.ID != "minor-suspected" {
		t.Fatalf("paired with a sensitive class resolved to %v", paired.ID)
	}
}

func TestVerdictAllowsAndRejectsUnknown(t *testing.T) {
	t.Parallel()
	taxonomy := trackedTaxonomy(t)
	if _, blocked, err := taxonomy.Verdict([]string{"game-gameplay", "small-talk"}); err != nil || blocked {
		t.Fatalf("allowed classes blocked: %v, %v", blocked, err)
	}
	if _, _, err := taxonomy.Verdict([]string{"not-a-class"}); err == nil {
		t.Fatal("a class outside the list was accepted")
	}
}

// The must-not-fire half. A topic filter that catches a bedtime story and also
// catches the game's own sleep mechanics is worse than no filter.
func TestAllowedClassesCoverTheOrdinaryQuestions(t *testing.T) {
	t.Parallel()
	taxonomy := trackedTaxonomy(t)
	for _, id := range []string{
		"game-gameplay",       // how do beds work in this game
		"community-logistics", // did that message read as hostile
		"general-knowledge",   // what is algebra
		"service-capability",  // what can you do
		"small-talk",          // hello
		"repository-request",  // list the open issues
		"server-operations",   // is the server up
	} {
		class, known := taxonomy.Lookup(id)
		if !known {
			t.Fatalf("%s is not in the taxonomy", id)
		}
		if class.Deny {
			t.Fatalf("%s denies, which would block an ordinary request", id)
		}
	}
}

func TestClassifierPromptCarriesEveryClass(t *testing.T) {
	t.Parallel()
	taxonomy := trackedTaxonomy(t)
	prompt := ContentClassifierPrompt(taxonomy)
	for _, class := range taxonomy.Classes {
		if !strings.Contains(prompt, class.ID) {
			t.Fatalf("the prompt omits %s, so the model cannot choose it", class.ID)
		}
	}
	if !strings.Contains(prompt, "Use `other` when none of the rest fit") {
		t.Fatal("the prompt does not tell the model what to do with an unlisted request")
	}
}

func writeAndLoadTaxonomy(t *testing.T, body string) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "classes.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write taxonomy: %v", err)
	}
	_, err := LoadContentTaxonomy(path)
	return err
}

func TestLoadContentTaxonomyRefusesAnUnusableList(t *testing.T) {
	t.Parallel()
	head := "schema: sirens-discord-ops.content-classes.v1\nclasses:\n"
	for name, body := range map[string]string{
		"no catch-all":          head + "  - id: game-gameplay\n    summary: game\n  - id: nsfw\n    summary: n\n    deny: true\n",
		"catch-all denies":      head + "  - id: other\n    summary: rest\n    deny: true\n  - id: game-gameplay\n    summary: game\n",
		"sensitive but allowed": head + "  - id: other\n    summary: rest\n  - id: game-gameplay\n    summary: game\n  - id: nsfw\n    summary: n\n    sensitive: true\n",
		"only the catch-all":    head + "  - id: other\n    summary: rest\n",
		"duplicate":             head + "  - id: other\n    summary: rest\n  - id: game-gameplay\n    summary: game\n  - id: game-gameplay\n    summary: again\n",
	} {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := writeAndLoadTaxonomy(t, body); err == nil {
				t.Fatal("expected the taxonomy to fail loading")
			}
		})
	}
}
