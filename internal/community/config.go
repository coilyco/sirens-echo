package community

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Every number this service ships, and the environment name that sets each
// one. See docs/sirens-echo-tuning.md.

// knobValue is the two shapes a number takes here.
type knobValue interface{ int | time.Duration }

// knob binds one number to one environment name, so a number cannot exist in
// this file without a way to set it.
type knob struct {
	env      string
	fallback string
	// value renders what the variable holds now, so a test can pin the
	// declaration against the thing it declares.
	value func() string
	apply func(raw string) bool
	reset func()
}

// overridable is the one helper every number goes through. It takes where the
// package reads the value, what sets it, and what it holds without one.
func overridable[T knobValue](target *T, env string, fallback T) knob {
	return knob{
		env:      env,
		fallback: fmt.Sprint(fallback),
		value:    func() string { return fmt.Sprint(*target) },
		apply: func(raw string) bool {
			value, ok := parseKnob[T](raw)
			// Zero and below are refused alike: every one of these is a bound,
			// a count, or a wait, and none means anything there.
			if !ok || value <= 0 {
				return false
			}
			*target = value
			return true
		},
		reset: func() { *target = fallback },
	}
}

// parseKnob reads the one text form each shape has. A duration takes Go's own
// spelling, so `90s` works and a bare `90` does not.
func parseKnob[T knobValue](raw string) (T, bool) {
	var zero T
	switch any(zero).(type) {
	case time.Duration:
		value, err := time.ParseDuration(raw)
		if err != nil {
			return zero, false
		}
		return any(value).(T), true
	case int:
		value, err := strconv.Atoi(raw)
		if err != nil {
			return zero, false
		}
		return any(value).(T), true
	}
	return zero, false
}

// Model calls: rounds, repairs, and the completion budget ladder
var (
	maxToolRounds      int
	maxResponseRepairs int
	// turnModelCalls bounds a turn where maxToolRounds bounds one completion of
	// the several a turn makes. See docs/sirens-echo-turn-stages.md.
	turnModelCalls int
	// maxToolResultBytes bounds one tool result before it re-enters the
	// prompt. Four parallel Eco calls inflated a 6k prompt past 47k.
	maxToolResultBytes int
	// maxAgentProxyResponseBytes bounds one upstream completion body.
	maxAgentProxyResponseBytes int
	// maxAssemblyPasses guards a future suffix that could grow faster than the
	// answer shrinks. A test reaches it.
	maxAssemblyPasses int
	// Completion budget, escalated rather than fixed.
	// See docs/sirens-echo-model-call.md.
	baseCompletionTokens int
	maxCompletionTokens  int
	completionBudgetStep int
	// budgetRaisesAllowed bounds the escalation so a pathological turn cannot
	// loop. One real rung remains: 1800 to 3600, then exhausted.
	budgetRaisesAllowed int
)

// Fetch tool
var (
	// maxFetchBytes bounds a response, because a large body becomes prompt.
	maxFetchBytes int
	// fetchTimeout bounds a slow host, which would otherwise spend the turn.
	fetchTimeout time.Duration
)

// MCP: refresh, timeouts, backoff, and grounding size
var (
	// defaultRosterRefresh bounds staleness for a transport that cannot push
	// tools/list_changed. See docs/sirens-echo-mcp.md.
	defaultRosterRefresh time.Duration
	mcpConnectTimeout    time.Duration
	mcpListTimeout       time.Duration
	mcpBackoffMin        time.Duration
	mcpBackoffMax        time.Duration
	// defaultCallTimeout keeps one tool call well inside the turn budget, so a
	// server that never answers cannot spend the whole turn.
	defaultCallTimeout time.Duration
	// Grounding bounds. Reference material must not crowd out the turn it is
	// meant to support.
	maxGroundingBytes int
	// maxServerGuidanceBytes bounds one server's own instructions, which the
	// server writes and this prompt carries. See sirens-echo#647.
	maxServerGuidanceBytes int
	maxGroundingDocuments  int
	// reexportRefreshInterval bounds staleness for the roster advertised over
	// /mcp. See docs/sirens-echo-http.md and sirens-echo#1025.
	reexportRefreshInterval time.Duration
)

// Turn progress cadence. Only the wait is a knob. The pair below it is derived,
// so it is set by setting the wait. See docs/sirens-echo-tuning.md.
var (
	// turnProgressAfter is how long a turn runs before it starts reporting. A
	// reply that beats this never posts anything.
	turnProgressAfter time.Duration
	// turnProgressEvery is the grid every later message releases on, so an edit,
	// a reply, and a failure notice all land on the same beat.
	turnProgressEvery time.Duration
	// turnLongReplyAfter is when a turn has taken long enough that its reply
	// wants somewhere of its own.
	turnLongReplyAfter time.Duration
)

// Job progress and execution
var (
	// jobProgressEvery bounds how often one job reports to its origin.
	jobProgressEvery time.Duration
	// defaultJobQueueDepth bounds accepted-but-unstarted work. A full queue
	// refuses rather than growing without limit.
	defaultJobQueueDepth int
	// defaultJobTimeout bounds one execution. Jobs are long, not unbounded.
	defaultJobTimeout time.Duration
	// The pair Kai decided on sirens-echo#236: ten messages or ten minutes,
	// whichever comes first.
	maxJobContentMessages int
	maxJobContentWindow   time.Duration
)

// Tool-call mirror. Metadata only, and off the turn's path entirely.
// See docs/sirens-echo-tool-markup.md.
var (
	// trajectoryIdle ends a turn's trajectory once its calls stop. It bounds the
	// gap BETWEEN calls rather than the turn. sirens-echo#1041.
	trajectoryIdle time.Duration
	// trajectoryLifetime closes a trajectory. With nothing polling the mirror's
	// queue it is the only closer. See docs/sirens-echo-tool-markup.md.
	trajectoryLifetime time.Duration
)

// Model retry. Only fast failures are retried, so the whole ladder fits well
// inside the turn ceiling. See docs/sirens-echo-model-call.md.
var (
	modelRetryAttempts int
	modelRetryBackoff  time.Duration
	// modelIdleTimeout bounds silence rather than the whole call, so a backend
	// still sending heartbeats is not cut. See docs/sirens-echo-model-call.md.
	modelIdleTimeout time.Duration
)

// Turn concurrency
var (
	// executionSlots multiplexes I/O rather than compute, a turn being almost
	// all wait on a hosted model. See docs/sirens-echo-admission.md.
	executionSlots int
)

// Turn timeouts
var (
	defaultRequestTimeout time.Duration
	// defaultQueueTimeout bounds the wait for a slot. A longer wait answers a
	// conversation that has already moved on.
	defaultQueueTimeout time.Duration
	// defaultShutdownGrace lets the turns in flight answer before a restart
	// takes them. It fits inside Kubernetes' 30s default kill window.
	defaultShutdownGrace time.Duration
	// shutdownNoticeGrace is the moment a cancelled turn gets to say why it
	// ended, after which the gateway closes and it could not say anything.
	shutdownNoticeGrace time.Duration
	// failureNoticeTimeout bounds the notice's own send. It is short because the
	// member has already waited out whatever failed.
	failureNoticeTimeout time.Duration
	// reactionClearTimeout bounds the tidy-up, which detaches from the turn for
	// the same reason the failure notice does.
	reactionClearTimeout time.Duration
)

// Workspace commands and the readiness probe
var (
	// defaultCommandTimeout bounds one command inside a job's own budget.
	defaultCommandTimeout time.Duration
	// maxCommandOutputBytes bounds what one command can return, so a runaway
	// build cannot be read into memory unbounded.
	maxCommandOutputBytes int
	// commandKillGrace bounds the wait after the deadline kills a command, so a
	// grandchild holding the pipe cannot outlive it. See sirens-echo#892.
	commandKillGrace        time.Duration
	defaultReadinessTimeout time.Duration
	// maxReadinessBody bounds the probe response this process will read.
	maxReadinessBody int
)

// Agent-to-agent exchange bound
var (
	// maxAgentExchange bounds consecutive agent-to-agent turns in one channel,
	// because two agents each answering the other is a runaway.
	maxAgentExchange int
	// agentExchangeWindow is how long a run of agent turns stays counted. A
	// quiet channel forgets, so an exchange later is a fresh one.
	agentExchangeWindow time.Duration
)

