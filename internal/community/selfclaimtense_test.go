package community

import "testing"

// The check built for a named self-claim required "has". Simple past is at
// least as natural, and walked straight through it. See sirens-echo#241.

func TestTheNamedSelfClaimIsCaughtInEitherTense(t *testing.T) {
	t.Parallel()
	identity := "Sirens Echo"
	for name, reply := range map[string]string{
		"perfect":            "Sirens Echo has filed a correction.",
		"simple past":        "Sirens Echo filed a correction.",
		"simple past opened": "Sirens Echo opened an issue for this.",
		"simple past made":   "Sirens Echo created a tracking issue.",
		"plural auxiliary":   "Sirens Echo have filed a correction.",
		"lowercased":         "sirens echo filed a correction.",
	} {
		if ValidateSelfAttributedClaim(reply, identity) == nil {
			t.Errorf("%s escaped the self-claim check: %q", name, reply)
		}
	}
}

// A claim the runtime actually performed is not a false claim, and dropping the
// auxiliary must not change that.
func TestATrackerWriteStillPermitsTheClaim(t *testing.T) {
	t.Parallel()
	executed := ExecutedTool{Name: "forgejo__create_issue", Outcome: ToolOutcomeOK}
	for _, reply := range []string{
		"Sirens Echo filed a correction.",
		"Sirens Echo has filed a correction.",
	} {
		if err := ValidateSelfAttributedClaim(reply, "Sirens Echo", executed); err != nil {
			t.Errorf("a performed write was refused: %q: %v", reply, err)
		}
	}
}

// The widening must not start reading ordinary prose as a self-claim. Each of
// these names the service without claiming it wrote to the tracker.
func TestOrdinaryProseNamingTheServiceIsNotAClaim(t *testing.T) {
	t.Parallel()
	identity := "Sirens Echo"
	for name, reply := range map[string]string{
		"a capability statement": "Sirens Echo can file an issue if you ask.",
		"a refusal":              "Sirens Echo will not file that.",
		"a name in isolation":    "Sirens Echo is the service in this channel.",
		"a comma breaks it":      "Sirens Echo, filed under the wrong label, is not the topic.",
		"someone else acted":     "A member filed a correction about Sirens Echo.",
	} {
		if err := ValidateSelfAttributedClaim(reply, identity); err != nil {
			t.Errorf("%s was read as a self-claim: %q", name, reply)
		}
	}
}

// No configured identity means no name to tell the service from a member, and
// a member filing something is a correct reply.
func TestWithoutAnIdentityNothingIsASelfClaim(t *testing.T) {
	t.Parallel()
	if err := ValidateSelfAttributedClaim("Sirens Echo filed a correction.", ""); err != nil {
		t.Errorf("an unconfigured identity refused a reply: %v", err)
	}
}
