package community

import (
	"strings"
	"testing"
)

// /poll is member-invoked and free-form. See docs/sirens-echo-commands.md.

func pollArguments(overrides map[string]string) map[string]string {
	arguments := map[string]string{
		"question": "Which Enshrouded boss this weekend?",
		"answer-1": "The Cinder Queen",
		"answer-2": "The Overgrown Guardian",
	}
	for key, value := range overrides {
		arguments[key] = value
	}
	return arguments
}

func TestPollBuildsFromTheDeclaredAnswers(t *testing.T) {
	t.Parallel()
	poll, err := pollFromArguments(pollArguments(map[string]string{"answer-3": "Neither, raid the mines"}))
	if err != nil {
		t.Fatalf("pollFromArguments: %v", err)
	}
	if poll.Question.Text != "Which Enshrouded boss this weekend?" {
		t.Fatalf("question = %q", poll.Question.Text)
	}
	if len(poll.Answers) != 3 {
		t.Fatalf("answers = %d, want 3", len(poll.Answers))
	}
	if poll.Answers[2].Media.Text != "Neither, raid the mines" {
		t.Fatalf("third answer = %q", poll.Answers[2].Media.Text)
	}
	// Discord assigns answer_id; sending one would misdeclare a poll neither
	// this deployment nor the member composed.
	if poll.Answers[0].AnswerID != 0 {
		t.Fatalf("answer_id was set on creation: %d", poll.Answers[0].AnswerID)
	}
}

func TestPollDefaultsToOneDayAndSingleSelect(t *testing.T) {
	t.Parallel()
	poll, err := pollFromArguments(pollArguments(nil))
	if err != nil {
		t.Fatalf("pollFromArguments: %v", err)
	}
	if poll.Duration != pollDefaultDuration {
		t.Fatalf("duration = %d, want the default %d", poll.Duration, pollDefaultDuration)
	}
	if poll.AllowMultiselect {
		t.Fatal("multiselect defaulted to true")
	}
}

func TestPollHonoursDurationAndMultiselect(t *testing.T) {
	t.Parallel()
	poll, err := pollFromArguments(pollArguments(map[string]string{
		"duration-hours": "168",
		"multiselect":    "true",
	}))
	if err != nil {
		t.Fatalf("pollFromArguments: %v", err)
	}
	if poll.Duration != 168 {
		t.Fatalf("duration = %d, want 168", poll.Duration)
	}
	if !poll.AllowMultiselect {
		t.Fatal("multiselect did not take effect")
	}
}

// This deployment says why before Discord's own API does, in a notice.
func TestPollNeedsAtLeastTwoAnswers(t *testing.T) {
	t.Parallel()
	reply := runPollCommand(map[string]string{
		"question": "Which boss?",
		"answer-1": "Only one",
	})
	if reply.Poll != nil {
		t.Fatal("a poll was built with one answer")
	}
	if !strings.Contains(reply.Notice, "at least") {
		t.Fatalf("notice = %q, want it to say why", reply.Notice)
	}
}

func TestPollDurationOutsideDiscordsRangeIsRefused(t *testing.T) {
	t.Parallel()
	for _, invalid := range []string{"0", "769", "not-a-number"} {
		reply := runPollCommand(pollArguments(map[string]string{"duration-hours": invalid}))
		if reply.Poll != nil {
			t.Fatalf("duration-hours=%s built a poll", invalid)
		}
		if reply.Notice == "" {
			t.Fatalf("duration-hours=%s produced no notice", invalid)
		}
	}
}

// A valid poll is the whole answer. A notice beside it would read as the
// command half-failing when it did not.
func TestAValidPollCarriesNoNotice(t *testing.T) {
	t.Parallel()
	reply := runPollCommand(pollArguments(nil))
	if reply.Poll == nil {
		t.Fatal("a valid poll was refused")
	}
	if reply.Notice != "" {
		t.Fatalf("notice = %q beside a valid poll", reply.Notice)
	}
}

// The declared schema is what Discord's picker renders and what BindArguments
// enforces, so the two cannot drift. See docs/sirens-echo-commands.md.
func TestPollCommandIsDeclaredWithTwoRequiredAnswers(t *testing.T) {
	t.Parallel()
	command, declared := LookupCommand("poll", nil)
	if !declared {
		t.Fatal("poll is not declared")
	}
	if command.Kind != "" {
		t.Fatalf("poll submits job kind %q", command.Kind)
	}
	if command.Ephemeral {
		t.Fatal("poll answered ephemerally, so nobody else could vote")
	}
	if err := command.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	required := 0
	for _, parameter := range command.Parameters {
		if parameter.Required {
			required++
		}
	}
	// question, answer-1, answer-2.
	if required != 3 {
		t.Fatalf("required parameters = %d, want 3 (question, answer-1, answer-2)", required)
	}
	if _, err := command.BindArguments(pollArguments(nil)); err != nil {
		t.Fatalf("a minimal poll was refused by its own schema: %v", err)
	}
	if _, err := command.BindArguments(map[string]string{"question": "Which boss?"}); err == nil {
		t.Fatal("a poll with no answers was accepted by its own schema")
	}
}

// poll submits no job, so it must answer above the jobs-disabled guard.
func TestRunCommandRoutesPollAboveTheJobsGuard(t *testing.T) {
	t.Parallel()
	agent := &Agent{telemetry: telemetryOrNoop(nil)}
	command, declared := LookupCommand("poll", nil)
	if !declared {
		t.Fatal("poll is not declared")
	}
	reply := agent.runCommand(t.Context(), commandRequest{
		Command:   command,
		Arguments: pollArguments(nil),
	})
	if reply.Poll == nil {
		t.Fatal("poll was not built with jobs disabled, but poll submits no job")
	}
}
