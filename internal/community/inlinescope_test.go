package community

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scopeMarker is the convention: an always-inline reference states what it
// applies to, in its own prose, where the model reads it.
const scopeMarker = "**Applies to**"

// An always-inline reference is in the prompt on every turn, whatever the
// subject, and carries no scope of its own. See sirens-echo#1049.
func TestEveryAlwaysInlineReferenceDeclaresItsScope(t *testing.T) {
	t.Parallel()
	found := 0
	err := filepath.WalkDir("../../.agents/skills", func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !isReferencePath(path) {
			return walkErr
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(raw)
		if !inlineAlways(text) {
			return nil
		}
		found++
		if !strings.Contains(text, scopeMarker) {
			t.Errorf("%s is inline on every turn and states no %s line. An unscoped "+
				"imperative is read on every subject, which is what #1049 measured.",
				filepath.ToSlash(path), scopeMarker)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk skills: %v", err)
	}
	if found == 0 {
		t.Fatal("no always-inline references found, so this asserts nothing")
	}
}

// A rule that really does bind everywhere says so, rather than reaching every
// subject by omission. That is the distinction the convention exists to force.
func TestAnUnscopedRuleSaysSoExplicitly(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../.agents/skills/sirens-echo-knowledge/references/boundaries.md")
	if err != nil {
		t.Fatalf("read boundaries: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, scopeMarker+" every request") {
		t.Error("the decline list binds every request and must state that, not omit a scope")
	}
}
