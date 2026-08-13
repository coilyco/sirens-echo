package community

import (
	"strings"
	"testing"
)

func TestParseReplyAcceptsPlainProse(t *testing.T) {
	t.Parallel()
	reply, err := ParseReply("  Welcome to the community.  ")
	if err != nil {
		t.Fatalf("ParseReply: %v", err)
	}
	if reply != "Welcome to the community." {
		t.Fatalf("reply = %q", reply)
	}
}

// A fenced block is legitimate reply content now that the envelope is gone.
// The old parser unwrapped fences and JSON, which would corrupt this answer.
func TestParseReplyPreservesFencedAndBracedContent(t *testing.T) {
	t.Parallel()
	raw := "Run this:\n```sh\necho {\"reply\":\"x\"}\n```"
	reply, err := ParseReply(raw)
	if err != nil {
		t.Fatalf("ParseReply: %v", err)
	}
	if reply != raw {
		t.Fatalf("reply = %q, want the input verbatim", reply)
	}
}

func TestParseReplyRejectsEmptyAndOverlongReplies(t *testing.T) {
	t.Parallel()
	if _, err := ParseReply("   "); err == nil {
		t.Fatal("expected empty reply error")
	}
	if _, err := ParseReply(strings.Repeat("a", 1801)); err == nil {
		t.Fatal("expected overlong reply error")
	}
}

