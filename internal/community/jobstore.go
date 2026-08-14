package community

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// The store is an interface because the deployment picks the backend. See
// docs/sirens-echo-jobs.md.

// ErrJobNotFound is returned for an id the store does not hold.
var ErrJobNotFound = errors.New("job not found")

// JobStore holds job records and survives whatever restart its durability
// promises. Concurrent use means goroutines in one process, never two.
type JobStore interface {
	// Submit records a job, or returns the existing one when the idempotency
	// key has been seen. The second return reports whether it already existed.
	Submit(job Job) (Job, bool, error)
	// Get returns one job by id, or ErrJobNotFound.
	Get(id string) (Job, error)
	// ListByPrincipal returns a principal's jobs, newest first.
	ListByPrincipal(principal string) ([]Job, error)
	// ListByThread returns the jobs bound to a thread, newest first. The
	// binding lives on the record, so this is a query and not a second index.
	ListByThread(threadID string) ([]Job, error)
	// Transition moves a job under the state machine, applying mutate before
	// the write. A refused move leaves the stored record untouched.
	Transition(id string, next JobState, mutate func(*Job)) (Job, error)
	// Update applies mutate to a stored job without moving it under the state
	// machine, for a writer whose subject is not the state. See issue 620.
	Update(id string, mutate func(*Job)) (Job, error)
}

// Clock is the store's only source of time, so a test does not sleep.
type Clock func() time.Time

// MemoryJobStore holds jobs in memory: every behaviour except durability,
// which makes it right for a test and wrong for a deployment.
type MemoryJobStore struct {
	Now Clock

	mu    sync.Mutex
	byID  map[string]Job
	byKey map[string]string
}

// NewMemoryJobStore returns an empty store.
func NewMemoryJobStore(now Clock) *MemoryJobStore {
	return &MemoryJobStore{
		Now:   now,
		byID:  make(map[string]Job),
		byKey: make(map[string]string),
	}
}

func (s *MemoryJobStore) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func (s *MemoryJobStore) Submit(job Job) (Job, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.submitLocked(job)
}

func (s *MemoryJobStore) submitLocked(job Job) (Job, bool, error) {
	if job.CreatedAt.IsZero() {
		job.CreatedAt = s.now()
	}
	job.UpdatedAt = job.CreatedAt
	if job.State == "" {
		job.State = JobQueued
	}
	if err := job.Validate(); err != nil {
		return Job{}, false, err
	}
	if existingID, seen := s.byKey[job.IdempotencyKey]; seen {
		return s.byID[existingID], true, nil
	}
	if _, taken := s.byID[job.ID]; taken {
		return Job{}, false, fmt.Errorf("job %s already exists under a different key", job.ID)
	}
	s.byID[job.ID] = job
	s.byKey[job.IdempotencyKey] = job.ID
	return job, false, nil
}

func (s *MemoryJobStore) Get(id string) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.byID[id]
	if !ok {
		return Job{}, fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}
	return job, nil
}

func (s *MemoryJobStore) ListByPrincipal(principal string) ([]Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	jobs := make([]Job, 0, len(s.byID))
	for _, job := range s.byID {
		if job.Principal == principal {
			jobs = append(jobs, job)
		}
	}
	sortJobsNewestFirst(jobs)
	return jobs, nil
}

func (s *MemoryJobStore) ListByThread(threadID string) ([]Job, error) {
	if threadID == "" {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	jobs := make([]Job, 0, 1)
	for _, job := range s.byID {
		if job.Origin.ThreadID == threadID {
			jobs = append(jobs, job)
		}
	}
	sortJobsNewestFirst(jobs)
	return jobs, nil
}

func (s *MemoryJobStore) Transition(id string, next JobState, mutate func(*Job)) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, err := s.transitionLocked(id, next, mutate)
	if err != nil {
		return Job{}, err
	}
	s.byID[id] = job
	return job, nil
}

func (s *MemoryJobStore) Update(id string, mutate func(*Job)) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, err := s.updateLocked(id, mutate)
	if err != nil {
		return Job{}, err
	}
	s.byID[id] = job
	return job, nil
}

// updateLocked applies the caller's mutation and nothing else, leaving the
// caller to persist. No state is named, so there is no move to refuse.
func (s *MemoryJobStore) updateLocked(id string, mutate func(*Job)) (Job, error) {
	job, ok := s.byID[id]
	if !ok {
		return Job{}, fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}
	if mutate != nil {
		mutate(&job)
	}
	job.UpdatedAt = s.now()
	if err := job.Validate(); err != nil {
		return Job{}, err
	}
	return job, nil
}