// Attachment ingest
var (
	// maxAttachmentBytes stays under the scratchpad's per-file limit, so an
	// oversized upload refuses here with a reason rather than there.
	maxAttachmentBytes int
	// attachmentFetchTimeout bounds one download inside a turn that already
	// owes the member an answer.
	attachmentFetchTimeout time.Duration
)

// Scratch space
var (
	// maxScratchFileBytes bounds one file. A Discord message can ask for an
	// unbounded write and the volume is shared with the pod.
	maxScratchFileBytes int
	// maxScratchEntries bounds a listing, keeping a result inside the turn
	// budget rather than returning a directory of unknown size.
	maxScratchEntries int
	// maxScratchMatches bounds a search result for the same reason.
	maxScratchMatches int
	// maxScratchMatchRunes bounds one hit, since a minified file is one line
	// and a single match could otherwise be the whole result.
	maxScratchMatchRunes int
	// maxScratchDepth bounds nesting so a walk stays cheap.
	maxScratchDepth int
	// maxSessionBytes bounds one shared workspace, which is the bound that
	// stops a thread accumulating under a rule that never expires it.
	maxSessionBytes int
	// threadSessionRetention is the quiet period before a thread workspace is
	// collected. A thread is a conversation people return to across a week.
	threadSessionRetention time.Duration
	// directSessionRetention collects a channel pairing, which has no natural
	// end and is permanent storage without one.
	directSessionRetention time.Duration
	// sessionSweepEvery paces the collector. A cleanup that never fires is
	// indistinguishable from no retention policy.
	sessionSweepEvery time.Duration
	// maxScratchPartitionBytes bounds one requester's footprint, so a single
	// account cannot fill the volume for every other account on it.
	maxScratchPartitionBytes int
)

// Discord's own shape. Raising one of these past what Discord accepts fails at
// Discord rather than here. See docs/sirens-echo-tuning.md.
var (
	maxCommandNameRunes        int
	maxCommandDescriptionRunes int
	// maxCommandOptions is Discord's ceiling on options per command.
	maxCommandOptions int
	// defaultParameterMaxLength bounds a declared string argument.
	defaultParameterMaxLength int
	// threadPrefillBytes is the context budget for a whole-thread prefill.
	// Oldest messages drop until the transcript fits. See sirens-echo#769.
	threadPrefillBytes int
	// threadPrefillPage is Discord's own ceiling on one history call.
	threadPrefillPage int
	// threadPrefillReads bounds the walk, so a pathological thread costs a
	// known number of calls rather than an unknown one.
	threadPrefillReads int
	// discordReplyLimit is the send budget for one message. It sits under
	// Discord's own 2000 so a reply the harness extended still arrives whole.
	discordReplyLimit int
	// mcpsReplyBudget leaves room under discordReplyLimit for /mcps's own
	// truncation notice, so a long roster is cut with a line saying so.
	mcpsReplyBudget int
	// mcpToolSummaryRunes bounds one tool's own description inside /mcp, so a
	// verbose server cannot spend the whole reply on its first tool.
	mcpToolSummaryRunes int
	// pollQuestionRunes is Discord's own cap on /poll's question.
	pollQuestionRunes int
	// pollAnswerRunes is Discord's own cap on one /poll answer.
	pollAnswerRunes int
	// pollDefaultDuration is how long a /poll runs when the member names no
	// duration. A deployment's own call, not Discord's.
	pollDefaultDuration int
	// pollMaxDuration is Discord's own cap on how long a poll may run, 32 days
	// in hours. A deployment may tighten it, never loosen it.
	pollMaxDuration int
	// replyAttachmentBytes bounds the file an overflowing reply is sent as.
	// Derived, so the scratchpad's limit is the one number to move.
	replyAttachmentBytes int
	// threadNameRunes is Discord's cap. A longer name is refused outright.
	threadNameRunes int
	// threadTitleRunes is what reads whole in a thread list, which is a tighter
	// bound than Discord's. See docs/sirens-echo-threads.md.
	threadTitleRunes int
	// threadTitleWords is what the titler is asked for, and the number that
	// actually decides title length. See docs/sirens-echo-threads.md.
	threadTitleWords int
	// threadArchiveMinutes matches the guild's own hide-after setting, so a
	// thread does not outlive the channel's expectation of it.
	threadArchiveMinutes int
)

// Reply rendering bounds
var (
	// maxProgressRows bounds how tall the progress element grows on either
	// surface: worklog rows in the embed, wait lines in the notice fallback.
	maxProgressRows int
	// maxCalculatorRunes bounds one expression, so a pasted document cannot
	// arrive as arithmetic. See docs/sirens-echo-tools.md.
	maxCalculatorRunes int
	// maxCalculatorDigits bounds one literal inside it.
	maxCalculatorDigits int
	// maxCalculatorExponent bounds a power, which grows fastest of the four.
	maxCalculatorExponent int
	// maxCalculatorPlaces is how far an inexact result is reported. A third is
	// not a decimal, so the answer says it was rounded rather than implying it.
	maxCalculatorPlaces int
	// maxProxyToolNameBytes bounds one served tool name.
	maxProxyToolNameBytes int
	// maxRedactedBlocks bounds the removal. Past it this is no longer a message
	// with a bad block in it, and the member is better served by the refusal.
	maxRedactedBlocks int
	// inventedChannelRunes bounds the one piece of model-written text a refusal
	// carries into telemetry. See docs/sirens-echo-delivery.md.
	inventedChannelRunes int
	// maxObjectEmoji bounds the legibility emoji a neutral reply may carry.
	// Kai set it on sirens-echo#203 after declining every earlier bound.
	maxObjectEmoji int
	// maxBlockReasonWords bounds the reason. Every volunteered justification is
	// a handle to pull, and this reply only ever appears at a boundary.
	maxBlockReasonWords int
	// mentionNameRunes is the shortest name worth resolving. A one or two
	// character display name matches too much ordinary prose to be safe.
	mentionNameRunes int
)

// Admission, transport, inventory, and policy load
var (
	// defaultRateLimiterCapacity bounds the tracked keys, so an unbounded set
	// of callers cannot grow the limiter without limit.
	defaultRateLimiterCapacity int
	// maxHTTPBody bounds one inbound request body.
	maxHTTPBody int
	// maxRepoInventoryEntries bounds one listing, so a large organization
	// cannot fill a tool result on its own.
	maxRepoInventoryEntries int
	// maxRepoFileBytes bounds one file read, so a large source file cannot fill
	// a tool result on its own.
	maxRepoFileBytes int
	// maxTrackerSearchRows bounds a tracker search, which reads a window of a
	// view. See docs/sirens-echo-issues.md.
	maxTrackerSearchRows int
	// maxSkillpackBytes bounds the concatenated policy roots, which become the
	// system prompt and are read once at construction.
	maxSkillpackBytes int
)

// The coalescing lane, which is off until SIRENS_ECHO_COALESCE_ENABLED turns it
// on. See docs/sirens-echo-admission.md.
var (
	// coalesceCapacity bounds the asks waiting for the window. Past it the
	// oldest is shed, because a gateway handler must never block on a turn.
	coalesceCapacity int
	// coalesceWindow and coalesceBatch are the narrow window: a member's rapid
	// follow-ups share a turn without a first comment waiting on a slow one.
	coalesceWindow time.Duration
	coalesceBatch  int
	// The wide pair coalesces harder once the backlog says the narrow window is
	// not keeping up.
	coalesceWideWindow time.Duration
	coalesceWideBatch  int
	// coalesceAgeCap is the oldest an ask may be before it forces the window
	// shut, so a steady stream cannot postpone the comment that opened it.
	coalesceAgeCap time.Duration
	// coalesceWorkers drain batches concurrently, one writer per member.
	coalesceWorkers int
	// coalesceHighWater is where widening starts. Derived, so it is set by
	// setting the pool and the narrow batch.
	coalesceHighWater int
)

