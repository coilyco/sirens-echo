package community

import (
	"sort"
	"strings"
	"time"
)

// Every tuning number this package has, in one file. See
// docs/sirens-echo-tuning.md for why they are here and how to change one.

// Model calls: rounds, repairs, and the completion budget ladder
const (
	maxToolRounds      = 6
	maxResponseRepairs = 1
	// maxToolResultBytes bounds one tool result before it re-enters the
	// prompt. Four parallel Eco calls inflated a 6k prompt past 47k.
	maxToolResultBytes = 8 * 1024
	// maxAgentProxyResponseBytes bounds one upstream completion body.
	maxAgentProxyResponseBytes = 2 * 1024 * 1024
	// maxAssemblyPasses guards a future suffix that could grow faster than the
	// answer shrinks. A test reaches it.
	maxAssemblyPasses = 8
	// Completion budget, escalated rather than fixed.
	// See docs/sirens-echo-budget.md.
	baseCompletionTokens = 1800
	maxCompletionTokens  = 3600
	completionBudgetStep = 2
	// budgetRaisesAllowed bounds the escalation so a pathological turn cannot
	// loop. One real rung remains: 1800 to 3600, then exhausted.
	budgetRaisesAllowed = 1
)

// Fetch tool
const (
	// maxFetchBytes bounds a response, because a large body becomes prompt.
	maxFetchBytes = 32 * 1024
	// fetchTimeout bounds a slow host, which would otherwise spend the turn.
	fetchTimeout = 10 * time.Second
)

// MCP: refresh, timeouts, backoff, and grounding size
var (
	// defaultRosterRefresh bounds staleness for a transport that cannot push
	// tools/list_changed. See docs/sirens-echo-mcp-roster.md.
	defaultRosterRefresh = time.Hour
	mcpConnectTimeout    = 10 * time.Second
	mcpListTimeout       = 15 * time.Second
	mcpBackoffMin        = 5 * time.Second
	mcpBackoffMax        = 2 * time.Minute //nolint:unused // read via the override table
	// defaultCallTimeout keeps one tool call well inside the turn budget, so a
	// server that never answers cannot spend the whole turn.
	defaultCallTimeout = 45 * time.Second
	// Grounding bounds. Reference material must not crowd out the turn it is
	// meant to support.
	maxGroundingBytes     = 8 * 1024
	maxGroundingDocuments = 8
)

// Turn progress cadence. Only the wait is written down. Overridable, and the
// derived pair is recomputed. See docs/sirens-echo-tuning-overrides.md.
var (
	// turnProgressAfter is how long a turn runs before it starts reporting. A
	// reply that beats this never posts anything.
	turnProgressAfter = 5 * time.Second
	// turnProgressEvery is the grid every later message releases on, so an edit,
	// a reply, and a failure notice all land on the same beat.
	turnProgressEvery = turnProgressAfter * 2
	// turnLongReplyAfter is when a turn has taken long enough that its reply
	// wants somewhere of its own. Derived, so there is one number to move.
	turnLongReplyAfter = turnProgressAfter + turnProgressEvery*2
)

// Job progress
const (
	// jobProgressEvery bounds how often one job reports to its origin.
	jobProgressEvery = 20 * time.Second
)

// Job execution
const (
	// defaultJobQueueDepth bounds accepted-but-unstarted work. A full queue
	// refuses rather than growing without limit.
	defaultJobQueueDepth = 64
	// defaultJobTimeout bounds one execution. Jobs are long, not unbounded.
	defaultJobTimeout = 30 * time.Minute
)

// Model retry. Only fast failures are retried, so the whole ladder fits well
// inside the turn ceiling. See docs/sirens-echo-model-retry.md.
const (
	modelRetryAttempts = 4
	modelRetryBackoff  = 250 * time.Millisecond
)