func TestValidateGroundingRejectsInventedChannel(t *testing.T) {
	t.Parallel()
	reply := "Please check #events."
	err := ValidateGrounding(reply, "The current channel is #bots.")
	if err == nil || !strings.Contains(err.Error(), "invented channel") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateGroundingRejectsClaimedAction(t *testing.T) {
	t.Parallel()
	reply := "I opened an issue for this."
	err := ValidateGrounding(reply, "The current channel is #bots.")
	if err == nil || !strings.Contains(err.Error(), "claimed an action") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateGroundingAllowsClaimConfirmedByTool(t *testing.T) {
	t.Parallel()
	reply := "I closed issue 42."
	err := ValidateGrounding(
		reply,
		"The current channel is #bots.",
		ExecutedTool{Name: "forgejo__close_issue"},
	)
	if err != nil {
		t.Fatalf("ValidateGrounding: %v", err)
	}
}

func TestValidateGroundingRejectsUnsupportedClaimAfterConfirmedTool(t *testing.T) {
	t.Parallel()
	reply := "I closed issue 42, then I deleted its comments."
	err := ValidateGrounding(
		reply,
		"The current channel is #bots.",
		ExecutedTool{Name: "forgejo__close_issue"},
	)
	if err == nil || !strings.Contains(err.Error(), "claimed an action") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateGroundingAllowsSuppliedChannel(t *testing.T) {
	t.Parallel()
	reply := "This is the #bots sandbox."
	if err := ValidateGrounding(reply, "The current channel is #bots."); err != nil {
		t.Fatalf("ValidateGrounding: %v", err)
	}
}

func TestValidateGroundingAllowsForgejoIssueReference(t *testing.T) {
	t.Parallel()
	reply := "The latest open Forgejo issue is #57."
	if err := ValidateGrounding(reply, "The current channel is #bots."); err != nil {
		t.Fatalf("ValidateGrounding: %v", err)
	}
}

func TestValidateNeutralStyleRejectsPersonalityTraits(t *testing.T) {
	t.Parallel()
	for _, reply := range []string{
		"Hey there! 🫡",
		"I'm Sirens Echo, your automated community host.",
		"So here's the thing: the requested tool is unavailable.",
		"The request cannot be completed. Would you like a draft instead?",
		"The Eco server is online!",
		"The Eco server is online 🟢.",
		"Thanks for flagging that.",
	} {
		reply := reply
		t.Run(reply, func(t *testing.T) {
			t.Parallel()
			if err := ValidateNeutralStyle(reply); err == nil {
				t.Fatal("expected personality trait rejection")
			}
		})
	}
}

func TestValidateNeutralStyleAcceptsDirectResults(t *testing.T) {
	t.Parallel()
	for _, reply := range []string{
		"The Eco server is online.",
		"The earlier answer is unverified pending source review.",
		"No approved event time is available.",
	} {
		if err := ValidateNeutralStyle(reply); err != nil {
			t.Errorf("ValidateNeutralStyle(%q): %v", reply, err)
		}
	}
}

// One address per host family in the approved registry. Masking is covered
// below, so this asserts the shapes Echo may actually publish.
func TestValidateNeutralStyleAcceptsRegistryHosts(t *testing.T) {
	t.Parallel()
	for _, reply := range []string{
		"Room tier follows the room's materials. https://wiki.play.eco/en/index.php?stable=1&title=Housing",
		"Open trades are listed at https://eco-app.coilysiren.me/trade",
		"Buy and sell prices are at https://eco-gnome.coilysiren.me/",
		"Operator writing is published at https://www.coilysiren.me/",
		"The simulation runs at https://galaxy-gen.coilysiren.me/",
		"The dependency graph is at https://atlas.coilysiren.me/",
		"Algebra is a branch of mathematics. https://en.wikipedia.org/wiki/Algebra",
	} {
		if err := ValidateNeutralStyle(reply); err != nil {
			t.Errorf("ValidateNeutralStyle(%q): %v", reply, err)
		}
	}
}

func TestValidateResponseStyleAllowsSocialVoice(t *testing.T) {
	t.Parallel()
	reply := "Hey! I found the Eco server status for you."
	if err := ValidateResponseStyle(ResponseStyleSocial, reply); err != nil {
		t.Fatalf("ValidateResponseStyle: %v", err)
	}
	if err := ValidateResponseStyle(ResponseStyleNeutral, reply); err == nil {
		t.Fatal("neutral response style accepted social voice")
	}
}

// A link is not prose: `coilysiren.me` ends in the pronoun "me" and a Forgejo
// fragment looks like a channel.
func TestValidateNeutralStyleAllowsLinks(t *testing.T) {
	t.Parallel()
	for _, reply := range []string{
		"The tracked report is https://forgejo.coilysiren.me/coilyco-gaming/sirens-echo/issues/233",
		"Trade history is at https://eco.coilysiren.me/trades",
		"The reference is https://forgejo.coilysiren.me/coilyco-gaming/sirens-echo/issues/233#issue-8117",
	} {
		reply := reply
		t.Run(reply, func(t *testing.T) {
			t.Parallel()
			if err := ValidateNeutralStyle(reply); err != nil {
				t.Fatalf("ValidateNeutralStyle rejected a link: %v", err)
			}
		})
	}
}

func TestValidateGroundingAllowsURLFragment(t *testing.T) {
	t.Parallel()
	reply := "The report is https://forgejo.coilysiren.me/coilyco-gaming/sirens-echo/issues/233#issue-8117"
	if err := ValidateGrounding(reply, "The current channel is #bots."); err != nil {
		t.Fatalf("ValidateGrounding rejected a URL fragment: %v", err)
	}
}

// Masking must not weaken the check for prose outside the link.
func TestValidateNeutralStyleStillRejectsProseAroundLinks(t *testing.T) {
	t.Parallel()
	reply := "Here is my tracked report: https://forgejo.coilysiren.me/coilyco-gaming/sirens-echo/issues/233"
	if err := ValidateNeutralStyle(reply); err == nil {
		t.Fatal("ValidateNeutralStyle accepted first-person prose beside a link")
	}
}

func TestValidateGroundingStillRejectsInventedChannelBesideLink(t *testing.T) {
	t.Parallel()
	reply := "See #announcements and https://forgejo.coilysiren.me/coilyco-gaming/sirens-echo/issues/233"
	if err := ValidateGrounding(reply, "The current channel is #bots."); err == nil {
		t.Fatal("ValidateGrounding accepted an invented channel beside a link")
	}
}

// A turn that reached the tracker may report what it found, including a status
// that reads as passive voice.
func TestValidateGroundingAllowsGroundedPassiveClaim(t *testing.T) {
	t.Parallel()
	read := ExecutedTool{Name: "sirens-echo-forgejo__get_issue"}
	filed := ExecutedTool{Name: "sirens-echo-forgejo__create_issue"}
	for name, call := range map[string]ExecutedTool{"read": read, "filed": filed} {
		call := call
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			reply := "A correction has been filed for review."
			if err := ValidateGrounding(reply, "The current channel is #bots.", call); err != nil {
				t.Fatalf("ValidateGrounding rejected a grounded claim: %v", err)
			}
		})
	}
}

// Passive voice about the game world is not a tracker claim, and rejecting it
// would fail a correct reply.
func TestValidateGroundingAllowsPassiveWorldProse(t *testing.T) {
	t.Parallel()
	for _, reply := range []string{
		"The Eco server was updated at the start of the cycle.",
		"The trade was created by a player in the settlement.",
		"Elk populations are tracked by the ecosystem simulation.",
		"The road was closed during the last world event.",
	} {
		reply := reply
		t.Run(reply, func(t *testing.T) {
			t.Parallel()
			if err := ValidateGrounding(reply, "The current channel is #bots."); err != nil {
				t.Fatalf("ValidateGrounding rejected correct world prose: %v", err)
			}
		})
	}
}

// A link carries the word "issues" in its path and must not seed the claim.
func TestValidateGroundingIgnoresIssueWordInsideLink(t *testing.T) {
	t.Parallel()
	reply := "The prior report is at https://forgejo.coilysiren.me/coilyco-gaming/sirens-echo/issues/233"
	if err := ValidateGrounding(reply, "The current channel is #bots."); err != nil {
		t.Fatalf("ValidateGrounding rejected a bare link: %v", err)
	}
}

// Known limitation, tracked in issue 253. Every validator pattern is a list of
// English words, so a translated reply matches none of them and ships.
func TestValidatorsAreEnglishOnly(t *testing.T) {
	t.Parallel()
	// Each pair is one English reply the validators reject and its direct
	// translation, which they do not. Failing here means a fix landed.
	for name, reply := range map[string]string{
		"first person, french":   "J'ai vérifié le serveur pour vous.",
		"first person, spanish":  "He comprobado el servidor para usted.",
		"social opening, french": "Bonjour, le serveur est en ligne.",
		"personality, french":    "Ravi de vous aider, dites-moi.",
	} {
		name, reply := name, reply
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateNeutralStyle(reply); err != nil {
				t.Fatalf("the coverage gap closed for %s, so issue 253 and this test need revisiting: %v", name, err)
			}
		})
	}
	// The grounding half is the consequential one. This sentence claims a
	// completed write no tool performed, and nothing rejects it.
	claim := "J'ai déposé une correction pour examen."
	if err := ValidateGrounding(claim, "The current channel is #bots."); err != nil {
		t.Fatalf("grounding reached a translated action claim, so issue 253 needs revisiting: %v", err)
	}
}