// Evaluation. These gate nothing in production and shape the packs only
var (
	// DefaultBoardEpochs repeats every case so the grader reads epoch 1 and the
	// remaining runs stay in the dataset as a failure-spread estimate.
	DefaultBoardEpochs int
	// DefaultVerbatimWords is the shingle width for system-prompt leakage. Eight
	// consecutive words is specific enough that a paraphrase cannot reach it.
	DefaultVerbatimWords int
	// defaultEvaluationCaseTimeout bounds one case, which never runs in a turn.
	defaultEvaluationCaseTimeout time.Duration
)

// route.jev. Gates nothing by itself: JevModel unset (Config, not a knob)
// skips the stage entirely regardless of these.
var (
	// jevTimeout bounds one systemone call (route.jev, jev.go).
	jevTimeout time.Duration
	// jevSplitAt is the question count above which route.jev sends two
	// parallel requests instead of one. See the spec's "measure first" step.
	jevSplitAt int
)

// knobs is every number and every name, one line each. Adding a number here is
// the only way to add one, which is what keeps the list complete.
func knobs() []knob {
	return []knob{
		overridable(&jevTimeout, "SIRENS_ECHO_JEV_TIMEOUT", 10*time.Second),
		overridable(&jevSplitAt, "SIRENS_ECHO_JEV_SPLIT_AT", 60),
		overridable(&maxToolRounds, "SIRENS_ECHO_TOOL_ROUNDS", 6),
		overridable(&turnModelCalls, "SIRENS_ECHO_TURN_MODEL_CALLS", 24),
		overridable(&maxResponseRepairs, "SIRENS_ECHO_RESPONSE_REPAIRS", 1),
		overridable(&maxToolResultBytes, "SIRENS_ECHO_TOOL_RESULT_BYTES", 8*1024),
		overridable(&maxAgentProxyResponseBytes, "SIRENS_ECHO_PROXY_RESPONSE_BYTES", 2*1024*1024),
		overridable(&maxAssemblyPasses, "SIRENS_ECHO_ASSEMBLY_PASSES", 8),
		overridable(&baseCompletionTokens, "SIRENS_ECHO_BASE_COMPLETION_TOKENS", 1800),
		overridable(&completionBudgetStep, "SIRENS_ECHO_COMPLETION_BUDGET_STEP", 2),
		overridable(&budgetRaisesAllowed, "SIRENS_ECHO_BUDGET_RAISES", 1),

		overridable(&maxFetchBytes, "SIRENS_ECHO_FETCH_BYTES", 32*1024),
		overridable(&fetchTimeout, "SIRENS_ECHO_FETCH_TIMEOUT", 10*time.Second),

		overridable(&defaultRosterRefresh, "SIRENS_ECHO_ROSTER_REFRESH", time.Hour),
		overridable(&mcpConnectTimeout, "SIRENS_ECHO_MCP_CONNECT", 10*time.Second),
		overridable(&mcpListTimeout, "SIRENS_ECHO_MCP_LIST", 15*time.Second),
		overridable(&mcpBackoffMin, "SIRENS_ECHO_MCP_BACKOFF_MIN", 5*time.Second),
		overridable(&mcpBackoffMax, "SIRENS_ECHO_MCP_BACKOFF_MAX", 2*time.Minute),
		overridable(&maxGroundingDocuments, "SIRENS_ECHO_GROUNDING_DOCUMENTS", 8),
		overridable(&reexportRefreshInterval, "SIRENS_ECHO_REEXPORT_REFRESH", time.Minute),

		overridable(&turnProgressAfter, "SIRENS_ECHO_PROGRESS_AFTER", 10*time.Second),

		overridable(&jobProgressEvery, "SIRENS_ECHO_JOB_PROGRESS_EVERY", 20*time.Second),
		overridable(&defaultJobQueueDepth, "SIRENS_ECHO_JOB_QUEUE_DEPTH", 64),
		overridable(&defaultJobTimeout, "SIRENS_ECHO_JOB_TIMEOUT", 30*time.Minute),
		overridable(&maxJobContentMessages, "SIRENS_ECHO_JOB_CONTENT_MESSAGES", 10),
		overridable(&maxJobContentWindow, "SIRENS_ECHO_JOB_CONTENT_WINDOW", 10*time.Minute),

		overridable(&modelRetryAttempts, "SIRENS_ECHO_MODEL_RETRY_ATTEMPTS", 4),
		overridable(&modelRetryBackoff, "SIRENS_ECHO_MODEL_RETRY_BACKOFF", 250*time.Millisecond),

		overridable(&trajectoryIdle, "SIRENS_ECHO_TRAJECTORY_IDLE", 2*time.Minute),

		overridable(&executionSlots, "SIRENS_ECHO_EXECUTION_SLOTS", 8),

		overridable(&defaultRequestTimeout, "SIRENS_ECHO_REQUEST_TIMEOUT", 3*time.Minute),
		overridable(&defaultShutdownGrace, "SIRENS_ECHO_SHUTDOWN_GRACE", 15*time.Second),
		overridable(&shutdownNoticeGrace, "SIRENS_ECHO_SHUTDOWN_NOTICE_GRACE", 3*time.Second),
		overridable(&failureNoticeTimeout, "SIRENS_ECHO_FAILURE_NOTICE_TIMEOUT", 10*time.Second),
		overridable(&reactionClearTimeout, "SIRENS_ECHO_REACTION_CLEAR_TIMEOUT", 10*time.Second),

		overridable(&defaultCommandTimeout, "SIRENS_ECHO_COMMAND_TIMEOUT", 10*time.Minute),
		overridable(&maxCommandOutputBytes, "SIRENS_ECHO_COMMAND_OUTPUT_BYTES", 64<<10),
		overridable(&commandKillGrace, "SIRENS_ECHO_COMMAND_KILL_GRACE", 5*time.Second),
		overridable(&defaultReadinessTimeout, "SIRENS_ECHO_READINESS_TIMEOUT", 5*time.Second),
		overridable(&maxReadinessBody, "SIRENS_ECHO_READINESS_BODY_BYTES", 16<<10),

		overridable(&maxAgentExchange, "SIRENS_ECHO_AGENT_EXCHANGE", 4),
		overridable(&agentExchangeWindow, "SIRENS_ECHO_AGENT_EXCHANGE_WINDOW", 10*time.Minute),

		overridable(&maxAttachmentBytes, "SIRENS_ECHO_ATTACHMENT_BYTES", 128*1024),
		overridable(&attachmentFetchTimeout, "SIRENS_ECHO_ATTACHMENT_FETCH_TIMEOUT", 10*time.Second),

		overridable(&maxScratchFileBytes, "SIRENS_ECHO_SCRATCH_FILE_BYTES", 256*1024),
		overridable(&maxScratchEntries, "SIRENS_ECHO_SCRATCH_ENTRIES", 200),
		overridable(&maxScratchMatches, "SIRENS_ECHO_SCRATCH_MATCHES", 100),
		overridable(&maxScratchMatchRunes, "SIRENS_ECHO_SCRATCH_MATCH_RUNES", 240),
		overridable(&maxScratchDepth, "SIRENS_ECHO_SCRATCH_DEPTH", 8),
		overridable(&maxSessionBytes, "SIRENS_ECHO_SESSION_BYTES", 1024*1024),
		overridable(&threadSessionRetention, "SIRENS_ECHO_THREAD_SESSION_RETENTION", 7*24*time.Hour),
		overridable(&directSessionRetention, "SIRENS_ECHO_DIRECT_SESSION_RETENTION", time.Hour),
		overridable(&sessionSweepEvery, "SIRENS_ECHO_SESSION_SWEEP", 10*time.Minute),

		overridable(&maxCommandNameRunes, "SIRENS_ECHO_COMMAND_NAME_RUNES", 32),
		overridable(&maxCommandDescriptionRunes, "SIRENS_ECHO_COMMAND_DESCRIPTION_RUNES", 100),
		overridable(&maxCommandOptions, "SIRENS_ECHO_COMMAND_OPTIONS", 25),
		overridable(&defaultParameterMaxLength, "SIRENS_ECHO_PARAMETER_MAX_LENGTH", 200),
		overridable(&threadPrefillBytes, "SIRENS_ECHO_THREAD_PREFILL_BYTES", 32*1024),
		overridable(&threadPrefillPage, "SIRENS_ECHO_THREAD_PREFILL_PAGE", 100),
		overridable(&threadPrefillReads, "SIRENS_ECHO_THREAD_PREFILL_READS", 10),
		overridable(&discordReplyLimit, "SIRENS_ECHO_REPLY_LIMIT", 1990),
		overridable(&mcpsReplyBudget, "SIRENS_ECHO_MCPS_REPLY_BUDGET", 1800),
		overridable(&mcpToolSummaryRunes, "SIRENS_ECHO_MCP_TOOL_SUMMARY_RUNES", 90),
		overridable(&pollQuestionRunes, "SIRENS_ECHO_POLL_QUESTION_RUNES", 300),
		overridable(&pollAnswerRunes, "SIRENS_ECHO_POLL_ANSWER_RUNES", 55),
		overridable(&pollDefaultDuration, "SIRENS_ECHO_POLL_DEFAULT_DURATION", 24),
		overridable(&pollMaxDuration, "SIRENS_ECHO_POLL_MAX_DURATION", 768),
		overridable(&threadNameRunes, "SIRENS_ECHO_THREAD_NAME_RUNES", 100),
		overridable(&threadTitleRunes, "SIRENS_ECHO_THREAD_TITLE_RUNES", 60),
		overridable(&threadTitleWords, "SIRENS_ECHO_THREAD_TITLE_WORDS", 9),
		overridable(&threadArchiveMinutes, "SIRENS_ECHO_THREAD_ARCHIVE_MINUTES", 60),

		overridable(&maxProgressRows, "SIRENS_ECHO_PROGRESS_ROWS", 12),
		overridable(&maxCalculatorRunes, "SIRENS_ECHO_CALCULATOR_RUNES", 200),
		overridable(&maxCalculatorDigits, "SIRENS_ECHO_CALCULATOR_DIGITS", 30),
		overridable(&maxCalculatorExponent, "SIRENS_ECHO_CALCULATOR_EXPONENT", 64),
		overridable(&maxCalculatorPlaces, "SIRENS_ECHO_CALCULATOR_PLACES", 10),
		overridable(&maxProxyToolNameBytes, "SIRENS_ECHO_PROXY_TOOL_NAME_BYTES", 64),
		overridable(&maxRedactedBlocks, "SIRENS_ECHO_REDACTED_BLOCKS", 2),
		overridable(&inventedChannelRunes, "SIRENS_ECHO_INVENTED_CHANNEL_RUNES", 64),
		overridable(&maxObjectEmoji, "SIRENS_ECHO_OBJECT_EMOJI", 3),
		overridable(&maxBlockReasonWords, "SIRENS_ECHO_BLOCK_REASON_WORDS", 20),
		overridable(&mentionNameRunes, "SIRENS_ECHO_MENTION_NAME_RUNES", 3),

		overridable(&defaultRateLimiterCapacity, "SIRENS_ECHO_RATE_LIMITER_CAPACITY", 4096),
		overridable(&maxHTTPBody, "SIRENS_ECHO_HTTP_BODY_BYTES", 64<<10),
		overridable(&maxRepoInventoryEntries, "SIRENS_ECHO_REPO_INVENTORY_ENTRIES", 100),
		overridable(&maxRepoFileBytes, "SIRENS_ECHO_REPO_FILE_BYTES", 64*1024),
		overridable(&maxTrackerSearchRows, "SIRENS_ECHO_TRACKER_SEARCH_ROWS", 50),

		overridable(&coalesceCapacity, "SIRENS_ECHO_COALESCE_CAPACITY", 200),
		overridable(&coalesceWindow, "SIRENS_ECHO_COALESCE_WINDOW", 25*time.Second),
		overridable(&coalesceBatch, "SIRENS_ECHO_COALESCE_BATCH", 4),
		overridable(&coalesceWideWindow, "SIRENS_ECHO_COALESCE_WIDE_WINDOW", 45*time.Second),
		overridable(&coalesceWideBatch, "SIRENS_ECHO_COALESCE_WIDE_BATCH", 8),
		overridable(&coalesceAgeCap, "SIRENS_ECHO_COALESCE_AGE_CAP", 90*time.Second),
		overridable(&coalesceWorkers, "SIRENS_ECHO_COALESCE_WORKERS", 3),

		overridable(&DefaultBoardEpochs, "SIRENS_ECHO_BOARD_EPOCHS", 5),
		overridable(&DefaultVerbatimWords, "SIRENS_ECHO_VERBATIM_WORDS", 8),
		overridable(&defaultEvaluationCaseTimeout, "SIRENS_ECHO_EVALUATION_CASE_TIMEOUT", 5*time.Minute),
	}
}