// Turn timeouts. Overridable, because these are what a deployment tunes.
// See docs/sirens-echo-tuning-overrides.md.
var (
	defaultRequestTimeout = 3 * time.Minute
	// defaultQueueTimeout bounds the wait for the execution slot. A longer
	// wait answers a conversation that has already moved on.
	defaultQueueTimeout = 30 * time.Second
	// defaultShutdownGrace lets the turns in flight answer before a restart
	// takes them. It fits inside Kubernetes' 30s default kill window.
	defaultShutdownGrace = 15 * time.Second
	// shutdownNoticeGrace is the moment a cancelled turn gets to say why it
	// ended, after which the gateway closes and it could not say anything.
	shutdownNoticeGrace = 3 * time.Second
	// failureNoticeTimeout bounds the notice's own send. It is short because the
	// member has already waited out whatever failed.
	failureNoticeTimeout = 10 * time.Second
	// reactionClearTimeout bounds the tidy-up, which detaches from the turn for
	// the same reason the failure notice does.
	reactionClearTimeout = 10 * time.Second
)

// Workspace commands
const (
	// defaultCommandTimeout bounds one command inside a job's own budget.
	defaultCommandTimeout = 10 * time.Minute
	// maxCommandOutputBytes bounds what one command can return, so a runaway
	// build cannot be read into memory unbounded.
	maxCommandOutputBytes = 64 << 10
)

// Readiness probe
const (
	defaultReadinessTimeout = 5 * time.Second
	// maxReadinessBody bounds the probe response this process will read.
	maxReadinessBody = 16 << 10
)

// Agent-to-agent exchange bound
const (
	// maxAgentExchange bounds consecutive agent-to-agent turns in one channel,
	// because two agents each answering the other is a runaway.
	maxAgentExchange = 4
	// agentExchangeWindow is how long a run of agent turns stays counted. A
	// quiet channel forgets, so an exchange later is a fresh one.
	agentExchangeWindow = 10 * time.Minute
)

// Attachment ingest
const (
	// maxAttachmentBytes stays under the scratchpad's per-file limit, so an
	// oversized upload refuses here with a reason rather than there.
	maxAttachmentBytes = 128 * 1024
	// attachmentFetchTimeout bounds one download inside a turn that already
	// owes the member an answer.
	attachmentFetchTimeout = 10 * time.Second
)

// Scratch space
const (
	// maxScratchFileBytes bounds one file. A Discord message can ask for an
	// unbounded write and the volume is shared with the pod.
	maxScratchFileBytes = 256 * 1024
	// maxScratchEntries bounds a listing, keeping a result inside the turn
	// budget rather than returning a directory of unknown size.
	maxScratchEntries = 200
	// maxScratchMatches bounds a search result for the same reason.
	maxScratchMatches = 100
	// maxScratchDepth bounds nesting so a walk stays cheap.
	maxScratchDepth = 8
	// maxScratchPartitionBytes bounds one requester's footprint, so a single
	// account cannot fill the volume for every other account on it.
	maxScratchPartitionBytes = 4 * 1024 * 1024
)

// Slash command shape, bounded by Discord
const (
	// Discord's limits. A breach fails the whole registration, so one
	// server's prompt would otherwise cost every command.
	maxCommandNameRunes        = 32
	maxCommandDescriptionRunes = 100
	// maxCommandOptions is Discord's ceiling on options per command.
	maxCommandOptions = 25
	// defaultParameterMaxLength bounds a declared string argument.
	defaultParameterMaxLength = 200
	// discordReplyLimit is the send budget for one message. It sits under
	// Discord's own 2000 so a reply the harness extended still arrives whole.
	discordReplyLimit = 1990
	// replyAttachmentBytes bounds the file an overflowing reply is sent as,
	// matching what the scratchpad stores and well under Discord's own floor.
	replyAttachmentBytes = maxScratchFileBytes
	// threadNameRunes is Discord's cap. A longer name is refused outright.
	threadNameRunes = 100
	// threadTitleRunes is what reads whole in a thread list, which is a tighter
	// bound than Discord's. See docs/sirens-echo-thread-title-length.md.
	threadTitleRunes = 50
	// threadPrefillRunes bounds a whole-thread prefill. Six tool rounds already
	// admit more than this. See docs/sirens-echo-thread-prefill.md.
	threadPrefillRunes = 32000
	// threadPrefillPages bounds the Discord round trips one prefill spends.
	threadPrefillPages = 10
	// threadPrefillPageSize is Discord's own per-call maximum.
	threadPrefillPageSize = 100
)

