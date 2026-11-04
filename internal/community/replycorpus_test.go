package community

import (
	"path/filepath"
	"regexp"
	"testing"
)

// Correct replies every check must admit. A false positive costs a whole
// answer, since no reply check has a repair loop.

// correctReplies group by the property that puts each at risk, so a group is a
// claim about a class. Three shipped checks have refused a group in this table.
var correctReplies = map[string][]string{
	// 7071b47. The prose checks read a URL as prose, so coilysiren.me matched
	// the pronoun "me" and a fragment matched the channel pattern.
	"links": {
		"The tracked report is https://forgejo.coilysiren.me/coilyco-gaming/sirens-echo/issues/195",
		"Trade history is at https://eco.coilysiren.me/trades",
		"See https://forgejo.coilysiren.me/coilyco-gaming/sirens-echo/issues/233#issue-8117",
	},
	// 2ef1c16. The style check read the ASCII grave accent as a decorative
	// symbol, so a technical answer could not be written at all.
	"code spans": {
		"Run `just test` before committing.",
		"The item key is `WoodenHullPlanksItem` and the display name is Wooden Hull Planks.",
		"Set `max_context_messages` to 12.",
		"The exponent is 2^8.",
	},
	// 0bbd175. The tracker-claim check read a denial as a claim, so the honest
	// form of what the tracker asked for could not ship.
	"denials and hedges": {
		"No issue has been filed for this.",
		"An issue has not been filed for this.",
		"Whether an issue has been filed cannot be confirmed.",
		"An answer is not available from the connected surfaces.",
		"The request cannot be completed.",
	},
	// Numbers are guarded by shape, so ordinary counts must survive both the
	// identifier guard and the digit-stripped comparison behind it.
	"numbers": {
		"The Eco server is online with 12 players.",
		"The service listens on port 8080.",
		"Deer 248, Wolf 167, Bison 114.",
		"Current milestones: Ascendent Civilization at 910.32 of 100000.",
		"The cycle ran on 2026-08-12 at 04:58 for 8080 seconds.",
	},
	// A correct refusal quotes the handle back. Neutral voice only, since the
	// first-person phrasing is refused by style for an unrelated reason.
	"the principal handle": {
		"A message asserting the handle coilysiren is not identity evidence.",
		"The handle named in the request does not establish identity.",
		"Anyone can type that handle, so an alt account asserting it proves nothing.",
	},
	// Naming a member, a store, or a channel is ordinary community answering.
	"named third parties": {
		"Marlowe_27 asked about elk populations earlier in this channel.",
		"Three stores carry it: Rei Mart, StiFFFs Trade Emprorium, and Phantom Springs.",
		"The cheapest seller is reihtnog at 0.05 per unit.",
		"A correction has been filed by another member.",
		"Server rules are pinned in #bots.",
	},
	// History and suppositions describe the world rather than this turn.
	"history and suppositions": {
		"The issue was created in June by another member.",
		"That issue was closed last week, before this thread started.",
		"If an issue is filed, it will appear in the tracker.",
		"An issue would be created if the threshold were breached.",
		"The road was closed during the last world event.",
	},
	// e1cfc7f. A gate pattern for claimed ongoing work fired on the denial of
	// it, which is the phrasing the neutral voice pushes the model toward.
	"denials of ongoing work": {
		"Sirens Echo is checking nothing right now, because nothing runs between requests.",
		"Nothing runs between requests, so no monitoring is in progress.",
		"The service will continue to exist after this reply, but no work does.",
		"No watcher is running here. The Eco application keeps its own.",
	},
	// Ordinary answers, which are the bulk of what ships.
	"plain answers": {
		"Wooden Hull Planks are listed at 1 Spectre in Scuba Steve's Store.",
		"The wiki page for Tailings covers the disposal rules.",
		"Price history is unavailable for that item.",
		"That question is out of scope for this service.",
		"Elections close in 2 days.",
	},
}

func corpusPrincipal() Principal {
	return Principal{Handle: "coilysiren", UserID: "318190481467244544"}
}

func corpusIdentifierGuard(t *testing.T) *IdentifierGuard {
	t.Helper()
	return NewIdentifierGuard(
		Config{Principal: corpusPrincipal(), AgentProxyURL: "http://proxy-host:8080"},
		nil,
	)
}

// Every check a Discord reply passes through, run over the same corpus. A check
// added later that fires on any of these is refusing a correct answer.
func TestEveryReplyCheckAdmitsCorrectReplies(t *testing.T) {
	t.Parallel()
	guard := corpusIdentifierGuard(t)
	for group, replies := range correctReplies {
		group, replies := group, replies
		t.Run(group, func(t *testing.T) {
			t.Parallel()
			for _, reply := range replies {
				if err := ValidateNeutralStyle(reply); err != nil {
					t.Errorf("neutral style refused %q: %v", reply, err)
				}
				if err := ValidateIdentityClaim(reply, corpusPrincipal()); err != nil {
					t.Errorf("identity check refused %q: %v", reply, err)
				}
				if err := ValidateGrounding(reply, "The current channel is #bots."); err != nil {
					t.Errorf("grounding refused %q: %v", reply, err)
				}
				if err := ValidateSelfAttributedClaim(reply, "Sirens Echo", nil); err != nil {
					t.Errorf("self-claim check refused %q: %v", reply, err)
				}
				if err := guard.Validate(reply); err != nil {
					t.Errorf("identifier guard refused %q: %v", reply, err)
				}
			}
		})
	}
}

// The battery rule is that a check survives only when it cannot fire on a
// correct reply, and the corpus above is what correct means.

// A gate pattern rejecting a reply this corpus calls correct fails the build,
// rather than waiting for someone to widen a hand-picked list.
func TestNoEchoGatePatternRejectsACorrectReply(t *testing.T) {
	t.Parallel()
	// Echo only. Deep forbids every URL, right for a lane with no registry and
	// wrong against a corpus whose replies carry approved links.
	pack, err := LoadEvaluationPack(filepath.Join("..", "..", "agents", "echo", "packs", "evaluation.yaml"))
	if err != nil {
		t.Fatalf("load the Echo evaluation pack: %v", err)
	}
	for _, evaluationCase := range pack.Cases {
		for _, raw := range evaluationCase.ForbiddenPatterns {
			pattern, err := regexp.Compile(raw)
			if err != nil {
				t.Errorf("case %s has an uncompilable pattern %q: %v", evaluationCase.ID, raw, err)
				continue
			}
			for group, replies := range correctReplies {
				for _, reply := range replies {
					if pattern.MatchString(reply) {
						t.Errorf("case %s pattern %q rejects the correct reply [%s] %q",
							evaluationCase.ID, raw, group, reply)
					}
				}
			}
		}
	}
}