// deriveKnobs recomputes what is defined as an expression of another number.
// A derived value is set by setting its input, never on its own.
func deriveKnobs() {
	turnProgressEvery = turnProgressAfter * 2
	turnLongReplyAfter = turnProgressAfter + turnProgressEvery*2
	replyAttachmentBytes = maxScratchFileBytes

	// The turn budget is the parent of every wait inside a turn, so one number
	// moves the whole shape and no child can outlive its turn. sirens-echo#942.

	// Half, not the sixth one slot was sized against: a pool waits for the
	// first slot to free. See docs/sirens-echo-admission.md.
	defaultQueueTimeout = defaultRequestTimeout / 2
	defaultCallTimeout = defaultRequestTimeout / 4
	modelIdleTimeout = defaultRequestTimeout / 4

	// The ceiling IS the escalation run to its end, so the two cannot disagree
	// the way a separately-set ceiling could.
	maxCompletionTokens = baseCompletionTokens
	for range budgetRaisesAllowed {
		maxCompletionTokens *= completionBudgetStep
	}

	// The bound covers the pool and the queue behind it, a turn counting from
	// acceptance to release. See docs/sirens-echo-admission.md.
	defaultRateLimitPolicy.MaxPending = executionSlots * 2

	// The longest a turn can still be calling tools, plus the window that ends
	// a trajectory. An hour was inherited. See sirens-echo#930.
	trajectoryLifetime = defaultRequestTimeout + trajectoryIdle

	// One context-injection budget. A grounding document and a tool result are
	// spent against the same window, so they move together.
	maxGroundingBytes = maxToolResultBytes
	maxServerGuidanceBytes = maxToolResultBytes / 4

	// Widening starts exactly where the window stopped keeping up, which is the
	// pool holding a narrow batch each rather than a number of its own.
	coalesceHighWater = coalesceWorkers * coalesceBatch

	// A skillpack is a scratch file, and a partition holds sixteen of them.
	maxSkillpackBytes = maxScratchFileBytes
	maxScratchPartitionBytes = maxScratchFileBytes * 16
}

// applyKnobs resets every number to its default and then applies what the
// environment set, so a second call is not cumulative with the first.
func applyKnobs(lookup func(string) string) (applied, rejected []string) {
	applied, rejected = make([]string, 0), make([]string, 0)
	table := knobs()
	for _, entry := range table {
		entry.reset()
	}
	for _, entry := range table {
		raw := strings.TrimSpace(lookup(entry.env))
		if raw == "" {
			continue
		}
		// A bad value keeps the default for every knob alike, and is named
		// rather than swallowed. See docs/sirens-echo-tuning.md.
		if !entry.apply(raw) {
			rejected = append(rejected, entry.env)
			continue
		}
		applied = append(applied, entry.env)
	}
	deriveKnobs()
	sort.Strings(applied)
	sort.Strings(rejected)
	return applied, rejected
}

// The defaults are in place before any other package variable reads one, and
// before a binary that never calls LoadConfig runs.
func init() { applyKnobs(func(string) string { return "" }) }

// KnobReferenceHeading opens the generated reference, so the writer and the
// staleness check agree on where the table starts.
const KnobReferenceHeading = "Every number a deployment may set."

