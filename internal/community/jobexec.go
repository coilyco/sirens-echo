package community

import (
	"context"
	"fmt"
	"strings"
)

// The executing job kind. It checks out a repository into its own workspace and
// runs one allowlisted verb. See docs/sirens-echo-execution.md.

// ExecutableVerbs is the closed set of ward verbs a job may run. A verb is a
// capability, so widening this is a reviewed act in this repository.
var ExecutableVerbs = map[string]string{
	"build": "compile the checked-out repository",
	"vet":   "run go vet across the tree",
	"test":  "run the unit test suite",
}

// ExecutableRepositories is the closed set a job may check out. An arbitrary
// clone URL would make the workspace a fetch-anything surface.
var ExecutableRepositories = map[string]string{
	"coilyco/sirens-echo": "https://forgejo.coilysiren.me/coilyco/sirens-echo.git",
}

// WardJobExecutor runs one bounded verb against one checked-out repository.
type WardJobExecutor struct {
	// WorkspaceRoot is the directory per-job workspaces are created under.
	WorkspaceRoot string
	Runner        CommandRunner
	// Repository and Verb are fixed per deployment for now, because nothing
	// yet carries a caller's arguments into a job record.
	Repository string
	Verb       string
}

// Execute prepares the workspace, checks out, runs, and cleans up. The
// workspace is removed on every exit path, including cancellation and panic.
func (e WardJobExecutor) Execute(
	ctx context.Context,
	job Job,
	progress func(string),
) (outcome string, err error) {
	clone, known := ExecutableRepositories[e.Repository]
	if !known {
		return "", fmt.Errorf("repository %q is not executable", e.Repository)
	}
	if _, allowed := ExecutableVerbs[e.Verb]; !allowed {
		return "", fmt.Errorf("verb %q is not executable", e.Verb)
	}
	if e.Runner == nil {
		return "", fmt.Errorf("no command runner is configured")
	}
	workspace, err := OpenWorkspace(e.WorkspaceRoot, job.ID)
	if err != nil {
		return "", err
	}
	// Removed on every exit including a panic, so a crashed job leaves no tree.
	defer func() {
		if removeErr := workspace.Remove(); removeErr != nil && err == nil {
			err = removeErr
		}
	}()

	if progress != nil {
		progress("checking out the repository")
	}
	if _, err := e.Runner.Run(ctx, workspace, "git", "clone", "--depth", "1", clone, "."); err != nil {
		return "", fmt.Errorf("checkout failed")
	}
	if progress != nil {
		progress(fmt.Sprintf("running %s", e.Verb))
	}
	result, err := e.Runner.Run(ctx, workspace, "exec", e.Verb)
	if err != nil {
		return "", fmt.Errorf("verb %s failed", e.Verb)
	}
	// The outcome is a harness phrase, so no command output reaches a member.
	if result.Truncated {
		return fmt.Sprintf("%s passed, output truncated", e.Verb), nil
	}
	return fmt.Sprintf("%s passed", e.Verb), nil
}

// buildExecutingKinds adds the executing kind when the deployment enabled it
// and the admission surface allows it.
func buildExecutingKinds(
	cfg Config, policy *AccessPolicy, telemetry *Telemetry,
) (map[string]JobExecutor, error) {
	executors := DefaultJobExecutors()
	if strings.TrimSpace(cfg.JobWorkspaceRoot) == "" {
		return executors, nil
	}
	if err := CheckExecutionAdmission(policy); err != nil {
		return nil, err
	}
	executors["ward-exec"] = WardJobExecutor{
		WorkspaceRoot: cfg.JobWorkspaceRoot,
		Runner:        WardCommandRunner{Telemetry: telemetry},
		Repository:    cfg.JobRepository,
		Verb:          cfg.JobVerb,
	}
	return executors, nil
}
