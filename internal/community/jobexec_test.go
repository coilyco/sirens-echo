package community

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeRunner records what was asked for without executing anything.
type fakeRunner struct {
	mu        sync.Mutex
	calls     [][]string
	dirs      []string
	failOn    string
	output    string
	truncated bool
}

func (r *fakeRunner) Run(
	_ context.Context,
	workspace *Workspace,
	name string,
	args ...string,
) (CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, append([]string{name}, args...))
	r.dirs = append(r.dirs, workspace.Root)
	if r.failOn == name {
		return CommandResult{ExitCode: 1}, os.ErrInvalid
	}
	return CommandResult{Output: r.output, Truncated: r.truncated}, nil
}

func (r *fakeRunner) recorded() [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func executingJob(t *testing.T, id string) Job {
	t.Helper()
	submission := discordSubmission()
	submission.Kind = "ward-exec"
	job, err := PrepareJob(submission)
	if err != nil {
		t.Fatalf("PrepareJob: %v", err)
	}
	if id != "" {
		job.ID = id
	}
	return job
}

// A job checks out and runs one verb in its own workspace, and the workspace
// is gone afterwards. See #145.
func TestExecutingJobCheckoutRunsAndCleansUp(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &fakeRunner{}
	executor := WardJobExecutor{
		WorkspaceRoot: root,
		Runner:        runner,
		Repository:    "coilyco/sirens-echo",
		Verb:          "test",
	}
	job := executingJob(t, "job-workspace1")

	outcome, err := executor.Execute(context.Background(), job, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if outcome != "test passed" {
		t.Errorf("outcome = %q", outcome)
	}
	calls := runner.recorded()
	if len(calls) != 2 {
		t.Fatalf("calls = %v", calls)
	}
	if calls[0][0] != "git" || calls[0][1] != "clone" {
		t.Errorf("first call was not a checkout: %v", calls[0])
	}
	if calls[1][0] != "exec" || calls[1][1] != "test" {
		t.Errorf("second call was not the verb: %v", calls[1])
	}
	if _, err := os.Stat(filepath.Join(root, job.ID)); !os.IsNotExist(err) {
		t.Errorf("workspace survived the job: %v", err)
	}
}

// Two concurrent jobs cannot observe each other's files.
func TestTwoJobsGetSeparateWorkspaces(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	first, err := OpenWorkspace(root, "job-aaaaaaaa11")
	if err != nil {
		t.Fatalf("OpenWorkspace: %v", err)
	}
	second, err := OpenWorkspace(root, "job-bbbbbbbb22")
	if err != nil {
		t.Fatalf("OpenWorkspace: %v", err)
	}
	if first.Root == second.Root {
		t.Fatalf("two jobs share a workspace: %s", first.Root)
	}
	if err := os.WriteFile(filepath.Join(first.Root, "secret"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(second.Root, "secret")); !os.IsNotExist(err) {
		t.Errorf("one job can see another's files: %v", err)
	}
	if err := first.Remove(); err != nil {
		t.Errorf("Remove: %v", err)
	}
	// Removing twice is safe, because a terminal state has more than one path.
	if err := first.Remove(); err != nil {
		t.Errorf("second Remove: %v", err)
	}
	if _, err := os.Stat(second.Root); err != nil {
		t.Errorf("removing one workspace took another: %v", err)
	}
}

// A failed verb must still clean up, or a red build leaks a tree per attempt.
func TestAFailedVerbStillRemovesTheWorkspace(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	executor := WardJobExecutor{
		WorkspaceRoot: root,
		Runner:        &fakeRunner{failOn: "exec"},
		Repository:    "coilyco/sirens-echo",
		Verb:          "build",
	}
	job := executingJob(t, "job-failedverb")
	if _, err := executor.Execute(context.Background(), job, nil); err == nil {
		t.Fatal("a failing verb reported success")
	}
	if _, err := os.Stat(filepath.Join(root, job.ID)); !os.IsNotExist(err) {
		t.Errorf("workspace survived a failure: %v", err)
	}
}

// An undeclared verb or repository is refused before any workspace exists.
func TestExecutionRefusesUndeclaredVerbsAndRepositories(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	job := executingJob(t, "job-refusals1")

	rogueVerb := WardJobExecutor{
		WorkspaceRoot: root,
		Runner:        &fakeRunner{},
		Repository:    "coilyco/sirens-echo",
		Verb:          "rm-rf",
	}
	if _, err := rogueVerb.Execute(context.Background(), job, nil); err == nil {
		t.Error("an undeclared verb ran")
	}
	rogueRepo := WardJobExecutor{
		WorkspaceRoot: root,
		Runner:        &fakeRunner{},
		Repository:    "somebody/else",
		Verb:          "test",
	}
	if _, err := rogueRepo.Execute(context.Background(), job, nil); err == nil {
		t.Error("an undeclared repository was checked out")
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Errorf("a refused job left %d entries behind", len(entries))
	}
}

// Command output never reaches the outcome, so no build log becomes a reply.
func TestCommandOutputDoesNotReachTheOutcome(t *testing.T) {
	t.Parallel()
	executor := WardJobExecutor{
		WorkspaceRoot: t.TempDir(),
		Runner:        &fakeRunner{output: "token=abc123 secret build log"},
		Repository:    "coilyco/sirens-echo",
		Verb:          "vet",
	}
	outcome, err := executor.Execute(context.Background(), executingJob(t, "job-nooutput1"), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, leaked := range []string{"token", "abc123", "secret"} {
		if strings.Contains(outcome, leaked) {
			t.Errorf("outcome %q leaked %q", outcome, leaked)
		}
	}
}

// Execution runs under pod authority with no per-requester attribution, so a
// widened admission surface has to disable it rather than outrun it.
func TestExecutionIsRefusedWhenAdmissionWidens(t *testing.T) {
	t.Parallel()
	oneAccount := &AccessPolicy{
		Schema:         accessPolicySchema,
		DirectMessages: DirectMessageAccess{Allow: []string{"318190481467244544"}},
	}
	if err := CheckExecutionAdmission(oneAccount); err != nil {
		t.Errorf("a single-account allowlist was refused: %v", err)
	}
	widened := []*AccessPolicy{
		nil,
		{Schema: accessPolicySchema},
		{Schema: accessPolicySchema, DirectMessages: DirectMessageAccess{Allow: []string{"1", "2"}}},
		{
			Schema:         accessPolicySchema,
			DirectMessages: DirectMessageAccess{Allow: []string{"1"}},
			Guilds:         []GuildAccess{{ID: "1300204416229441587"}},
		},
		{
			Schema:         accessPolicySchema,
			DirectMessages: DirectMessageAccess{Allow: []string{"1"}},
			legacyOpenDMs:  true,
		},
	}
	for index, policy := range widened {
		if err := CheckExecutionAdmission(policy); err == nil {
			t.Errorf("case %d: execution was allowed against a widened surface", index)
		}
	}
}

// The guard has to be wired, not merely available.
func TestExecutingKindIsOnlyBuiltWhenTheSurfaceAllowsIt(t *testing.T) {
	t.Parallel()
	safe := &AccessPolicy{
		Schema:         accessPolicySchema,
		DirectMessages: DirectMessageAccess{Allow: []string{"318190481467244544"}},
	}
	cfg := Config{
		JobWorkspaceRoot: t.TempDir(),
		JobRepository:    "coilyco/sirens-echo",
		JobVerb:          "test",
	}
	executors, err := buildExecutingKinds(cfg, safe, nil)
	if err != nil {
		t.Fatalf("buildExecutingKinds: %v", err)
	}
	if _, present := executors["ward-exec"]; !present {
		t.Error("execution was not enabled against a safe surface")
	}

	widened := &AccessPolicy{
		Schema: accessPolicySchema,
		Guilds: []GuildAccess{{ID: "1300204416229441587"}},
	}
	if _, err := buildExecutingKinds(cfg, widened, nil); err == nil {
		t.Error("execution was built against a widened surface")
	}

	// With no workspace root there is no execution at all, which is the default.
	none, err := buildExecutingKinds(Config{}, widened, nil)
	if err != nil {
		t.Fatalf("default posture errored: %v", err)
	}
	if _, present := none["ward-exec"]; present {
		t.Error("execution was enabled without a workspace root")
	}
}

// Once per-principal authority exists, a wider surface is bounded by grants
// rather than by there being only one requester. See #150 and #145.
func TestAGrantTableUnblocksExecutionOnAWiderSurface(t *testing.T) {
	t.Parallel()
	widened := &AccessPolicy{
		Schema: accessPolicySchema,
		Guilds: []GuildAccess{{ID: "1300204416229441587"}},
	}
	if err := CheckExecutionAdmission(widened); err == nil {
		t.Fatal("a widened surface with no grant table permitted execution")
	}
	widened.Grants = GrantTable{Principals: []PrincipalGrant{{
		ID:    "318190481467244544",
		Kinds: Allowlist{IDs: []string{"ward-exec"}},
	}}}
	if err := CheckExecutionAdmission(widened); err != nil {
		t.Errorf("a granted surface still refused execution: %v", err)
	}
	// A table nobody is granted execution in enables nothing, so saying so
	// beats starting a runner that can never run anything.
	ungranted := &AccessPolicy{
		Schema: accessPolicySchema,
		Guilds: []GuildAccess{{ID: "1300204416229441587"}},
		Grants: GrantTable{Principals: []PrincipalGrant{{
			ID:    "318190481467244544",
			Kinds: Allowlist{IDs: []string{"echo"}},
		}}},
	}
	if err := CheckExecutionAdmission(ungranted); err == nil {
		t.Error("execution was enabled with no principal granted it")
	}
}