// RenderKnobReference renders the table as the tracked reference. Generated
// from the table itself, so the list cannot be maintained in two places.
func RenderKnobReference() string {
	pairs := make([]string, 0, len(knobs()))
	for _, entry := range knobs() {
		pairs = append(pairs, strings.TrimPrefix(entry.env, "SIRENS_ECHO_")+"="+entry.fallback)
	}
	sort.Strings(pairs)
	// Two pairs per line: the one-per-row table outgrew the documentation band,
	// and the fix for a generated file over the cap is the generator.
	rows := make([]string, 0, (len(pairs)+1)/2)
	for i := 0; i < len(pairs); i += 2 {
		row := fmt.Sprintf("%-46s", pairs[i])
		if i+1 < len(pairs) {
			row += pairs[i+1]
		}
		rows = append(rows, strings.TrimRight(row, " "))
	}
	return KnobReferenceHeading + "\n" +
		"Generated by `just knobs`, every name prefixed SIRENS_ECHO_. Each is read once at\n" +
		"startup, and an unparsable value keeps the default and is named in the startup log.\n" +
		"Why the numbers live where they do: docs/sirens-echo-tuning.md\n\n" +
		strings.Join(rows, "\n") + "\n"
}

const (
	defaultDefinitionPath = "agents/echo/definition.yaml"
	defaultHTTPListenAddr = "127.0.0.1:8080"
	// The fallback is a live service, so it is only safe for the definition that
	// service actually runs. See resolveInstanceName and sirens-echo#542.
	defaultInstanceName = "sirens-echo"
	// The identity that fallback belongs to, so the guard compares the thing
	// itself rather than a filename that can be spelled anything. See #706.
	defaultInstanceIdentity = "Sirens Echo"
	defaultBundleDir        = "/app/agent/bundles"

	ResponseStyleNeutral = "neutral"
	ResponseStyleSocial  = "social"
)

// Admission defaults are sized for a guild the operator does not moderate.
// See docs/sirens-echo-admission.md.

var defaultRateLimitPolicy = RateLimitPolicy{
	PerUser:    RateLimit{Burst: 3, Every: 30 * time.Second},
	PerContext: RateLimit{Burst: 10, Every: 10 * time.Second},
	Global:     RateLimit{Burst: 20, Every: 5 * time.Second},
	// Derived from executionSlots in deriveKnobs, never set here.
	NotifyEvery: 5 * time.Minute,
}

// DefaultAgentProxyURL is a neutral fallback. Deployment owns the real
// endpoint and sets AGENT_PROXY_URL.
const DefaultAgentProxyURL = "http://agent-proxy:8080"

// DefaultOTLPEndpoint is the existing in-cluster SigNoZ collector.
const DefaultOTLPEndpoint = "http://signoz-otel-collector.observability.svc.cluster.local:4318"

var (
	mcpServerNamePattern   = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	environmentNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	// RFC 7230 token characters, because the vendor picks the header name and
	// x-api-key is as common as a capitalised one.
	headerNamePattern    = regexp.MustCompile(`^[A-Za-z0-9!#$%&'*+.^_` + "`" + `|~-]+$`)
	discordSnowflake     = regexp.MustCompile(`^[0-9]{15,20}$`)
	discordHandlePattern = regexp.MustCompile(`^[a-z0-9._]{2,32}$`)
	composedRolePattern  = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	// channelLabelPattern matches the grounding validator's channel form, so a
	// label cannot introduce a reference the model is rejected for repeating.
	channelLabelPattern = regexp.MustCompile(`^#[A-Za-z_][A-Za-z0-9_-]*$`)
	// rateLimitPattern is "<burst>/<refill interval for one token>".
	rateLimitPattern = regexp.MustCompile(`^([0-9]+)/(.+)$`)
)

// MCP client transports. Deployment picks the servers and their transports.
// Echo validates the shape of an entry, never which server is acceptable.
const (
	MCPTransportStreamable = "streamable"
	MCPTransportSSE        = "sse"
	MCPTransportStdio      = "stdio"
)

// MCPServerDefinition is one resolved roster entry. Deployment owns the roster,
// so this is built by the loader rather than written in a source definition.
type MCPServerDefinition struct {
	Name      string
	Transport string
	URL       string
	Command   string
	Args      []string
	Env       map[string]string
	Headers   map[string]string
}

// ResolvedTransport defaults to streamable, so an entry written before
// transports were selectable keeps working unchanged.
func (s MCPServerDefinition) ResolvedTransport() string {
	if trimmed := strings.TrimSpace(s.Transport); trimmed != "" {
		return trimmed
	}
	return MCPTransportStreamable
}

// Definition is the source-controlled attribution, route, and policy selection.
type Definition struct {
	Schema             string   `json:"schema" yaml:"schema"`
	Identity           string   `json:"identity" yaml:"identity"`
	SelfAliases        []string `json:"self_aliases,omitempty" yaml:"self_aliases,omitempty"`
	AuditRole          string   `json:"audit_role" yaml:"audit_role"`
	ResponseStyle      string   `json:"response_style" yaml:"response_style"`
	Channel            string   `json:"channel" yaml:"channel"`
	MaxContextMessages int      `json:"max_context_messages" yaml:"max_context_messages"`
	LocalSkillRoots    []string `json:"local_skill_roots" yaml:"local_skill_roots"`
	IssueTracker       string   `json:"issue_tracker,omitempty" yaml:"issue_tracker,omitempty"`
	// Composed requires a materialized agent-compose bundle, so a profile that
	// asks for an identity fails startup rather than answering without one.
	Composed bool `json:"composed,omitempty" yaml:"composed,omitempty"`
	// ModelBudget is empty for a definition that takes the packaged ceilings.
	ModelBudget ModelBudget `json:"model_budget,omitempty" yaml:"model_budget,omitempty"`
}

// ModelBudget is what one turn on this definition may spend on model calls. The
// two profiles do not share a substrate, so they cannot share one ceiling.
type ModelBudget struct {
	ToolRounds int `json:"tool_rounds,omitempty" yaml:"tool_rounds,omitempty"`
	// TurnModelCalls is the whole turn's allowance, shared by the content gate,
	// the answer, and the filing check. ToolRounds bounds each of those alone.
	TurnModelCalls       int `json:"turn_model_calls,omitempty" yaml:"turn_model_calls,omitempty"`
	BaseCompletionTokens int `json:"base_completion_tokens,omitempty" yaml:"base_completion_tokens,omitempty"`
	MaxCompletionTokens  int `json:"max_completion_tokens,omitempty" yaml:"max_completion_tokens,omitempty"`
	BudgetRaises         int `json:"budget_raises,omitempty" yaml:"budget_raises,omitempty"`
	ToolResultBytes      int `json:"tool_result_bytes,omitempty" yaml:"tool_result_bytes,omitempty"`

	// Keyed by the model-facing tool name. One ceiling across every tool is
	// more suspect than its value, so a tool may name its own. See #635.
	ToolResultBytesByTool map[string]int `json:"tool_result_bytes_by_tool,omitempty" yaml:"tool_result_bytes_by_tool,omitempty"`
}

// ToolResultBytesFor resolves the bound for one tool, falling back to the
// budget-wide ceiling so an unnamed tool keeps exactly today's behaviour.
func (b ModelBudget) ToolResultBytesFor(tool string) int {
	if bound, ok := b.ToolResultBytesByTool[tool]; ok {
		return bound
	}
	return b.ToolResultBytes
}

// resolved fills each unset field from the packaged default, so a definition
// names only what it changes and an unset budget is today's behaviour exactly.
func (b ModelBudget) resolved() ModelBudget {
	if b.ToolRounds == 0 {
		b.ToolRounds = maxToolRounds
	}
	if b.TurnModelCalls == 0 {
		b.TurnModelCalls = turnModelCalls
	}
	if b.BaseCompletionTokens == 0 {
		b.BaseCompletionTokens = baseCompletionTokens
	}
	if b.MaxCompletionTokens == 0 {
		b.MaxCompletionTokens = maxCompletionTokens
	}
	if b.BudgetRaises == 0 {
		b.BudgetRaises = budgetRaisesAllowed
	}
	if b.ToolResultBytes == 0 {
		b.ToolResultBytes = maxToolResultBytes
	}
	return b
}

