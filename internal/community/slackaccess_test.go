package community

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validSlackPolicy = `schema: coilyco-harness.slack-access.v1
deny:
  users: ["U0DENIED1"]
direct_messages:
  allow: ["U0KAI0001"]
workspaces:
  - id: T0COILYCO
    note: coilyco workspace
    channels: ["C0GENERAL", "G0PRIVATE"]
    users: ["U0KAI0001", "U0OTHER01"]
`

func writeSlackPolicy(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "slack-access.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	return path
}

func TestASlackPolicyThatGrantsNothingFailsTheLoad(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"wrong schema":      strings.Replace(validSlackPolicy, "slack-access.v1", "access.v1", 1),
		"no workspace":      "schema: coilyco-harness.slack-access.v1\n",
		"unknown field":     validSlackPolicy + "extra: 1\n",
		"bad workspace id":  strings.Replace(validSlackPolicy, "T0COILYCO", "coilyco", 1),
		"bad channel id":    strings.Replace(validSlackPolicy, "C0GENERAL", "general", 1),
		"bad user id":       strings.Replace(validSlackPolicy, "U0OTHER01", "kai", 1),
		"no channel":        strings.Replace(validSlackPolicy, `channels: ["C0GENERAL", "G0PRIVATE"]`, "channels: []", 1),
		"no member":         strings.Replace(validSlackPolicy, `users: ["U0KAI0001", "U0OTHER01"]`, "users: []", 1),
		"every member open": strings.Replace(validSlackPolicy, `users: ["U0KAI0001", "U0OTHER01"]`, "users: all", 1),
	}
	for name, body := range cases {
		if _, err := LoadSlackAccessPolicy(writeSlackPolicy(t, body)); err == nil {
			t.Errorf("%s: policy loaded, want a startup failure", name)
		}
	}
}

func TestAnOpenWorkspaceNeedsARealPerUserBound(t *testing.T) {
	t.Parallel()
	open := strings.Replace(validSlackPolicy, `users: ["U0KAI0001", "U0OTHER01"]`,
		"users: all\n    rate_limit: {per_user: \"1/60s\"}", 1)
	if _, err := LoadSlackAccessPolicy(writeSlackPolicy(t, open)); err != nil {
		t.Fatalf("a bounded open workspace must load: %v", err)
	}
}

func TestSlackAccessIsDecidedFromThePayloadInCostOrder(t *testing.T) {
	t.Parallel()
	policy, err := LoadSlackAccessPolicy(writeSlackPolicy(t, validSlackPolicy))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cases := []struct {
		name                string
		team, channel, user string
		direct              bool
		want                accessReason
	}{
		{"allowed in a listed channel", "T0COILYCO", "C0GENERAL", "U0KAI0001", false, accessAllowed},
		{"private channel", "T0COILYCO", "G0PRIVATE", "U0OTHER01", false, accessAllowed},
		{"deny beats every allow", "T0COILYCO", "C0GENERAL", "U0DENIED1", false, accessDeniedBlocked},
		{"foreign workspace", "T0FOREIGN", "C0GENERAL", "U0KAI0001", false, accessDeniedGuild},
		{"unlisted channel", "T0COILYCO", "C0ELSEWHR", "U0KAI0001", false, accessDeniedChannel},
		{"unlisted member", "T0COILYCO", "C0GENERAL", "U0STRANGE", false, accessDeniedMember},
		{"allowed direct message", "T0COILYCO", "D0DIRECT01", "U0KAI0001", true, accessAllowed},
		{"direct message from a member not on the list", "T0COILYCO", "D0DIRECT01", "U0OTHER01", true, accessDeniedDM},
		{"direct message from a foreign workspace", "T0FOREIGN", "D0DIRECT01", "U0KAI0001", true, accessDeniedGuild},
	}
	for _, tc := range cases {
		got, _ := policy.Evaluate(tc.team, tc.channel, tc.user, tc.direct)
		if got != tc.want {
			t.Errorf("%s: reason = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestASlackDeploymentMissingTokensOrPolicyRefusesToStart(t *testing.T) {
	t.Parallel()
	if got := missingSlackConfig(Config{}); len(got) != 3 {
		t.Fatalf("missing = %v, want the two tokens and the policy", got)
	}
	whole := Config{
		SlackBotToken: "xoxb-1", SlackAppToken: "xapp-1", SlackAccessPolicyPath: "/policy.yaml",
	}
	if got := missingSlackConfig(whole); len(got) != 0 {
		t.Fatalf("missing = %v, want none", got)
	}
	wrongShape := whole
	wrongShape.SlackBotToken = "xapp-swapped"
	if got := missingSlackConfig(wrongShape); len(got) != 1 {
		t.Fatalf("a token of the wrong kind must be named, got %v", got)
	}
}

func TestTheReferenceSlackPolicyLoadsThroughTheRuntimeLoader(t *testing.T) {
	t.Parallel()
	if _, err := LoadSlackAccessPolicy("../../docs/slack-access-policy.reference.yaml"); err != nil {
		t.Fatalf("the reference copy no longer loads: %v", err)
	}
}
