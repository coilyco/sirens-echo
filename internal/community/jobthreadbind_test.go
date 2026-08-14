package community

import (
	"context"
	"strings"
	"testing"
	"time"
)

// A job command's id-less fallback could never resolve, because nothing wrote
// the binding it reads. See sirens-echo#620.

// commandAgent runs commands against a memory store, which is what a job
// submission needs and all this exercises.
func commandAgent(t *testing.T) *Agent {
	t.Helper()
	runner := &JobRunner{
		Store:     NewMemoryJobStore(nil),
		Executors: map[string]JobExecutor{"echo": stalledExecutor{}},
		Timeout:   5 * time.Second,
	}
	if err := runner.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(runner.Stop)
	agent := &Agent{jobs: runner, telemetry: telemetryOrNoop(nil)}
	agent.ensureRuntimeDefaults()
	return agent
}

// stalledExecutor holds a job running, so the binding is observable before the
// job settles and the test does not race the runner.
type stalledExecutor struct{}

func (stalledExecutor) Execute(ctx context.Context, _ Job, _ func(string)) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

// submitInThread runs the submitting command as if it were typed in threadID.
func submitInThread(t *testing.T, agent *Agent, threadID string) string {
	t.Helper()
	command, declared := LookupCommand("echo")
	if !declared {
		t.Fatal("the echo command is not declared, so this test asserts nothing")
	}
	notice := agent.runCommand(context.Background(), commandRequest{
		Command:       command,
		Arguments:     map[string]string{"text": "hello"},
		Principal:     "318190481467244544",
		Origin:        summonContext{Kind: contextKindGuild, ChannelID: threadID},
		InteractionID: "interaction-1",
		ThreadID:      threadID,
	})
	if !strings.Contains(notice, "submitted") {
		t.Fatalf("the job was not submitted: %s", notice)
	}
	return notice
}

// The headline. A job started in a thread is bound to it, so the id-less
// follow-up the commands doc describes actually resolves.
func TestAJobStartedInAThreadBindsToIt(t *testing.T) {
	t.Parallel()
	agent := commandAgent(t)

	submitInThread(t, agent, "thread-1")

	id, err := ResolveJobReference(agent.jobs.Store, "", "thread-1")
	if err != nil {
		t.Fatalf("the thread resolved to no job: %v", err)
	}
	if id == "" {
		t.Error("the thread resolved to an empty job id")
	}
}

// A command run in an ordinary channel binds nothing. Binding a channel would
// make it resolve to one arbitrary job of however many were started there.
func TestAJobStartedInAChannelBindsNothing(t *testing.T) {
	t.Parallel()
	agent := commandAgent(t)

	// ThreadID empty is what threadOrigin returns for a channel.
	notice := agent.runCommand(context.Background(), commandRequest{
		Command:       mustCommand(t, "echo"),
		Arguments:     map[string]string{"text": "hello"},
		Principal:     "318190481467244544",
		Origin:        summonContext{Kind: contextKindGuild, ChannelID: "channel-1"},
		InteractionID: "interaction-1",
	})
	if !strings.Contains(notice, "submitted") {
		t.Fatalf("the job was not submitted: %s", notice)
	}

	if _, err := ResolveJobReference(agent.jobs.Store, "", "channel-1"); err == nil {
		t.Error("a job started in a channel bound the channel")
	}
}

// The singularity the commands doc states. A second job in the same thread does
// not steal the binding, and it does not fail to submit either.
func TestASecondJobInAThreadNeitherBindsNorFails(t *testing.T) {
	t.Parallel()
	agent := commandAgent(t)
	submitInThread(t, agent, "thread-1")
	first, err := ResolveJobReference(agent.jobs.Store, "", "thread-1")
	if err != nil {
		t.Fatalf("the first job did not bind: %v", err)
	}

	submitInThread(t, agent, "thread-1")

	second, err := ResolveJobReference(agent.jobs.Store, "", "thread-1")
	if err != nil {
		t.Fatalf("the binding was lost: %v", err)
	}
	if second != first {
		t.Errorf("the thread now resolves to %s, want the first job %s", second, first)
	}
}

// racingStore starts the job the moment a reader looks, and hands the reader
// the record it saw first. That is the runner's timing, made certain.
type racingStore struct{ *MemoryJobStore }

func (s racingStore) Get(id string) (Job, error) {
	job, err := s.MemoryJobStore.Get(id)
	if err != nil {
		return Job{}, err
	}
	if job.State == JobQueued {
		if _, err := s.MemoryJobStore.Transition(id, JobRunning, nil); err != nil {
			return Job{}, err
		}
	}
	return job, nil
}

// Binding named the state it had just read, and queued is not reachable from
// running, so a job the runner started first lost its thread. See issue 620.
func TestBindingSurvivesTheRunnerStartingTheJob(t *testing.T) {
	t.Parallel()
	store := racingStore{NewMemoryJobStore(nil)}
	job, _, err := store.Submit(Job{
		ID:             "job-bind-race",
		Kind:           "test",
		Principal:      "318190481467244544",
		IdempotencyKey: "job-bind-race-key",
		State:          JobQueued,
		Origin:         JobOrigin{Transport: transportDiscord, ChannelID: "thread-1"},
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	if _, err := BindJobToThread(store, job.ID, "thread-1"); err != nil {
		t.Fatalf("the binding lost to the runner: %v", err)
	}

	bound, err := ResolveJobReference(store, "", "thread-1")
	if err != nil {
		t.Fatalf("the thread resolved to no job: %v", err)
	}
	if bound != job.ID {
		t.Errorf("the thread resolves to %s, want %s", bound, job.ID)
	}
}

// threadOrigin decides whether a binding is attempted at all, and a nil session
// is every non-Discord caller.
func TestThreadOriginNeedsASessionAndAChannel(t *testing.T) {
	t.Parallel()
	if got := threadOrigin(nil, summonContext{ChannelID: "thread-1"}); got != "" {
		t.Errorf("threadOrigin with no session = %q, want empty", got)
	}
	if got := threadOrigin(nil, summonContext{}); got != "" {
		t.Errorf("threadOrigin with no channel = %q, want empty", got)
	}
}

func mustCommand(t *testing.T, name string) CommandDefinition {
	t.Helper()
	command, declared := LookupCommand(name)
	if !declared {
		t.Fatalf("the %s command is not declared", name)
	}
	return command
}
