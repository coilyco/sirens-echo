package community

import (
	"strings"
	"testing"
)

// Every profile, not only the neutral one: #843 measured the 14-of-15 breach on
// the social lane against its real composed bundle.
func TestBoundaryBrevityShipsOnEveryResponseStyle(t *testing.T) {
	t.Parallel()
	for _, style := range []string{ResponseStyleNeutral, ResponseStyleSocial} {
		t.Run(style, func(t *testing.T) {
			definition := Definition{Identity: "Test Agent", ResponseStyle: style}
			prompt := BuildSystemPrompt(definition, Principal{}, "", "policy")
			if !strings.Contains(prompt, boundaryBrevityPolicy) {
				t.Fatalf("%s profile ships no boundary-brevity rule", style)
			}
			if err := ValidateSystemPrompt(definition, Principal{}, prompt); err != nil {
				t.Fatalf("ValidateSystemPrompt: %v", err)
			}
		})
	}
}

// A rendered prompt missing the rule is refused, so it cannot drop out of a
// profile the way it was never in one.
func TestValidateRefusesAPromptWithoutBoundaryBrevity(t *testing.T) {
	t.Parallel()
	definition := Definition{Identity: "Test Agent", ResponseStyle: ResponseStyleNeutral}
	prompt := BuildSystemPrompt(definition, Principal{}, "", "policy")
	stripped := strings.Replace(prompt, boundaryBrevityPolicy, "", 1)
	if err := ValidateSystemPrompt(definition, Principal{}, stripped); err == nil {
		t.Fatal("a prompt without the boundary-brevity rule must be refused")
	}
}

// The forbidden clauses and the outside case, which is what keeps this from
// becoming a licence to truncate an ordinary answer.
func TestBoundaryBrevityNamesItsForbiddenClausesAndItsScope(t *testing.T) {
	t.Parallel()
	for _, required := range []string{
		"cite which guideline forbids it",
		"characterise what",
		"explain the trust model",
		"identifier, path, or\nconfiguration key",
		"offer a route to satisfying the requirement",
		"bounds declining replies only",
		"An ordinary answer is not shortened",
	} {
		if !strings.Contains(boundaryBrevityPolicy, required) {
			t.Errorf("the rule no longer states %q", required)
		}
	}
}

// The coverage rule ships on every profile and is pinned, so a bounded search
// cannot quietly become an unbounded claim in one lane and not the other.
func TestCoverageRuleShipsAndIsPinned(t *testing.T) {
	t.Parallel()
	for _, style := range []string{ResponseStyleNeutral, ResponseStyleSocial} {
		definition := Definition{Identity: "Test Agent", ResponseStyle: style}
		prompt := BuildSystemPrompt(definition, Principal{}, "", "policy")
		if !strings.Contains(prompt, coveragePolicy) {
			t.Errorf("%s profile ships no coverage rule", style)
		}
		stripped := strings.Replace(prompt, coveragePolicy, "", 1)
		if err := ValidateSystemPrompt(definition, Principal{}, stripped); err == nil {
			t.Errorf("%s: a prompt without the coverage rule must be refused", style)
		}
	}
}

// Both halves: the claim it forbids, and the scope that keeps it from turning
// every complete empty answer into a hedge.
func TestCoverageRuleNamesTheClaimAndItsScope(t *testing.T) {
	t.Parallel()
	for _, required := range []string{
		`"none in what I searched" and never "none exists"`,
		"An empty result is the case this is about",
		"bounds claims about absence",
		"genuine, complete, empty answer is still given as one",
	} {
		if !strings.Contains(coveragePolicy, required) {
			t.Errorf("the rule no longer states %q", required)
		}
	}
}