// validate refuses a budget that would spend without bound or contradict
// itself. Every field is a ceiling, so none of them may be negative.
func (b ModelBudget) validate() error {
	for _, field := range []struct {
		name  string
		value int
	}{
		{"tool_rounds", b.ToolRounds},
		{"turn_model_calls", b.TurnModelCalls},
		{"base_completion_tokens", b.BaseCompletionTokens},
		{"max_completion_tokens", b.MaxCompletionTokens},
		{"budget_raises", b.BudgetRaises},
		{"tool_result_bytes", b.ToolResultBytes},
	} {
		if field.value < 0 {
			return fmt.Errorf("model_budget %s must not be negative, got %d",
				field.name, field.value)
		}
	}
	// An absent key inherits. A present one is deliberate, so zero here means
	// deliver nothing rather than unset, and that is never what anyone meant.
	for tool, bound := range b.ToolResultBytesByTool {
		if bound <= 0 {
			return fmt.Errorf(
				"model_budget tool_result_bytes_by_tool[%q] must be positive, got %d: "+
					"remove the entry to inherit tool_result_bytes", tool, bound)
		}
	}
	resolved := b.resolved()
	// A tool round costs two calls, so an allowance below two is a toolless
	// lane spelled as a number, and never what the value meant.
	if resolved.TurnModelCalls < 2 {
		return fmt.Errorf(
			"model_budget turn_model_calls %d funds no tool round, so the lane "+
				"would answer without tools on every turn",
			resolved.TurnModelCalls,
		)
	}
	if resolved.MaxCompletionTokens < resolved.BaseCompletionTokens {
		return fmt.Errorf(
			"model_budget max_completion_tokens %d is below base_completion_tokens %d",
			resolved.MaxCompletionTokens, resolved.BaseCompletionTokens,
		)
	}
	// A ceiling the rungs stop short of never applies. See sirens-echo#522.
	if reached := resolved.ladderTop(); reached < resolved.MaxCompletionTokens {
		return fmt.Errorf(
			"model_budget max_completion_tokens %d is unreachable: %d raises from %d "+
				"tops out at %d",
			resolved.MaxCompletionTokens, resolved.BudgetRaises,
			resolved.BaseCompletionTokens, reached,
		)
	}
	return nil
}

// ladderTop is where the raises stop climbing. It walks nextCompletionBudget
// rather than repeating it, which also clamps to the ceiling as the proxy does.
func (b ModelBudget) ladderTop() int {
	reached := b.BaseCompletionTokens
	for raise := 0; raise < b.BudgetRaises; raise++ {
		next, canRaise := nextCompletionBudget(reached, b.MaxCompletionTokens)
		if !canRaise {
			break
		}
		reached = next
	}
	return reached
}

// Principal identifies the one speaker the prompt trusts. The values are
// deployment-owned. See docs/sirens-echo-prompt.md.
type Principal struct {
	Handle string
	UserID string
}

// Configured reports whether deployment supplied both signals. One alone
// identifies nobody, so the prompt renders neither.
func (p Principal) Configured() bool { return p.Handle != "" && p.UserID != "" }

// PlaceholderPrincipal renders the tracked snapshot and the build-time policy
// check. It is not a real account, matching docs/access-policy.reference.yaml.
var PlaceholderPrincipal = Principal{Handle: "example_handle", UserID: "1024000000000000001"}

// Config combines the source-controlled definition with deployment secrets.
type Config struct {
	Definition     Definition
	DefinitionPath string
	InstanceName   string
	// LogWriter receives structured logs. Empty means stdout, which is what the
	// service wants. A runner emitting a dataset on stdout selects stderr.
	LogWriter io.Writer
	// Principal is empty until deployment names the trusted account.
	Principal Principal
	// BundlePath is the materialized bundle for the deployment's role, empty
	// when the definition composes nothing.
	BundlePath     string
	DiscordEnabled bool
	DiscordToken   string
	// DiscordChannelIDs are the channels that may summon this deployment, plus
	// their threads. Channel IDs are globally unique, so the list spans guilds.
	DiscordChannelIDs []string
	// DiscordGuildIDs optionally restricts which guilds may summon at all.
	// Empty means every guild the bot joined, still bounded by the channels.
	DiscordGuildIDs []string
	// DiscordDMEnabled admits direct messages. Off by default, because a
	// direct message has no guild moderation behind it.
	DiscordDMEnabled bool
	AgentProxyURL    string
	AgentProxyModel  string
	OTLPEndpoint     string
	HTTPListenAddr   string
	// MCPRosterPath names the deployment-owned mcpServers file. Empty is a
	// valid no-tool boundary.
	MCPRosterPath string
	// ContentClassesPath names the taxonomy the content gate enforces. Empty
	// runs no gate at all, so the deployment is the switch.
	ContentClassesPath string
	// JevModel pins the systemone model route.jev calls. Empty skips the
	// stage entirely: no network call, every family falls back to today's turn.
	JevModel string
	// JevDisable names the families route.jev must not ask about, each
	// falling back to today's behaviour independently of the others.
	JevDisable []string
	// HTTPTrustToken authenticates a caller on the tailnet. Empty trusts
	// nobody. See docs/sirens-echo-http.md.
	HTTPTrustToken string
	// MCPReexport offers the roster's tools over /mcp beside turn. False serves
	// turn alone, because re-export moves a security boundary. sirens-echo#1025.
	MCPReexport bool
	// FetchHosts is the allowlist the fetch tool may reach. Empty offers no
	// tool. See docs/sirens-echo-tools.md.
	FetchHosts []string
	// TrackerIssuesTable and TrackerCommentsTable are both written by a
	// filing: an issue is a header plus a comment thread.
	TrackerIssuesTable   string
	TrackerCommentsTable string
	// TrackerOpenIssuesView carries the filter a search cannot send. Empty
	// reads the table's default view.
	TrackerOpenIssuesView string
	// TrackerOrg and TrackerRepo replace the move-to-repo label. See #756.
	TrackerOrg  string
	TrackerRepo string
	// TrackerPriority and TrackerRoles fill the tracker's remaining notNull
	// fields. Empty leaves the tracker's own default in place.
	TrackerPriority string
	TrackerRoles    []string
	// AccessPolicyPath names the deployment's tracked allowlist file. Empty
	// synthesizes the equivalent from the Discord environment variables.
	AccessPolicyPath string
	// DiscordCommandsEnabled registers and serves application commands. Off by
	// default because registering is a write to Discord's API.
	DiscordCommandsEnabled bool
	// CoalesceEnabled batches a member's rapid comments onto a worker pool in
	// place of the execution slot. See docs/sirens-echo-admission.md.
	CoalesceEnabled bool
	// JobWorkspaceRoot enables executing jobs. Empty means no execution at all,
	// which is the default posture.
	JobWorkspaceRoot string
	// JobRepository and JobVerb fix what an executing job does, both from
	// closed sets in this repository.
	JobRepository string
	JobVerb       string
	// JobStoreDir is the durable job store's directory. Empty keeps jobs in
	// memory, which loses them on restart. See docs/sirens-echo-jobs.md.
	JobStoreDir string
	// JobStoreDSN points the store at Postgres instead of a directory. It is a
	// credential, so it is never logged. See docs/sirens-echo-jobs.md.
	JobStoreDSN string
	// RepoInventoryURL and RepoInventoryOrg name the forge and organization the
	// inventory lists. Either empty offers no tool.
	RepoInventoryURL string
	// The read is unauthenticated, so it sees public repositories only.
	RepoInventoryOrg string
	// ScratchDir mounts the per-requester scratchpad. Empty offers no scratchpad
	// tools at all. See docs/sirens-echo-scratchpad.md.
	ScratchDir string
	// PhrasesPath names the canonical phrase registry. Empty renders nothing,
	// which is today's behaviour. See docs/sirens-echo-phrases.md.
	PhrasesPath string
	// TuningApplied and TuningRejected record what the knob pass did, so a
	// typed name is visible at startup rather than silently ignored.
	TuningApplied  []string
	TuningRejected []string
	RequestTimeout time.Duration
	QueueTimeout   time.Duration
	// ThreadSummary ends a thread turn by editing this service's first reply
	// there. Off is the shipped default. See docs/sirens-echo-threads.md.
	ThreadSummary bool
	// ExecutionSlots is how many turns run at once. See sirens-echo#995.
	ExecutionSlots int
	// ShutdownGrace is how long a restart waits for the turns already running.
	// It has to fit the pod's kill window. See docs/sirens-echo-execution.md.
	ShutdownGrace time.Duration
	RateLimit     RateLimitPolicy
}

