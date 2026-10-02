package community

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// capability.md tells the model what the service can do, and the model repeats
// it to members. A number that drifts from the code becomes a false claim.

const capabilityDocGlob = "../../.agents/skills/*/references/capability.md"

// Each lane carries its own copy, so every assertion runs against all of them.
// One guarded copy and one unguarded copy is the drift this file exists to stop.
func capabilityDocs(t *testing.T) map[string]string {
	t.Helper()
	paths, err := filepath.Glob(capabilityDocGlob)
	if err != nil || len(paths) == 0 {
		t.Fatalf("glob %s: %v, found %d", capabilityDocGlob, err, len(paths))
	}
	docs := make(map[string]string, len(paths))
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		docs[lane(path)] = string(body)
	}
	return docs
}

// lane names a copy by its skill root, which is what a failure needs to say.
func lane(path string) string {
	return filepath.Base(filepath.Dir(filepath.Dir(path)))
}

// A shared copy declines to name the ceilings at all, and says so in this
// phrase. Why, in docs/sirens-echo-prompt.md.
var deploymentSetBounds = regexp.MustCompile(`set per deployment`)

// Digits only, because the docs spell small counts as words and "a separate
// one on model calls" is not a figure.
var figureBesideABound = regexp.MustCompile(`(?i)\d+[^.\n]{0,40}(?:tool rounds?|model calls?)|(?:tool rounds?|model calls?)[^.\n]{0,40}\d+`)

// The doc's bounds follow the table rather than pinning it, so moving a number
// rewrites the sentence instead of failing. The outer budget fired in issue 258.
func TestTheCapabilityDocsFollowTheHarnessBounds(t *testing.T) {
	budget := maxToolRounds + maxResponseRepairs + budgetRaisesAllowed + 1
	rules := []struct {
		sentence *regexp.Regexp
		want     string
	}{
		{regexp.MustCompile(`At most \d+ tool rounds`), fmt.Sprintf("At most %d tool rounds", maxToolRounds)},
		{regexp.MustCompile(`A budget of \d+ model calls`), fmt.Sprintf("A budget of %d model calls", budget)},
	}
	paths, err := filepath.Glob(capabilityDocGlob)
	if err != nil || len(paths) == 0 {
		t.Fatalf("glob %s: %v, found %d", capabilityDocGlob, err, len(paths))
	}
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if deploymentSetBounds.Match(body) {
			if figure := figureBesideABound.Find(body); figure != nil {
				t.Errorf("%s says its ceilings are deployment-set and then names one in %q",
					lane(path), figure)
			}
			continue
		}
		updated := string(body)
		for _, rule := range rules {
			// A reworded sentence is the one thing this cannot follow, and a
			// silent no-op here is how the doc would start lying to the model.
			if !rule.sentence.MatchString(updated) {
				t.Errorf("%s has no sentence matching %s, so the bound cannot follow the table",
					lane(path), rule.sentence)
				continue
			}
			updated = rule.sentence.ReplaceAllString(updated, rule.want)
		}
		if updated == string(body) {
			continue
		}
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			t.Fatalf("rewrite %s: %v", path, err)
		}
		t.Logf("%s followed a moved number and was rewritten. Commit it.", path)
	}
}

// The turn answers from the results rather than failing, so a doc still
// promising a failure would send a member the wrong expectation.
func TestCapabilityDocDoesNotPromiseAnOutrightFailure(t *testing.T) {
	t.Parallel()
	for name, doc := range capabilityDocs(t) {
		if strings.Contains(doc, "fails outright") {
			t.Errorf("%s still says the turn fails outright; it degrades to an answer", name)
		}
	}
}

// The reply cap is asserted through ParseReply rather than a constant, because
// the limit is a literal there and behavior is what the member meets.
func TestCapabilityDocStatesTheRealReplyCap(t *testing.T) {
	t.Parallel()
	const stated = 1800
	for name, doc := range capabilityDocs(t) {
		if !strings.Contains(doc, "1800") {
			t.Errorf("%s does not name the %d character reply cap", name, stated)
		}
	}
	if _, err := ParseReply(strings.Repeat("a", stated)); err != nil {
		t.Errorf("a reply of exactly %d runes was rejected: %v", stated, err)
	}
	if _, err := ParseReply(strings.Repeat("a", stated+1)); err == nil {
		t.Errorf("a reply of %d runes was accepted, so the doc's cap is wrong", stated+1)
	}
}

