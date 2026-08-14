package community

import (
	"fmt"
	"strings"
)

// A thread binds to the job it is about, so a follow-up needs no id. The
// binding is on the record. See docs/sirens-echo-commands.md.

// BindJobToThread records the thread a job owns. The binding is set once, so a
// thread cannot be repointed at a different job later.
func BindJobToThread(store JobStore, id, threadID string) (Job, error) {
	if strings.TrimSpace(threadID) == "" {
		return Job{}, fmt.Errorf("job %s: thread id is required", id)
	}
	job, err := store.Get(id)
	if err != nil {
		return Job{}, err
	}
	if job.Origin.ThreadID != "" && job.Origin.ThreadID != threadID {
		return Job{}, fmt.Errorf("job %s is already bound to another thread", id)
	}
	existing, err := store.ListByThread(threadID)
	if err != nil {
		return Job{}, err
	}
	for _, bound := range existing {
		if bound.ID != id {
			return Job{}, fmt.Errorf("thread is already bound to job %s", bound.ID)
		}
	}
	// Not a transition: the state read above goes stale the moment the runner
	// starts the job, and naming it here loses the binding. See issue 620.
	return store.Update(id, func(target *Job) {
		target.Origin.ThreadID = threadID
	})
}

// ResolveThreadJob returns the job a thread is bound to. It is an explicit
// lookup of a recorded binding, never an inference from recent history.
func ResolveThreadJob(store JobStore, threadID string) (Job, bool, error) {
	jobs, err := store.ListByThread(threadID)
	if err != nil {
		return Job{}, false, err
	}
	if len(jobs) == 0 {
		return Job{}, false, nil
	}
	return jobs[0], true, nil
}

// ResolveJobReference picks the job a command is about: the argument when one
// is supplied, otherwise the thread's binding.
func ResolveJobReference(store JobStore, suppliedID, threadID string) (string, error) {
	if trimmed := strings.TrimSpace(suppliedID); trimmed != "" {
		return trimmed, nil
	}
	job, bound, err := ResolveThreadJob(store, threadID)
	if err != nil {
		return "", err
	}
	if !bound {
		return "", fmt.Errorf("name a job, or run this inside a thread bound to one")
	}
	return job.ID, nil
}
