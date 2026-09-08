package community

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func roleGraph(t *testing.T) (string, RoleGraph) {
	t.Helper()
	path := filepath.Join("..", "..", "agent", "compose", "roles.kdl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw), ParseRoleGraph(string(raw))
}

func catalogRoot(t *testing.T) string {
	t.Helper()
	root := os.Getenv("AOS_CATALOG")
	if root == "" {
		t.Skip("set AOS_CATALOG to an agentic-os checkout to expand the graph")
	}
	return root
}

// What only a real catalogue answers: whether roles.kdl still matches anything.
// This skips without AOS_CATALOG, so enforcement lives in composedeny_test.go.
func TestGraphPatternsNeverReachDeniedSources(t *testing.T) {
	t.Parallel()
	catalog := catalogRoot(t)
	_, graph := roleGraph(t)
	for role := range graph.Patterns {
		if _, err := ExpandRole([]string{catalog}, role, graph); err != nil {
			t.Fatalf("role %q: %v", role, err)
		}
	}
	widened := RoleGraph{Patterns: map[string][]string{"creator": {"kai-*"}}}
	if _, err := ExpandRole([]string{catalog}, "creator", widened); err == nil {
		t.Fatal("expansion accepted a pattern reaching the private kai- family")
	}
	social := RoleGraph{Patterns: map[string][]string{"creator": {"personal-preference-social"}}}
	if _, err := ExpandRole([]string{catalog}, "creator", social); err == nil {
		t.Fatal("expansion accepted the one member-owned preference source")
	}
	// A family glob must survive a denied member by dropping it, or a wider
	// catalogue would make every broad pattern unusable.
	brush := RoleGraph{Patterns: map[string][]string{"creator": {"personal-preference-*"}}}
	admitted, dropped, err := ExpandRoleWithExclusions([]string{catalog}, "creator", brush)
	if err != nil {
		t.Fatalf("family glob failed on a denied member: %v", err)
	}
	if _, leaked := admitted["personal-preference-social"]; leaked {
		t.Fatal("a denied source survived a family glob")
	}
	if len(admitted) == 0 {
		t.Fatal("the family glob dropped everything")
	}
	_ = dropped
	empty := RoleGraph{Patterns: map[string][]string{"creator": {"writing-nothing-*"}}}
	if _, err := ExpandRole([]string{catalog}, "creator", empty); err == nil {
		t.Fatal("expansion accepted a pattern matching nothing")
	}
	// A wider layer passes several catalogues, and one name owned by two of them
	// is ambiguous rather than a silent first-wins.
	if _, err := ExpandRole([]string{catalog, catalog}, "creator", graph); err == nil {
		t.Fatal("expansion accepted the same skill from two catalogues")
	}
}

// A global naming a private repository would hand a public agent private
// context without any composed-skill pattern being involved.
func TestGraphGlobalsAreNeverPrivate(t *testing.T) {
	t.Parallel()
	_, graph := roleGraph(t)
	// The graph declares no provider today. Sources reach a bundle through the
	// request's declaration entries, which is the reviewed path. See #126.
	if len(graph.Globals) != 0 || len(graph.Repositories) != 0 {
		t.Errorf("the graph globalizes %v from %v", graph.Globals, graph.Repositories)
	}
	// The guard stays wired regardless, so re-adding a block cannot quietly
	// globalize a private repository.
	guarded := RoleGraph{
		Repositories: map[string]string{"lore": "coilysiren/lore"},
		Globals:      []string{"lore"},
	}
	if err := CheckGraphGlobals(guarded); err == nil {
		t.Error("a private global was accepted")
	}
	if err := CheckGraphGlobals(RoleGraph{
		Repositories: map[string]string{"echo": "coilyco-gaming/sirens-echo"},
		Globals:      []string{"echo"},
	}); err != nil {
		t.Errorf("a public global was refused: %v", err)
	}
	if err := CheckGraphGlobals(RoleGraph{Globals: []string{"absent"}}); err == nil {
		t.Error("a global naming no repository was accepted")
	}
}

// The declaration is build output. Nothing tracked should carry those paths.
func TestDeclarationIsNeverTracked(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"aos-public.kdl", "skills"} {
		path := filepath.Join("..", "..", "agent", "compose", name)
		if _, err := os.Stat(path); err == nil {
			t.Errorf("%s is build output and must not be committed", path)
		}
	}
}

