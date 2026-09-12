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

// seasonsReference is the one neutral file that may name a game. Which game is
// running outlives a swap. A game's mechanics do not. See AGENTS.md.
const seasonsReference = "sirens-echo-knowledge/references/game-seasons.md"

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

// neutralTexts returns one entry per file a neutral root contributes, keyed by
// path, so a check reports the file rather than the root that contains it.
func neutralTexts(t *testing.T, root string) map[string]string {
	t.Helper()
	dir := filepath.Join("..", "..", ".agents", "skills", root)
	texts := make(map[string]string)
	pack, err := LoadSkillpack([]string{dir})
	if err != nil {
		t.Fatalf("load %s: %v", root, err)
	}
	for path, body := range packSections(pack) {
		texts[path] = body
	}
	references, err := LoadSkillReferences([]string{dir})
	if err != nil {
		t.Fatalf("load %s references: %v", root, err)
	}
	for _, reference := range references {
		texts[reference.Path] = reference.Body
	}
	return texts
}

// packSections splits a rendered pack back into its files. A check that reports
// the file beats one searching a blob spanning several of them.
func packSections(pack string) map[string]string {
	sections := make(map[string]string)
	for _, section := range strings.Split(pack, "\n## Source: ")[1:] {
		header, body, _ := strings.Cut(section, "\n")
		sections[strings.TrimSpace(header)] = body
	}
	return sections
}

// The load-bearing one. A game named outside its focus survives the swap, and
// the reply that comes out is confidently about the wrong world.
func TestNeutralRootsNameNoGame(t *testing.T) {
	t.Parallel()
	games := gameFocusesOnDisk(t)
	for _, root := range neutralRoots {
		for path, body := range neutralTexts(t, root) {
			if strings.HasSuffix(path, seasonsReference) {
				continue
			}
			for _, game := range games {
				if wordPattern(game).MatchString(body) {
					t.Errorf("the neutral file %s names %s, which outlives a swap. Move a "+
						"mechanic into that game's focus root, or a schedule into %s",
						path, game, seasonsReference)
				}
			}
		}
	}
}

// The exemption earns its place only while the file it names carries the
// schedule. An exemption over an empty file is a hole nobody is watching.
func TestTheSeasonsExemptionGuardsSomething(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", ".agents", "skills", seasonsReference))
	if err != nil {
		t.Fatalf("read the seasons reference: %v", err)
	}
	text := string(raw)
	if !inlineAlways(text) {
		t.Error("the schedule must be inline, since a fetched season is one the model may answer without")
	}
	named := 0
	for _, game := range gameFocusesOnDisk(t) {
		if wordPattern(game).MatchString(text) {
			named++
		}
	}
	if named == 0 {
		t.Errorf("%s names no game, so exempting it from TestNeutralRootsNameNoGame guards nothing",
			seasonsReference)
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
	sections := packSections(pack)
	for root, game := range gameFocusesOnDisk(t) {
		matched := false
		for path, body := range sections {
			// The schedule names a game on purpose, so it cannot answer whether
			// that game's knowledge reached the prompt.
			if strings.HasSuffix(path, seasonsReference) {
				continue
			}
			if wordPattern(game).MatchString(body) {
				matched = true
				break
			}
		}
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