// Every lane's context window has to match the one number the doc gives the
// model, or the doc is right for one deployment and wrong for the other.
func TestCapabilityDocStatesTheRealContextWindow(t *testing.T) {
	t.Parallel()
	docs := capabilityDocs(t)
	words := map[string]string{"10": "ten", "11": "eleven", "12": "twelve", "13": "thirteen"}
	definitions, err := filepath.Glob("../../agents/*/definition.yaml")
	if err != nil || len(definitions) == 0 {
		t.Fatalf("glob agent definitions: %v, found %d", err, len(definitions))
	}

	checked := 0
	for _, path := range definitions {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		value := yamlScalar(string(body), "max_context_messages")
		if value == "" {
			continue
		}
		checked++
		stated, known := words[value]
		if !known {
			t.Errorf("%s sets max_context_messages: %s, which this test cannot spell; extend the map",
				filepath.Base(path), value)
			continue
		}
		// A phrase, because a bare number word hides in prose — "ten" is inside
		// "softening". It stops before the noun, which differs per lane.
		phrase := stated + " recent"
		for name, doc := range docs {
			if !strings.Contains(doc, phrase) {
				t.Errorf("%s sets max_context_messages: %s but %s does not say %q",
					filepath.Base(path), value, name, phrase)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no agent definition declared max_context_messages")
	}
}

// The no-background-work claim holds only while the harness declares no job
// kind the deployment can enable, so the claim is tied to that closed set.
func TestCapabilityDocDoesNotDenyWorkTheHarnessCanRun(t *testing.T) {
	t.Parallel()
	claiming := make([]string, 0, 2)
	for name, doc := range capabilityDocs(t) {
		if strings.Contains(doc, "Nothing runs between requests") {
			claiming = append(claiming, name)
		}
	}
	if len(claiming) == 0 {
		t.Skip("no capability.md makes the no-background-work claim")
	}
	for kind := range JobKinds {
		if kind == "echo" || kind == "ward-exec" {
			continue
		}
		t.Errorf("JobKinds declares %q, which is work that outlives a reply; %v "+
			"still tell the model nothing runs between requests", kind, claiming)
	}
}

// The limits capability.md states are properties of the harness, not of one
// agent's persona, so every lane needs them and only one lane has them.
func TestCapabilityDocReachesEveryAgent(t *testing.T) {
	t.Parallel()

	// Every agent declaring skill roots must reach a capability reference. The
	// last exemption went when sirens-deep gained one.
	without := map[string]string{}

	definitions, err := filepath.Glob("../../agents/*/definition.yaml")
	if err != nil || len(definitions) == 0 {
		t.Fatalf("glob agent definitions: %v, found %d", err, len(definitions))
	}
	checked := 0
	for _, path := range definitions {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		definition := string(body)
		if !strings.Contains(definition, "local_skill_roots:") {
			continue
		}
		checked++
		name := filepath.Base(path)
		reaches := agentReachesCapabilityDoc(t, definition)
		issue, expected := without[name]
		switch {
		case reaches && expected:
			t.Errorf("%s now reaches capability.md; issue %s is fixed, so drop it "+
				"from the without map and let this assert the invariant", name, issue)
		case !reaches && !expected:
			t.Errorf("%s declares skill roots but none carries references/capability.md, "+
				"so its model is told none of the harness limits", name)
		}
	}
	if checked == 0 {
		t.Fatal("no agent definition declared local_skill_roots")
	}
}

// agentReachesCapabilityDoc reports whether any declared root holds the
// capability reference on disk.
func agentReachesCapabilityDoc(t *testing.T, body string) bool {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		root := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
		if !strings.HasPrefix(root, ".agents/skills/") {
			continue
		}
		if _, err := os.Stat(filepath.Join("../..", root, "references", "capability.md")); err == nil {
			return true
		}
	}
	return false
}

// repoSourceLink is where the capability docs send a reader for this source. The
// module path no longer spells this URL, so it is stated here, not read from go.mod.
const repoSourceLink = "https://forgejo.coilysiren.me/coilyco-gaming/sirens-echo/src/branch/main/"

// The doc hands members a source link, so the address has to name this repository.
// A move of the repository breaks this link and the test with it.
func TestCapabilityDocLinksThisRepository(t *testing.T) {
	t.Parallel()
	for name, doc := range capabilityDocs(t) {
		if !strings.Contains(doc, repoSourceLink) {
			t.Errorf("%s does not give the link form %q", name, repoSourceLink)
		}
	}
}

// The doc tells the model it cannot know which revision produced a reply. That
// is only true while the build stamps nothing, so the build is the assertion.
func TestCapabilityDocIsRightThatTheBuildCarriesNoRevision(t *testing.T) {
	t.Parallel()
	claim := "built without its commit"
	claiming := make([]string, 0, 2)
	// Matched against reflowed text, because the claim wraps across lines in the
	// doc and a raw Contains would skip this whole test without saying so.
	for name, doc := range capabilityDocs(t) {
		if strings.Contains(strings.Join(strings.Fields(doc), " "), claim) {
			claiming = append(claiming, name)
		}
	}
	if len(claiming) == 0 {
		t.Skip("no capability.md claims the build carries no revision")
	}

	raw, err := os.ReadFile(filepath.Join("..", "..", "Dockerfile"))
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	body := string(raw)
	// A .git directory lets the toolchain stamp vcs.revision on its own, and an
	// -X assignment sets a variable outright. Either one makes the claim false.
	for _, stamp := range []string{"COPY .git", "-ldflags", "-X "} {
		if strings.Contains(body, stamp) {
			t.Errorf("the build stage carries %q, so the revision is knowable; %v "+
				"still tell the model the build carries no commit", stamp, claiming)
		}
	}
}

// yamlScalar reads one top-level scalar without a YAML dependency, which the
// test does not otherwise need.
func yamlScalar(body, key string) string {
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, key+":") {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(line, key+":"))
	}
	return ""
}