// root= hands selection to the source repository's own roles.kdl, which for the
// private catalogue deliberately binds Kai's career and LinkedIn context.
func TestRequestSourcesAlwaysUseADeclaration(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "agent", "compose", "request.kdl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	pattern := regexp.MustCompile(`^\s*source\s+"([^"]+)"\s+(\w+)=`)
	sources := 0
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		if match := pattern.FindStringSubmatch(line); match != nil {
			sources++
			if match[2] != "declaration" {
				t.Errorf("source %q uses %s=, want declaration=", match[1], match[2])
			}
		}
	}
	if sources == 0 {
		t.Fatal("request names no source")
	}
}

// A composing profile must show the bundle, and the neutral profile must not.
// See docs/sirens-echo-compose.md for why the anchors changed.
func TestComposedProfileRequiresItsBundleSurface(t *testing.T) {
	t.Parallel()
	definition := Definition{
		Identity:      "Sirens Deep of Coilyco",
		AuditRole:     "general",
		ResponseStyle: ResponseStyleSocial,
		Composed:      true,
	}
	composed, err := LoadBundle(filepath.Join(writeFixtureBundle(t), "creator"))
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	prompt := BuildSystemPrompt(definition, PlaceholderPrincipal, composed, "general CoilyCo policy")
	for _, expected := range []string{
		"<composed-identity>",
		"Agent-compose assigned",
		"## Personality meld",
		"**Role skill //",
		"CoilyCo house style",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("composed prompt missing %q", expected)
		}
	}
	if err := ValidateSystemPrompt(definition, PlaceholderPrincipal, prompt); err != nil {
		t.Fatalf("ValidateSystemPrompt: %v", err)
	}
	// A bundle that failed to compose must stop startup, not answer neutrally.
	bare := BuildSystemPrompt(definition, PlaceholderPrincipal, "", "general CoilyCo policy")
	if err := ValidateSystemPrompt(definition, PlaceholderPrincipal, bare); err == nil {
		t.Fatal("validator accepted a composing profile with no bundle")
	}
	// A profile that selected no bundle must never carry one, whatever its voice.
	// This is the leak check; the neutrality check moved to the test below.
	neutral := Definition{Identity: "Sirens Echo", AuditRole: "community", ResponseStyle: ResponseStyleNeutral}
	leaked := BuildSystemPrompt(neutral, PlaceholderPrincipal, composed, "approved Sirens facts")
	if err := ValidateSystemPrompt(neutral, PlaceholderPrincipal, leaked); err == nil {
		t.Fatal("validator accepted a bundle in a profile that selected none")
	}
}

// Echo composes ops and still answers neutrally, a pair the validator used to
// make unsatisfiable. See docs/sirens-echo-identity.md.
func TestNeutralProfileComposesARoleWithoutTakingItsVoice(t *testing.T) {
	t.Parallel()
	definition := Definition{
		Identity:      "Sirens Echo",
		AuditRole:     "community",
		ResponseStyle: ResponseStyleNeutral,
		Channel:       "#bots",
		Composed:      true,
	}
	composed, err := LoadBundle(filepath.Join(writeFixtureBundle(t), "ops"))
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	prompt := BuildSystemPrompt(definition, PlaceholderPrincipal, composed, "approved Sirens facts")
	// Both halves, in one prompt: the bundle the role selected and the voice
	// rules the profile keeps.
	for _, expected := range []string{
		"<composed-identity>",
		"## Personality meld",
		"**Role skill //",
		"Do not adopt or express a personality",
		"Use neutral, concise, impersonal language",
		"the response rules in this prompt",
		"never spoken or written",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("composed neutral prompt missing %q", expected)
		}
	}
	if err := ValidateSystemPrompt(definition, PlaceholderPrincipal, prompt); err != nil {
		t.Fatalf("ValidateSystemPrompt: %v", err)
	}
	// The precedence clause is what makes the meld admissible, so a prompt that
	// carries a bundle without it is the failure this replaced, not a pass.
	stripped := strings.Replace(prompt, composedVoicePolicy, "", 1)
	if err := ValidateSystemPrompt(definition, PlaceholderPrincipal, stripped); err == nil {
		t.Fatal("validator accepted a composed neutral prompt with no stated precedence")
	}
	// A social profile takes its voice from the bundle, so it renders no clause
	// and gains no neutral prohibition to reconcile.
	social := definition
	social.ResponseStyle = ResponseStyleSocial
	if strings.Contains(
		BuildSystemPrompt(social, PlaceholderPrincipal, composed, "general CoilyCo policy"),
		composedVoicePolicy,
	) {
		t.Fatal("social prompt rendered the neutral precedence clause")
	}
}

// writeFixtureBundle builds a minimal materialized bundle per baked role. The
// tree is Markdown, which documentation-layout bounds to docs, so it generates.
func writeFixtureBundle(t *testing.T) string {
	t.Helper()
	bundles := t.TempDir()
	for _, role := range fixtureRoles {
		skill := filepath.Join(bundles, role, "content", "skills", "aos-public", "writing-kai-voice")
		if err := os.MkdirAll(skill, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		for path, body := range map[string]string{
			filepath.Join(bundles, role, "manifest.json"):              fmt.Sprintf(`{"format":"agent-compose.bundle.v0.1","role":%q}`, role),
			filepath.Join(bundles, role, "content", "instructions.md"): fixtureCard(role),
			filepath.Join(skill, "SKILL.md"):                           fixtureSkill,
		} {
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatalf("WriteFile %s: %v", path, err)
			}
		}
	}
	return bundles
}

// Both lanes compose now, so the fixture bakes both roles rather than the one
// role that used to be the only composing profile.
var fixtureRoles = []string{"creator", "ops"}

// useFixtureBundles points a deployment at the fixture tree and names its role.
// Naming it is not optional in the runtime either: see resolveBundlePath.
func useFixtureBundles(t *testing.T, role string) {
	t.Helper()
	t.Setenv("SIRENS_ECHO_BUNDLE_DIR", writeFixtureBundle(t))
	t.Setenv("SIRENS_ECHO_ROLE", role)
}

func fixtureCard(role string) string {
	return fmt.Sprintf(`# Role instructions

Agent-compose assigned you the `+"`%s`"+` role from the caller's compose request.

**Role skill // `+"`role-%s`"+`**
**Agent // Gem (they)**

## Personality meld

Fixture meld for the offline harness.
`, role, role)
}

const fixtureSkill = `---
name: writing-kai-voice
---

# CoilyCo house style

Fixture body.
`

// An entry for a role the roster lost grants its skills to nothing, which looks
// exactly like a role deliberately left bare. sirens-echo#1147.
func TestGraphRolesMustExistInTheRoster(t *testing.T) {
	t.Parallel()
	graph := RoleGraph{Patterns: map[string][]string{
		"creator":  {"writing-*"},
		"engineer": {"coding-*"},
	}}
	err := CheckGraphRoles(graph, []string{"platform", "devrel", "librarian"})
	if err == nil {
		t.Fatal("an entry naming no roster role was accepted")
	}
	for _, want := range []string{"creator", "engineer", "platform"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
	if err := CheckGraphRoles(graph, []string{"creator", "engineer", "librarian"}); err != nil {
		t.Errorf("a graph matching the roster was refused: %v", err)
	}
	// A roster role with no entry is the deliberately bare case and stays legal.
	if err := CheckGraphRoles(RoleGraph{}, []string{"librarian"}); err != nil {
		t.Errorf("a roster role with no entry was refused: %v", err)
	}
}