// Block responses
const (
	// maxBlockReasonWords bounds the reason. Every volunteered justification is
	// a handle to pull, and this reply only ever appears at a boundary.
	maxBlockReasonWords = 20
)

// Threads
const (
	// threadArchiveMinutes matches the guild's own hide-after setting, so a
	// thread does not outlive the channel's expectation of it.
	threadArchiveMinutes = 60
)

// Mentions
const (
	// mentionNameRunes is the shortest name worth resolving. A one or two
	// character display name matches too much ordinary prose to be safe.
	mentionNameRunes = 3
)

// Admission and transport
const (
	// defaultRateLimiterCapacity bounds the tracked keys, so an unbounded set
	// of callers cannot grow the limiter without limit.
	defaultRateLimiterCapacity = 4096
	// maxHTTPBody bounds one inbound request body.
	maxHTTPBody = 64 << 10
)

// Repository inventory
const (
	// maxRepoInventoryEntries bounds one listing, so a large organization
	// cannot fill a tool result on its own.
	maxRepoInventoryEntries = 100
	// maxRepoFileBytes bounds one file read, so a large source file cannot fill
	// a tool result on its own.
	maxRepoFileBytes = 64 * 1024
)

// Policy load
const (
	// maxSkillpackBytes bounds the concatenated policy roots, which become the
	// system prompt and are read once at construction.
	maxSkillpackBytes = 256 * 1024
)

// Evaluation. These gate nothing in production and shape the packs only
const (
	// DefaultBoardEpochs repeats every case so the grader reads epoch 1 and the
	// remaining runs stay in the dataset as a failure-spread estimate.
	DefaultBoardEpochs = 5
	// DefaultVerbatimWords is the shingle width for system-prompt leakage. Eight
	// consecutive words is specific enough that a paraphrase cannot reach it.
	DefaultVerbatimWords = 8
	// defaultEvaluationCaseTimeout bounds one case, which never runs in a turn.
	defaultEvaluationCaseTimeout = 5 * time.Minute
)

// tuningOverrides names every number a deployment may set. A table rather than
// a parser per constant, so a name cannot be typed once and read never.
func tuningOverrides() map[string]*time.Duration {
	return map[string]*time.Duration{
		"SIRENS_ECHO_REQUEST_TIMEOUT": &defaultRequestTimeout,
		"SIRENS_ECHO_QUEUE_TIMEOUT":   &defaultQueueTimeout,
		"SIRENS_ECHO_PROGRESS_AFTER":  &turnProgressAfter,
		"SIRENS_ECHO_ROSTER_REFRESH":  &defaultRosterRefresh,
		"SIRENS_ECHO_MCP_CONNECT":     &mcpConnectTimeout,
		"SIRENS_ECHO_MCP_LIST":        &mcpListTimeout,
		"SIRENS_ECHO_TOOL_CALL":       &defaultCallTimeout,
	}
}

// applyTuningOverrides reads the table and recomputes what derives from it. A
// bad value keeps the default. See docs/sirens-echo-tuning-overrides.md.
func applyTuningOverrides(lookup func(string) string) []string {
	applied := make([]string, 0)
	for name, target := range tuningOverrides() {
		raw := strings.TrimSpace(lookup(name))
		if raw == "" {
			continue
		}
		value, err := time.ParseDuration(raw)
		if err != nil || value <= 0 {
			continue
		}
		*target = value
		applied = append(applied, name)
	}
	// Derived, and recomputed after rather than read before. An override that
	// moved the beat and not the threshold would half-work in silence.
	turnProgressEvery = turnProgressAfter * 2
	turnLongReplyAfter = turnProgressAfter + turnProgressEvery*2
	sort.Strings(applied)
	return applied
}