// resolveInstanceName refuses to hand a non-Echo definition Echo's service name.
// Defaulting there merges another profile's spans into Echo's. See #542.
func resolveInstanceName(identity, configured string) (string, error) {
	if name := strings.TrimSpace(configured); name != "" {
		return name, nil
	}
	// The definition's own identity, never its path or filename: both are
	// spellings a foreign definition can also be given. See #706.
	if identity != defaultInstanceIdentity {
		return "", fmt.Errorf(
			"SIRENS_ECHO_INSTANCE is required when the definition is %q rather "+
				"than %q: defaulting to %q would report this profile as Echo",
			identity, defaultInstanceIdentity, defaultInstanceName,
		)
	}
	return defaultInstanceName, nil
}

// LoadConfig loads the Sirens Echo deployment from environment and its
// source-controlled definition. Secrets never have source defaults.
func LoadConfig() (Config, error) {
	// Applied before anything reads a tuning number, so a derived value is
	// never computed from a default the deployment replaced.
	tuningApplied, tuningRejected := applyKnobs(os.Getenv)
	definitionPath := valueOrDefault(os.Getenv("SIRENS_ECHO_DEFINITION"), defaultDefinitionPath)
	definition, err := LoadDefinition(definitionPath)
	if err != nil {
		return Config{}, err
	}
	instanceName, err := resolveInstanceName(definition.Identity, os.Getenv("SIRENS_ECHO_INSTANCE"))
	if err != nil {
		return Config{}, err
	}
	// Read from the knob pass rather than parsed a second time. Two readers of
	// one name disagreed about what a bad value does. See sirens-echo#829.
	rateLimit, err := loadRateLimitPolicy()
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Definition:     definition,
		DefinitionPath: definitionPath,
		InstanceName:   instanceName,
		Principal: Principal{
			Handle: strings.TrimSpace(os.Getenv("SIRENS_ECHO_PRINCIPAL_HANDLE")),
			UserID: strings.TrimSpace(os.Getenv("SIRENS_ECHO_PRINCIPAL_USER_ID")),
		},
		DiscordToken:         strings.TrimSpace(os.Getenv("DISCORD_TOKEN")),
		DiscordChannelIDs:    splitList(os.Getenv("DISCORD_CHANNEL_ID")),
		DiscordGuildIDs:      splitList(os.Getenv("DISCORD_GUILD_IDS")),
		AgentProxyURL:        valueOrDefault(os.Getenv("AGENT_PROXY_URL"), DefaultAgentProxyURL),
		AgentProxyModel:      strings.TrimSpace(os.Getenv("AGENT_PROXY_MODEL")),
		OTLPEndpoint:         valueOrDefault(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"), DefaultOTLPEndpoint),
		HTTPListenAddr:       valueOrDefault(os.Getenv("SIRENS_ECHO_HTTP_ADDR"), defaultHTTPListenAddr),
		MCPRosterPath:        strings.TrimSpace(os.Getenv("SIRENS_ECHO_MCP_ROSTER")),
		AccessPolicyPath:     strings.TrimSpace(os.Getenv("SIRENS_ECHO_ACCESS_POLICY")),
		ContentClassesPath:   strings.TrimSpace(os.Getenv("SIRENS_ECHO_CONTENT_CLASSES")),
		JevModel:             strings.TrimSpace(os.Getenv("SIRENS_ECHO_JEV_MODEL")),
		JevDisable:           splitList(os.Getenv("SIRENS_ECHO_JEV_DISABLE")),
		HTTPTrustToken:       strings.TrimSpace(os.Getenv("SIRENS_ECHO_HTTP_TOKEN")),
		FetchHosts:           fetchHosts(os.Getenv("SIRENS_ECHO_FETCH_HOSTS")),
		TrackerIssuesTable:   strings.TrimSpace(os.Getenv("SIRENS_ECHO_TRACKER_ISSUES_TABLE")),
		TrackerCommentsTable: strings.TrimSpace(os.Getenv("SIRENS_ECHO_TRACKER_COMMENTS_TABLE")),
		TrackerOpenIssuesView: strings.TrimSpace(
			os.Getenv("SIRENS_ECHO_TRACKER_OPEN_VIEW")),
		TrackerOrg:       strings.TrimSpace(os.Getenv("SIRENS_ECHO_TRACKER_ORG")),
		TrackerRepo:      strings.TrimSpace(os.Getenv("SIRENS_ECHO_TRACKER_REPO")),
		TrackerPriority:  strings.TrimSpace(os.Getenv("SIRENS_ECHO_TRACKER_PRIORITY")),
		TrackerRoles:     commaValues(os.Getenv("SIRENS_ECHO_TRACKER_ROLES")),
		JobStoreDir:      strings.TrimSpace(os.Getenv("SIRENS_ECHO_JOB_STORE")),
		JobStoreDSN:      strings.TrimSpace(os.Getenv("SIRENS_ECHO_JOB_STORE_DSN")),
		RepoInventoryURL: strings.TrimSpace(os.Getenv("SIRENS_ECHO_REPO_INVENTORY_URL")),
		RepoInventoryOrg: strings.TrimSpace(os.Getenv("SIRENS_ECHO_REPO_INVENTORY_ORG")),
		ScratchDir:       strings.TrimSpace(os.Getenv("SIRENS_ECHO_SCRATCH")),
		PhrasesPath:      strings.TrimSpace(os.Getenv("SIRENS_ECHO_PHRASES")),
		TuningApplied:    tuningApplied,
		TuningRejected:   tuningRejected,
		RequestTimeout:   defaultRequestTimeout,
		QueueTimeout:     defaultQueueTimeout,
		ExecutionSlots:   executionSlots,
		ShutdownGrace:    defaultShutdownGrace,
		RateLimit:        rateLimit,
	}
	// One pass over the flag table, so a switch cannot be read at a call site
	// the reference does not know about. See internal/community/featureflags.go.
	if err := applyFeatureFlags(&cfg, os.Getenv); err != nil {
		return Config{}, err
	}
	if !mcpServerNamePattern.MatchString(cfg.InstanceName) {
		return Config{}, fmt.Errorf("SIRENS_ECHO_INSTANCE must be a lowercase service name")
	}
	if err := validatePrincipal(cfg.Principal); err != nil {
		return Config{}, err
	}
	if cfg.Definition.Composed {
		path, err := resolveBundlePath()
		if err != nil {
			return Config{}, err
		}
		cfg.BundlePath = path
	}
	snowflakes := append([]string{}, cfg.DiscordChannelIDs...)
	snowflakes = append(snowflakes, cfg.DiscordGuildIDs...)
	for _, id := range snowflakes {
		if !discordSnowflake.MatchString(id) {
			return Config{}, fmt.Errorf("Discord IDs must be numeric snowflakes, got %q", id)
		}
	}
	missing := make([]string, 0, 5)
	if cfg.DiscordEnabled {
		if cfg.DiscordToken == "" {
			missing = append(missing, "DISCORD_TOKEN")
		}
		// The access policy file supplies scope on its own. Otherwise a
		// channel list is required unless direct messages are the only ingress.
		if cfg.AccessPolicyPath == "" &&
			len(cfg.DiscordChannelIDs) == 0 && !cfg.DiscordDMEnabled {
			missing = append(missing, "DISCORD_CHANNEL_ID")
		}
	}
	if cfg.AgentProxyModel == "" {
		missing = append(missing, "AGENT_PROXY_MODEL")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required env: %v", missing)
	}
	return cfg, nil
}

// validatePrincipal rejects a half-configured principal, which would otherwise
// render a sentence naming one signal and an empty string for the other.
func validatePrincipal(principal Principal) error {
	if principal.Handle == "" && principal.UserID == "" {
		return nil
	}
	if !principal.Configured() {
		return fmt.Errorf(
			"SIRENS_ECHO_PRINCIPAL_HANDLE and SIRENS_ECHO_PRINCIPAL_USER_ID must be set together",
		)
	}
	if !discordHandlePattern.MatchString(principal.Handle) {
		return fmt.Errorf(
			"SIRENS_ECHO_PRINCIPAL_HANDLE must be a Discord username, got %q",
			principal.Handle,
		)
	}
	if !discordSnowflake.MatchString(principal.UserID) {
		return fmt.Errorf(
			"SIRENS_ECHO_PRINCIPAL_USER_ID must be a numeric snowflake, got %q",
			principal.UserID,
		)
	}
	return nil
}