// A code span is how a technical answer stays readable, and the grave accent is
// ASCII Sk, so scanning that category banned it outright.
func TestValidateNeutralStyleAllowsCodeSpansAndExponents(t *testing.T) {
	t.Parallel()
	for _, reply := range []string{
		"Run `ward exec test` to check.",
		"The item key is `WoodenHullPlanksItem`.",
		"The exponent is 2^8.",
		"Set `max_context_messages` to 12.",
	} {
		reply := reply
		t.Run(reply, func(t *testing.T) {
			t.Parallel()
			if err := ValidateNeutralStyle(reply); err != nil {
				t.Fatalf("ValidateNeutralStyle rejected ordinary punctuation: %v", err)
			}
		})
	}
}

// The emoji ban itself is unchanged.
func TestValidateNeutralStyleStillRejectsEmoji(t *testing.T) {
	t.Parallel()
	for _, reply := range []string{
		"The Eco server is online 🟢.",
		"Wood 🪵 is listed at 3 Spectres.",
		"Understood 🙂.",
	} {
		reply := reply
		t.Run(reply, func(t *testing.T) {
			t.Parallel()
			if err := ValidateNeutralStyle(reply); err == nil {
				t.Fatal("ValidateNeutralStyle accepted an emoji")
			}
		})
	}
}

// Characterization of ValidateGrounding alone, tracked in issue 241. One of the
// two below is refused by a later check, so read the pipeline test beside it.
func TestGroundingStillMissesTwoShapes(t *testing.T) {
	t.Parallel()
	missed := map[string]string{
		"simple past passive": "A tracking issue was created.",
		"third-person named":  "Sirens Echo has filed a correction.",
	}
	for name, reply := range missed {
		name, reply := name, reply
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if ValidateGrounding(reply, "The current channel is #bots.") != nil {
				t.Fatalf("grounding now reads %s, so drop it from missed and assert it is caught", name)
			}
		})
	}
	// The five already caught must stay caught, or a later widening traded one
	// shape for another.
	for _, reply := range []string{
		"A correction has been filed for review.",
		"An issue has been opened for this.",
		"Filed a correction for review.",
		"I filed a correction for review.",
		"The system is now processing these requests sequentially as instructed.",
	} {
		if ValidateGrounding(reply, "The current channel is #bots.") == nil {
			t.Errorf("grounding stopped catching %q", reply)
		}
	}
}

// The turn runs five checks, so a shape grounding misses is not a shape a
// member sees. Measuring one and reporting an escape is the issue 210 error.

// These two shapes only. QA measured twelve survivors in issue 241, so the
// old name claimed more than this tests.
func TestBothCharacterizedShapesBehaveThroughTheWholeReplyPath(t *testing.T) {
	t.Parallel()
	identity := "Sirens Echo"
	survives := map[string]bool{
		// Simple past with no agent is how a member's own action reads, so this
		// one is deliberately unread. See docs/sirens-echo-grounding.md.
		"A tracking issue was created.": true,
		// Caught by the check built for a named self-claim, not by grounding.
		"Sirens Echo has filed a correction.": false,
	}
	for reply, expected := range survives {
		reply, expected := reply, expected
		t.Run(reply, func(t *testing.T) {
			t.Parallel()
			refused := ValidateGrounding(reply, "The current channel is #bots.") != nil ||
				ValidateSelfAttributedClaim(reply, identity) != nil
			if refused == expected {
				t.Fatalf("reply survives=%v against expected %v; the pipeline "+
					"changed, so update issue 241 rather than this assertion", !refused, expected)
			}
		})
	}
}