// transitionLocked applies the machine and the caller's mutation, leaving the
// caller to persist. The stored record is untouched on a refusal.
func (s *MemoryJobStore) transitionLocked(
	id string,
	next JobState,
	mutate func(*Job),
) (Job, error) {
	job, ok := s.byID[id]
	if !ok {
		return Job{}, fmt.Errorf("%w: %s", ErrJobNotFound, id)
	}
	if !next.Valid() {
		return Job{}, fmt.Errorf("job %s: unknown target state %q", id, next)
	}
	if !job.State.CanTransitionTo(next) {
		return Job{}, jobTransitionError{ID: id, From: job.State, To: next}
	}
	moment := s.now()
	if next == JobRunning && job.State != JobRunning {
		job.StartedAt = moment
		job.Attempts++
	}
	if next.Terminal() && !job.State.Terminal() {
		job.EndedAt = moment
	}
	job.State = next
	job.UpdatedAt = moment
	if mutate != nil {
		mutate(&job)
	}
	if err := job.Validate(); err != nil {
		return Job{}, err
	}
	return job, nil
}

func sortJobsNewestFirst(jobs []Job) {
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].CreatedAt.Equal(jobs[j].CreatedAt) {
			return jobs[i].ID < jobs[j].ID
		}
		return jobs[i].CreatedAt.After(jobs[j].CreatedAt)
	})
}

// FileJobStore persists each job as one JSON file, so the record survives a
// pod restart on any volume the deployment mounts.
type FileJobStore struct {
	*MemoryJobStore
	dir string
}

// OpenFileJobStore loads whatever is already on disk, which is what makes a
// restart recoverable rather than merely non-fatal.
func OpenFileJobStore(dir string, now Clock) (*FileJobStore, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("job store directory is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create job store: %w", err)
	}
	store := &FileJobStore{MemoryJobStore: NewMemoryJobStore(now), dir: dir}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read job store: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read job %s: %w", entry.Name(), err)
		}
		var job Job
		if err := json.Unmarshal(raw, &job); err != nil {
			return nil, fmt.Errorf("parse job %s: %w", entry.Name(), err)
		}
		if err := job.Validate(); err != nil {
			return nil, fmt.Errorf("stored job %s is invalid: %w", entry.Name(), err)
		}
		store.byID[job.ID] = job
		store.byKey[job.IdempotencyKey] = job.ID
	}
	return store, nil
}

func (s *FileJobStore) Submit(job Job) (Job, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, existed, err := s.submitLocked(job)
	if err != nil || existed {
		return stored, existed, err
	}
	if err := s.persist(stored); err != nil {
		// Roll the memory view back, so a failed write cannot leave a job that
		// exists in this process and nowhere else.
		delete(s.byID, stored.ID)
		delete(s.byKey, stored.IdempotencyKey)
		return Job{}, false, err
	}
	return stored, false, nil
}

func (s *FileJobStore) Transition(id string, next JobState, mutate func(*Job)) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, err := s.transitionLocked(id, next, mutate)
	if err != nil {
		return Job{}, err
	}
	if err := s.persist(job); err != nil {
		return Job{}, err
	}
	s.byID[id] = job
	return job, nil
}

func (s *FileJobStore) Update(id string, mutate func(*Job)) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, err := s.updateLocked(id, mutate)
	if err != nil {
		return Job{}, err
	}
	if err := s.persist(job); err != nil {
		return Job{}, err
	}
	s.byID[id] = job
	return job, nil
}

// persist writes through a temporary file and renames, so a crash mid-write
// leaves the previous record rather than a truncated one.
func (s *FileJobStore) persist(job Job) error {
	raw, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return fmt.Errorf("encode job %s: %w", job.ID, err)
	}
	final := filepath.Join(s.dir, job.ID+".json")
	temporary := final + ".tmp"
	if err := os.WriteFile(temporary, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write job %s: %w", job.ID, err)
	}
	if err := os.Rename(temporary, final); err != nil {
		return fmt.Errorf("commit job %s: %w", job.ID, err)
	}
	return nil
}

// RecoverStrandedJobs moves jobs a crash left mid-flight to a terminal state.
// Only running and cancelling can be stranded; queued is still accurate.
func RecoverStrandedJobs(store JobStore, ids []string, outcome string) ([]Job, error) {
	recovered := make([]Job, 0, len(ids))
	for _, id := range ids {
		job, err := store.Get(id)
		if err != nil {
			return nil, err
		}
		if job.State != JobRunning && job.State != JobCancelling {
			continue
		}
		next := JobFailed
		if job.State == JobCancelling {
			next = JobCancelled
		}
		moved, err := store.Transition(id, next, func(target *Job) {
			target.Outcome = outcome
		})
		if err != nil {
			return nil, err
		}
		recovered = append(recovered, moved)
	}
	return recovered, nil
}

// StrandedJobIDs lists the jobs a restart found mid-flight.
func StrandedJobIDs(jobs []Job) []string {
	ids := make([]string, 0, len(jobs))
	for _, job := range jobs {
		if job.State == JobRunning || job.State == JobCancelling {
			ids = append(ids, job.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

// All returns every stored job, newest first. Used by restart recovery, which
// has no principal to scope by.
func (s *MemoryJobStore) All() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	jobs := make([]Job, 0, len(s.byID))
	for _, job := range s.byID {
		jobs = append(jobs, job)
	}
	sortJobsNewestFirst(jobs)
	return jobs
}
