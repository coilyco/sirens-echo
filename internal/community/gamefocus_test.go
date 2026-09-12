package community

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A game focus is one skill root carrying everything true of one game. Exactly
// one loads and the rest sit parked. See AGENTS.md, the game focus section.

const gameFocusPrefix = "sirens-game-"

// The swap is only real while there is somewhere to swap to, so a second focus
// is the feature rather than spare content.
const minGameFocuses = 2

// neutralRoots must name no game: a fact written here rather than into a focus
// survives the swap and then answers for the wrong world.
var neutralRoots = []string{
	"coilyco-org",
	"sirens-echo-community",
	"sirens-echo-knowledge",
	"sirens-echo-science",
}

// gameFocusesOnDisk maps each focus root to the game name its slug declares.
func gameFocusesOnDisk(t *testing.T) map[string]string {
	t.Helper()
	focuses := make(map[string]string)
	for _, root := range skillRootsOnDisk(t) {
		if !strings.HasPrefix(root, gameFocusPrefix) {
			continue
		}
		focuses[root] = gameNameOf(root)
	}
	if len(focuses) < minGameFocuses {
		t.Fatalf("%d game focuses on disk, want at least %d: one focus is not a swap",
			len(focuses), minGameFocuses)
	}
	return focuses
}

// gameNameOf titles the slug tail, so adding a focus extends every check here
// without editing a list of names.
func gameNameOf(root string) string {
	words := strings.Split(strings.TrimPrefix(root, gameFocusPrefix), "-")
	for i, word := range words {
		if word == "" {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

// activeGameFocus resolves the one focus the community profile names.
func activeGameFocus(t *testing.T) string {
	t.Helper()
	definition, err := LoadDefinition(filepath.Join("..", "..", "agents", "echo", "definition.yaml"))
	if err != nil {
		t.Fatalf("load echo definition: %v", err)
	}
	named := make([]string, 0, 1)
	for _, root := range definition.LocalSkillRoots {
		if base := filepath.Base(root); strings.HasPrefix(base, gameFocusPrefix) {
			named = append(named, base)
		}
	}
	if len(named) != 1 {
		t.Fatalf("echo names %d game focuses (%s), want exactly one: two focuses answer "+
			"for two worlds and none leaves the game unstated",
			len(named), strings.Join(named, ", "))
	}
	return named[0]
}

func TestExactlyOneGameFocusIsActive(t *testing.T) {
	t.Parallel()
	active := activeGameFocus(t)
	if _, ok := gameFocusesOnDisk(t)[active]; !ok {
		t.Fatalf("echo names %s, which is not a focus root on disk", active)
	}
}

// Only the community profile answers for a game. The general profile starts
// from the request and would contradict its own charter by assuming one.
func TestNoOtherProfileNamesAGameFocus(t *testing.T) {
	t.Parallel()
	for name, path := range profileDefinitions(t) {
		if name == "echo" {
			continue
		}
		definition, err := LoadDefinition(path)
		if err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
		for _, root := range definition.LocalSkillRoots {
			if strings.HasPrefix(filepath.Base(root), gameFocusPrefix) {
				t.Errorf("%s names the game focus %s, so it assumes a subject it has none", name, root)
			}
		}
	}
}

// A parked focus that has decayed is not a swap, it is a rebuild discovered at
// the worst moment. Every focus carries the same shape whether it loads or not.
func TestEveryGameFocusStaysSwappable(t *testing.T) {
	t.Parallel()
	for root, game := range gameFocusesOnDisk(t) {
		dir := filepath.Join("..", "..", ".agents", "skills", root)
		raw, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
		if err != nil {
			t.Errorf("%s: no SKILL.md, so naming it in a definition would fail startup: %v", root, err)
			continue
		}
		text := string(raw)
		if !inlineAlways(text) {
			t.Errorf("%s: the focus must be inline, a fetched game subject is one the model may skip", root)
		}
		if frontmatterValue(text, "description") == "" {
			t.Errorf("%s: no description", root)
		}
		if !wordPattern(game).MatchString(text) {
			t.Errorf("%s: its SKILL.md never names %s, so the focus does not state its own subject", root, game)
		}
		if _, err := os.Stat(filepath.Join(dir, "references", "links.md")); err != nil {
			t.Errorf("%s: no references/links.md, so a reply has no approved URL registry for %s", root, game)
		}
	}
}

// The load-bearing one. A game named outside its focus survives the swap, and
// the reply that comes out is confidently about the wrong world.
func TestNeutralRootsNameNoGame(t *testing.T) {
	t.Parallel()
	games := gameFocusesOnDisk(t)
	for _, root := range neutralRoots {
		pack, err := LoadSkillpack([]string{filepath.Join("..", "..", ".agents", "skills", root)})
		if err != nil {
			t.Fatalf("load %s: %v", root, err)
		}
		references, err := LoadSkillReferences([]string{filepath.Join("..", "..", ".agents", "skills", root)})
		if err != nil {
			t.Fatalf("load %s references: %v", root, err)
		}
		text := pack
		for _, reference := range references {
			text += "\n" + reference.Body
		}
		for _, game := range games {
			if wordPattern(game).MatchString(text) {
				t.Errorf("the neutral root %s names %s, which outlives a swap. Move the fact "+
					"into that game's focus root", root, game)
			}
		}
	}
}

// Rendered rather than declared. A root named in a definition and a root whose
// text reached the prompt are different claims, and the swap is about the text.
func TestOnlyTheActiveGameReachesThePrompt(t *testing.T) {
	t.Parallel()
	active := activeGameFocus(t)
	definition, err := LoadDefinition(filepath.Join("..", "..", "agents", "echo", "definition.yaml"))
	if err != nil {
		t.Fatalf("load echo definition: %v", err)
	}
	pack, err := LoadSkillpack(prefixedRoots(definition.LocalSkillRoots))
	if err != nil {
		t.Fatalf("load echo skillpack: %v", err)
	}
	for root, game := range gameFocusesOnDisk(t) {
		matched := wordPattern(game).MatchString(pack)
		if root == active && !matched {
			t.Errorf("the active focus %s is named but %s never reaches the prompt", root, game)
		}
		if root != active && matched {
			t.Errorf("%s is parked yet %s still reaches the prompt, so the swap did not take", root, game)
		}
	}
}

// wordPattern matches a game name as a word, so Eco does not match Echo and a
// path segment such as eco-app does not count as naming the game.
func wordPattern(game string) *regexp.Regexp {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(game) + `\b`)
}
