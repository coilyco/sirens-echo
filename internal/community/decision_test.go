package community

import (
	"strings"
	"testing"
)

func TestParseDecisionAcceptsOrdinaryReply(t *testing.T) {
	t.Parallel()
	decision, err := ParseDecision(`{"reply":"Welcome to #bots!","issue":null}`)
	if err != nil {
		t.Fatalf("ParseDecision: %v", err)
	}
	if decision.Reply != "Welcome to #bots!" || decision.Issue != nil {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestParseDecisionAcceptsPlainTextReply(t *testing.T) {
	t.Parallel()
	decision, err := ParseDecision("Welcome to the community.")
	if err != nil {
		t.Fatalf("ParseDecision: %v", err)
	}
	if decision.Reply != "Welcome to the community." || decision.Issue != nil {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestParseDecisionAcceptsEmbeddedJSON(t *testing.T) {
	t.Parallel()
	decision, err := ParseDecision(
		`Here is the result: {"reply":"Use {curly braces} safely.","issue":null}`,
	)
	if err != nil {
		t.Fatalf("ParseDecision: %v", err)
	}
	if decision.Reply != "Use {curly braces} safely." || decision.Issue != nil {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestParseDecisionRejectsMalformedProtocol(t *testing.T) {
	t.Parallel()
	if _, err := ParseDecision(`Result: {"reply":"broken","issue":`); err == nil {
		t.Fatal("expected malformed protocol error")
	}
}

func TestParseDecisionSanitizesIssueContext(t *testing.T) {
	t.Parallel()
	raw := `{"reply":"I do not know yet.","issue":{"kind":"knowledge-gap","title":"Event time","body":"Member <@123456789012345678> cited https://discord.com/channels/123456789012345678/223456789012345678/323456789012345678."}}`
	decision, err := ParseDecision(raw)
	if err != nil {
		t.Fatalf("ParseDecision: %v", err)
	}
	if decision.Issue == nil {
		t.Fatal("expected issue draft")
	}
	for _, forbidden := range []string{"123456789012345678", "discord.com/channels"} {
		if strings.Contains(decision.Issue.Body, forbidden) {
			t.Fatalf("issue body retained %q: %s", forbidden, decision.Issue.Body)
		}
	}
}

func TestParseDecisionRejectsUnsupportedIssueKind(t *testing.T) {
	t.Parallel()
	_, err := ParseDecision(`{"reply":"No.","issue":{"kind":"moderation","title":"Ban","body":"Ban a member."}}`)
	if err == nil {
		t.Fatal("expected unsupported issue kind error")
	}
}

func TestValidateGroundingRejectsInventedChannel(t *testing.T) {
	t.Parallel()
	decision := Decision{Reply: "Please check #events."}
	err := ValidateGrounding(decision, "The current channel is #bots.")
	if err == nil || !strings.Contains(err.Error(), "invented channel") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateGroundingRejectsClaimedAction(t *testing.T) {
	t.Parallel()
	decision := Decision{Reply: "I opened an issue for this."}
	err := ValidateGrounding(decision, "The current channel is #bots.")
	if err == nil || !strings.Contains(err.Error(), "claimed an action") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateGroundingAllowsSuppliedChannel(t *testing.T) {
	t.Parallel()
	decision := Decision{Reply: "This is the #bots sandbox."}
	if err := ValidateGrounding(decision, "The current channel is #bots."); err != nil {
		t.Fatalf("ValidateGrounding: %v", err)
	}
}
