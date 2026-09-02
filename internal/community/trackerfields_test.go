package community

import (
	"reflect"
	"testing"
)

// The model never supplies these values and cannot omit them. See #208, #756.

func sandboxPolicy() trackerPolicy {
	return trackerPolicy{
		Tracker:       "teable",
		IssuesTable:   "tblIssues",
		CommentsTable: "tblComments",
		Org:           "coilyco-gaming",
		Repo:          "sirens-echo",
		Priority:      "P3",
		Roles:         []string{"platform"},
	}
}

// A filing writes two rows, so a policy missing either table files nothing
// rather than filing a title with no body anyone can read.
func TestAPolicyMissingEitherTableIsInactive(t *testing.T) {
	t.Parallel()
	for name, policy := range map[string]trackerPolicy{
		"no tracker":  {IssuesTable: "tblIssues", CommentsTable: "tblComments"},
		"no issues":   {Tracker: "teable", CommentsTable: "tblComments"},
		"no comments": {Tracker: "teable", IssuesTable: "tblIssues"},
		"blank issues": {
			Tracker: "teable", IssuesTable: "   ", CommentsTable: "tblComments",
		},
		"nothing": {},
	} {
		if policy.active() {
			t.Errorf("%s reported itself active", name)
		}
	}
	if !sandboxPolicy().active() {
		t.Error("a fully configured policy reported itself inactive")
	}
}

// The control this file exists for. Every row the service files says its
// contents are unverified, and no configuration turns that off.
func TestEveryFiledIssueIsMarkedUnverified(t *testing.T) {
	t.Parallel()
	for name, policy := range map[string]trackerPolicy{
		"fully configured": sandboxPolicy(),
		"bare":             {Tracker: "teable", IssuesTable: "i", CommentsTable: "c"},
	} {
		if got := policy.issueFields("a report")["autonomy"]; got != sandboxAutonomy {
			t.Errorf("%s autonomy = %v, want the sandbox marker", name, got)
		}
	}
}

// A filed issue starts open. The state is the harness's, so a model cannot
// file something already closed and have it read as triaged.
func TestAFiledIssueStartsOpen(t *testing.T) {
	t.Parallel()
	if got := sandboxPolicy().issueFields("a report")["state"]; got != trackerStateOpen {
		t.Errorf("state = %v, want %q", got, trackerStateOpen)
	}
}

// The title is the only value that crosses from the model.
func TestTheHarnessOwnsEveryFieldButTheTitle(t *testing.T) {
	t.Parallel()
	got := sandboxPolicy().issueFields("a report")
	want := map[string]any{
		"title":    "a report",
		"state":    trackerStateOpen,
		"autonomy": sandboxAutonomy,
		"org":      "coilyco-gaming",
		"repo":     "sirens-echo",
		"priority": "P3",
		"roles":    []string{"platform"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fields = %v, want %v", got, want)
	}
}

// Each destination value is independently optional, so an unset knob leaves
// the tracker's own default rather than writing an empty select.
func TestAnUnsetDestinationWritesNothingForIt(t *testing.T) {
	t.Parallel()
	policy := trackerPolicy{Tracker: "teable", IssuesTable: "i", CommentsTable: "c"}
	got := policy.issueFields("a report")
	for _, key := range []string{"org", "repo", "priority", "roles"} {
		if _, present := got[key]; present {
			t.Errorf("unset %s was written as %v", key, got[key])
		}
	}
}

// A trailing comma configures nothing rather than an empty choice, which the
// tracker would accept and nobody could filter on.
func TestABlankListValueConfiguresNothing(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"", "   ", ",", " , ,"} {
		if got := commaValues(raw); len(got) != 0 {
			t.Errorf("commaValues(%q) = %v, want none", raw, got)
		}
	}
	got := commaValues(" platform , ,sysadmin ")
	if !reflect.DeepEqual(got, []string{"platform", "sysadmin"}) {
		t.Errorf("commaValues = %v", got)
	}
}