// resolveBundlePath selects the baked bundle for the deployment's role, with no
// default now that two lanes compose. See docs/sirens-echo-config.md.
func resolveBundlePath() (string, error) {
	role := strings.TrimSpace(os.Getenv("SIRENS_ECHO_ROLE"))
	if role == "" {
		return "", fmt.Errorf("a composing definition requires SIRENS_ECHO_ROLE to name a role slug")
	}
	if !composedRolePattern.MatchString(role) {
		return "", fmt.Errorf("SIRENS_ECHO_ROLE must be a lowercase role slug, got %q", role)
	}
	dir := valueOrDefault(strings.TrimSpace(os.Getenv("SIRENS_ECHO_BUNDLE_DIR")), defaultBundleDir)
	path := filepath.Join(dir, role)
	if _, err := os.Stat(filepath.Join(path, "manifest.json")); err != nil {
		return "", fmt.Errorf("no composed bundle for role %q under %s: %w", role, dir, err)
	}
	return path, nil
}

// LoadDefinition reads and validates the repository-owned agent definition.
func LoadDefinition(path string) (Definition, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, fmt.Errorf("read agent definition: %w", err)
	}
	var definition Definition
	if err := yaml.Unmarshal(raw, &definition); err != nil {
		return Definition{}, fmt.Errorf("parse agent definition: %w", err)
	}
	if definition.Schema != "coilyco-harness.agent.v1" {
		return Definition{}, fmt.Errorf("unsupported agent definition schema %q", definition.Schema)
	}
	if definition.Identity == "" || definition.AuditRole == "" {
		return Definition{}, fmt.Errorf("agent definition requires identity and audit_role")
	}
	// Channel is the prompt's boundary label, not the routing key. Deployment
	// owns routing through DISCORD_CHANNEL_ID.
	if definition.Channel != "" && !channelLabelPattern.MatchString(definition.Channel) {
		return Definition{}, fmt.Errorf(
			"agent definition channel must be empty or a #channel-name, got %q",
			definition.Channel,
		)
	}
	if definition.ResponseStyle != ResponseStyleNeutral &&
		definition.ResponseStyle != ResponseStyleSocial {
		return Definition{}, fmt.Errorf("unsupported response_style %q", definition.ResponseStyle)
	}
	if definition.MaxContextMessages < 1 || definition.MaxContextMessages > 50 {
		return Definition{}, fmt.Errorf("max_context_messages must be between 1 and 50")
	}
	if len(definition.LocalSkillRoots) == 0 {
		return Definition{}, fmt.Errorf("agent definition requires at least one local skill root")
	}
	// The roster is deployment-owned, so whether this names a live server is
	// checked where both are known rather than here.
	if definition.IssueTracker != "" &&
		!mcpServerNamePattern.MatchString(definition.IssueTracker) {
		return Definition{}, fmt.Errorf("invalid issue_tracker %q", definition.IssueTracker)
	}
	if err := definition.ModelBudget.validate(); err != nil {
		return Definition{}, err
	}
	return definition, nil
}

// validateMCPServer checks that an entry carries the fields its transport needs
// and none belonging to another. It makes no judgement about which server runs.
func validateMCPServer(server MCPServerDefinition) error {
	hasURL := strings.TrimSpace(server.URL) != ""
	hasCommand := strings.TrimSpace(server.Command) != ""
	switch server.ResolvedTransport() {
	case MCPTransportStreamable, MCPTransportSSE:
		// An unset interpolation variable lands here as an empty endpoint.
		if !hasURL {
			return fmt.Errorf("MCP server %q requires a baseUrl", server.Name)
		}
		if !validHTTPURL(server.URL) {
			return fmt.Errorf("MCP server %q has invalid baseUrl", server.Name)
		}
		if hasCommand || len(server.Args) > 0 || len(server.Env) > 0 {
			return fmt.Errorf("MCP server %q takes no command, args, or env", server.Name)
		}
		for name, value := range server.Headers {
			if !headerNamePattern.MatchString(name) {
				return fmt.Errorf("MCP server %q has invalid header name %q", server.Name, name)
			}
			// An unset variable lands here empty, and sending it would fail as
			// the vendor's anonymous-call error rather than as a roster one.
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("MCP server %q has empty header %q", server.Name, name)
			}
		}
	case MCPTransportStdio:
		if !hasCommand {
			return fmt.Errorf("MCP server %q requires a command", server.Name)
		}
		if hasURL {
			return fmt.Errorf("MCP server %q takes no baseUrl", server.Name)
		}
		if len(server.Headers) > 0 {
			return fmt.Errorf("MCP server %q takes no headers", server.Name)
		}
		for name := range server.Env {
			if !environmentNamePattern.MatchString(name) {
				return fmt.Errorf("MCP server %q has invalid env name %q", server.Name, name)
			}
		}
	default:
		return fmt.Errorf("MCP server %q has unsupported transport %q", server.Name, server.Transport)
	}
	return nil
}

func validHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func valueOrDefault(value, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}

// splitList parses a comma-separated deployment list, dropping empty entries so
// a trailing comma is not a configuration error.
func splitList(value string) []string {
	items := make([]string, 0, 4)
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func durationOrDefault(value string, fallback time.Duration) (time.Duration, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, fmt.Errorf("must be a Go duration such as 90s")
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("must be greater than zero")
	}
	return parsed, nil
}

// loadRateLimitPolicy overlays deployment overrides onto the packaged
// defaults. See docs/sirens-echo-admission.md for the format.
func loadRateLimitPolicy() (RateLimitPolicy, error) {
	policy := defaultRateLimitPolicy
	tiers := []struct {
		name   string
		target *RateLimit
	}{
		{"SIRENS_ECHO_RATE_USER", &policy.PerUser},
		{"SIRENS_ECHO_RATE_CONTEXT", &policy.PerContext},
		{"SIRENS_ECHO_RATE_GLOBAL", &policy.Global},
	}
	for _, tier := range tiers {
		limit, ok, err := parseRateLimit(os.Getenv(tier.name))
		if err != nil {
			return RateLimitPolicy{}, fmt.Errorf("%s: %w", tier.name, err)
		}
		if ok {
			*tier.target = limit
		}
	}
	pending, err := intOrDefault(os.Getenv("SIRENS_ECHO_MAX_PENDING"), policy.MaxPending)
	if err != nil {
		return RateLimitPolicy{}, fmt.Errorf("SIRENS_ECHO_MAX_PENDING: %w", err)
	}
	policy.MaxPending = pending
	notify, err := durationOrDefault(os.Getenv("SIRENS_ECHO_RATE_NOTIFY_EVERY"), policy.NotifyEvery)
	if err != nil {
		return RateLimitPolicy{}, fmt.Errorf("SIRENS_ECHO_RATE_NOTIFY_EVERY: %w", err)
	}
	policy.NotifyEvery = notify
	return policy, nil
}

func parseRateLimit(value string) (RateLimit, bool, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return RateLimit{}, false, nil
	}
	if trimmed == "off" {
		return RateLimit{}, true, nil
	}
	match := rateLimitPattern.FindStringSubmatch(trimmed)
	if match == nil {
		return RateLimit{}, false, fmt.Errorf(`must be "<burst>/<interval>" such as 3/30s, or "off"`)
	}
	burst, err := strconv.Atoi(match[1])
	if err != nil || burst < 1 {
		return RateLimit{}, false, fmt.Errorf("burst must be a positive integer")
	}
	every, err := time.ParseDuration(match[2])
	if err != nil || every <= 0 {
		return RateLimit{}, false, fmt.Errorf("interval must be a positive Go duration such as 30s")
	}
	return RateLimit{Burst: burst, Every: every}, true, nil
}

func intOrDefault(value string, fallback int) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("must be a non-negative integer")
	}
	return parsed, nil
}

func boolOrDefault(value string, fallback bool) (bool, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(trimmed)
	if err != nil {
		return false, fmt.Errorf("must be true or false")
	}
	return parsed, nil
}
