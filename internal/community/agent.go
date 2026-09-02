package community

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	transportDiscord = "discord"
	transportHTTP    = "http"
	transportMCP     = "mcp"
)

// Agent owns the Sirens Echo Discord session and its outbound boundaries.
type Agent struct {
	// temporal is held only to close it. Nil when no mirror is configured.
	temporal interface{ Close() }
	// trajectoryWorker runs the workflow the mirror signals. Nil when off.
	trajectoryWorker interface{ Stop() }
	cfg              Config
	session          *discordgo.Session
	tools            *MCPProvider
	// reexport caches the roster advertised over /mcp. Zero value is ready.
	reexport          reexportCache
	completions       CompletionClient
	systemPrompt      string
	telemetry         *Telemetry
	readinessClient   *http.Client
	readinessEndpoint string
	readinessRoute    string
	readinessTimeout  time.Duration
	slots             chan struct{}
	// turns records a turn while it runs, so a roll leaves a report rather
	// than a silence. See docs/sirens-echo-execution.md.
	turns TurnLog
	// process names this run. A record carrying another run is an interruption.
	process string
	seen    *seenMessages
	scope   *channelScope
	// summaries remembers this service's first reply per thread, for the
	// living-answer shape. Nil until built. See sirens-echo#951.
	summaries *threadSummaries
	// threads caches thread ownership, so a state miss costs one REST lookup
	// per channel rather than one per message. See sirens-echo#750.
	threads *channelScope
	access  *AccessPolicy
	limiter *rateLimiter
	// lookups bounds Discord REST calls forced during gate evaluation, so an
	// unscoped channel cannot make the process call the API per message.
	lookups *rateLimiter
	// jobs is nil when the deployment enables no job kinds.
	jobs *JobRunner
	// exchanges bounds a run of agent-to-agent turns per channel.
	exchanges *exchangeLimiter
	beats     *heartbeat
	// identifiers refuses a reply carrying a value this process holds.
	identifiers *IdentifierGuard
	// taxonomy is empty when the deployment configures no content gate.
	taxonomy ContentTaxonomy
	// phrases is empty when the deployment names no registry, which renders
	// nothing and is today's behaviour.
	phrases PhraseRegistry
	// drain holds the Discord turns in flight, so a restart can wait for them
	// and then tell the rest why they stopped.
	drain drainState
	// lane batches a member's comments into one turn and drains batches across
	// a pool. Nil is the serial execution slot, which is the shipped default.
	lane *coalesceLane
}

// NewAgent builds the independently deployable Sirens Echo runtime.
func NewAgent(cfg Config, telemetry *Telemetry) (*Agent, error) {
	readinessEndpoint, err := agentProxyReadinessEndpoint(
		cfg.AgentProxyURL,
		cfg.AgentProxyModel,
	)
	if err != nil {
		return nil, err
	}
	localSkillpack, err := LoadSkillpack(cfg.Definition.LocalSkillRoots)
	if err != nil {
		return nil, err
	}
	composed := ""
	if cfg.Definition.Composed {
		composed, err = LoadBundle(cfg.BundlePath)
		if err != nil {
			return nil, err
		}
	}
	var phrases PhraseRegistry
	if cfg.PhrasesPath != "" {
		phrases, err = LoadPhraseRegistry(cfg.PhrasesPath)
		if err != nil {
			return nil, err
		}
	}
	// Both appended rather than built in, so a caller with no phrase registry
	// still renders what it rendered. See docs/sirens-echo-phrases.md.
	systemPrompt := withReactionPolicy(withPhrasePolicy(
		BuildSystemPrompt(cfg.Definition, cfg.Principal, composed, localSkillpack),
		phrases,
	))
	if err := ValidateSystemPrompt(cfg.Definition, cfg.Principal, systemPrompt); err != nil {
		return nil, err
	}
	var session *discordgo.Session
	if cfg.DiscordEnabled {
		session, err = discordgo.New("Bot " + cfg.DiscordToken)
		if err != nil {
			return nil, fmt.Errorf("discord session: %w", err)
		}
		session.Identify.Intents = discordgo.IntentsGuilds |
			discordgo.IntentsGuildMessages |
			discordgo.IntentsMessageContent
		if cfg.DiscordDMEnabled {
			session.Identify.Intents |= discordgo.IntentsDirectMessages
		}
	}
	accessPolicy, err := resolveAccessPolicy(cfg)
	if err != nil {
		return nil, err
	}
	telemetry = telemetryOrNoop(telemetry)
	httpClient := &http.Client{
		// No total timeout: it would cut a streaming completion on schedule
		// however many heartbeats arrived. See docs/sirens-echo-model-call.md.
		Transport: otelhttp.NewTransport(
			http.DefaultTransport,
			otelhttp.WithTracerProvider(telemetry.traceProvider),
			otelhttp.WithPropagators(telemetry.propagator),
		),
	}
	roster, err := loadRoster(cfg)
	if err != nil {
		return nil, err
	}
	tools := &MCPProvider{
		Servers:    roster,
		HTTPClient: sessionHTTPClient(telemetry),
		Telemetry:  telemetry,
	}
	// Issue verbs rather than the record verbs its MCP publishes, and an
	// inactive policy wraps nothing. See docs/sirens-echo-issues.md.
	tracker := &TrackerProvider{Inner: tools, Policy: trackerPolicyFor(cfg)}
	// The roster handle stays concrete because the agent closes it and serves
	// prompts through it. Only what the model sees is composed.
	var modelTools ToolProvider = tracker
	extras := []ToolProvider{tracker}
	if cfg.ScratchDir != "" {
		extras = append(extras, &ScratchProvider{Root: cfg.ScratchDir})
	}
	if len(cfg.FetchHosts) > 0 {
		extras = append(extras, &FetchProvider{Hosts: cfg.FetchHosts})
	}
	// The references the prompt no longer carries, read when the model decides
	// one is relevant. See sirens-echo#859.
	references, err := LoadSkillReferences(cfg.Definition.LocalSkillRoots)
	if err != nil {
		return nil, err
	}
	// The bundle indexes its own references in the pack above, so they reach the
	// tool too, or the prompt names paths read_skill then refuses.
	if cfg.Definition.Composed {
		bundled, bundleErr := LoadBundleReferences(cfg.BundlePath)
		if bundleErr != nil {
			return nil, bundleErr
		}
		references = append(references, bundled...)
	}
	if len(references) > 0 {
		extras = append(extras, &SkillProvider{References: references})
	}
	// Unconditional, because it needs no configuration and a tool shipped dark
	// behind an unset switch is a tool nobody has. See sirens-echo#916.
	extras = append(extras, &CalculatorProvider{})
	if cfg.RepoInventoryOrg != "" && cfg.RepoInventoryURL != "" {
		extras = append(extras, &RepoInventoryProvider{
			BaseURL: cfg.RepoInventoryURL,
			Org:     cfg.RepoInventoryOrg,
		})
	}
	if len(extras) > 1 {
		modelTools = &CompositeProvider{Providers: extras}
	}
	proxy := ProxyClient{
		BaseURL:       cfg.AgentProxyURL,
		Model:         cfg.AgentProxyModel,
		AuditRole:     cfg.Definition.AuditRole,
		Attribution:   cfg.Definition.Identity,
		ResponseStyle: cfg.Definition.ResponseStyle,
		Harness:       deploymentHarness(cfg),
		HTTPClient:    httpClient,
		Tools:         modelTools,
		Telemetry:     telemetry,
		Budget:        cfg.Definition.ModelBudget,
		Admission:     AdmissionBound(accessPolicy),
	}
	agent := &Agent{
		cfg:               cfg,
		session:           session,
		tools:             tools,
		completions:       proxy,
		systemPrompt:      systemPrompt,
		phrases:           phrases,
		telemetry:         telemetry,
		readinessClient:   newReadinessHTTPClient(defaultReadinessTimeout),
		readinessEndpoint: readinessEndpoint,
		readinessRoute:    cfg.AgentProxyModel,
		readinessTimeout:  defaultReadinessTimeout,
		seen:              newSeenMessages(1024),
		scope:             newChannelScope(256),
		access:            accessPolicy,
	}
	if cfg.ContentClassesPath != "" {
		taxonomy, err := LoadContentTaxonomy(cfg.ContentClassesPath)
		if err != nil {
			return nil, err
		}
		agent.taxonomy = taxonomy
	}
	agent.identifiers = NewIdentifierGuard(cfg, roster)
	// Offered to the repair loop only after the guard exists, because the checks
	// read it. See docs/sirens-echo-reply-assembly.md.
	proxy.ValidateReply = agent.repairableReplyChecks
	agent.completions = proxy
	// Attached after the agent exists, because the checks are model calls it
	// makes. See docs/sirens-echo-issues.md.
	tracker.FilingCheck = agent.checkMemberFiling
	agent.ensureRuntimeDefaults()
	if err := agent.attachToolMirror(); err != nil {
		return nil, err
	}
	if err := agent.buildJobRunner(); err != nil {
		return nil, err
	}
	// After the handlers would need it and before Open can deliver anything.
	if cfg.CoalesceEnabled && session != nil {
		agent.buildCoalesceLane()
	}
	if session != nil {
		session.AddHandler(agent.onReady)
		session.AddHandler(agent.onMessage)
		session.AddHandler(agent.onMessageEdit)
		if cfg.DiscordCommandsEnabled {
			session.AddHandler(agent.onInteraction)
		}
	}
	return agent, nil
}

// mcpSessionSpanName keeps a held-open session out of the request-path
// percentiles. Its lifetime is not a latency. See sirens-echo#560.
func mcpSessionSpanName(_ string, request *http.Request) string {
	return "mcp.session " + request.Method
}

// sessionHTTPClient serves the MCP transport, with no whole-request timeout
// because a held-open session has no request to bound. See sirens-echo#160.
func sessionHTTPClient(telemetry *Telemetry) *http.Client {
	return &http.Client{
		Transport: otelhttp.NewTransport(
			http.DefaultTransport,
			otelhttp.WithTracerProvider(telemetry.traceProvider),
			otelhttp.WithPropagators(telemetry.propagator),
			otelhttp.WithSpanNameFormatter(mcpSessionSpanName),
		),
	}
}

// defaultLookupPolicy bounds Discord REST calls made while evaluating gates.
// See docs/sirens-echo-admission.md.
var defaultLookupPolicy = RateLimitPolicy{
	PerContext: RateLimit{Burst: 30, Every: time.Second},
	Global:     RateLimit{Burst: 60, Every: 500 * time.Millisecond},
}

// ensureRuntimeDefaults fills fields a hand-constructed Agent leaves zero. A
// zero timeout expires on creation and a nil limiter panics.
func (a *Agent) ensureRuntimeDefaults() {
	if a.cfg.RequestTimeout <= 0 {
		a.cfg.RequestTimeout = defaultRequestTimeout
	}
	if a.cfg.QueueTimeout <= 0 {
		a.cfg.QueueTimeout = defaultQueueTimeout
	}
	if a.cfg.ShutdownGrace <= 0 {
		a.cfg.ShutdownGrace = defaultShutdownGrace
	}
	if a.slots == nil {
		if a.cfg.ExecutionSlots <= 0 {
			a.cfg.ExecutionSlots = executionSlots
		}
		a.slots = make(chan struct{}, a.cfg.ExecutionSlots)
	}
	if a.process == "" {
		a.process = newProcessID()
	}
	if a.summaries == nil {
		a.summaries = newThreadSummaries(256)
	}
	if a.turns == nil {
		a.turns = NewMemoryTurnLog()
	}
	if a.limiter == nil {
		a.limiter = newRateLimiter(a.cfg.RateLimit, defaultRateLimiterCapacity)
	}
	if a.lookups == nil {
		a.lookups = newRateLimiter(defaultLookupPolicy, 512)
	}
	if a.scope == nil {
		a.scope = newChannelScope(256)
	}
	if a.threads == nil {
		a.threads = newChannelScope(256)
	}
	if a.seen == nil {
		a.seen = newSeenMessages(1024)
	}
	if a.access == nil {
		a.access = synthesizeAccessPolicy(a.cfg)
	}
	if a.exchanges == nil {
		a.exchanges = newExchangeLimiter(nil)
	}
}

// resolveAccessPolicy prefers the deployment's tracked allowlist file and falls
// back to the equivalent policy built from the legacy environment variables.
func resolveAccessPolicy(cfg Config) (*AccessPolicy, error) {
	if cfg.AccessPolicyPath == "" {
		return synthesizeAccessPolicy(cfg), nil
	}
	return LoadAccessPolicy(cfg.AccessPolicyPath)
}

// deploymentHarness attributes model calls to the ingress this deployment
// actually has, rather than asserting Discord for every profile.
func deploymentHarness(cfg Config) string {
	if cfg.DiscordEnabled {
		return transportDiscord
	}
	return transportHTTP
}

// trackerPolicyFor assembles what the harness writes on a filed issue, none
// of which is a model decision. See docs/sirens-echo-issues.md.
func trackerPolicyFor(cfg Config) trackerPolicy {
	return trackerPolicy{
		Tracker:        cfg.Definition.IssueTracker,
		IssuesTable:    cfg.TrackerIssuesTable,
		CommentsTable:  cfg.TrackerCommentsTable,
		OpenIssuesView: cfg.TrackerOpenIssuesView,
		Org:            cfg.TrackerOrg,
		Repo:           cfg.TrackerRepo,
		Priority:       cfg.TrackerPriority,
		Roles:          cfg.TrackerRoles,
	}
}

// loadRoster reads the deployment-owned roster and checks the definition's
// issue tracker against it, the one place both are known.
func loadRoster(cfg Config) ([]MCPServerDefinition, error) {
	var roster []MCPServerDefinition
	if cfg.MCPRosterPath != "" {
		loaded, err := LoadMCPRoster(cfg.MCPRosterPath)
		if err != nil {
			return nil, err
		}
		roster = loaded
	}
	tracker := cfg.Definition.IssueTracker
	if tracker == "" {
		return roster, nil
	}
	for _, server := range roster {
		if server.Name == tracker {
			return roster, nil
		}
	}
	return nil, fmt.Errorf("issue_tracker %q names no server in the MCP roster", tracker)
}

// buildJobRunner wires durable work when the deployment asked for it. See
// docs/sirens-echo-jobs.md for which variable selects which store.
func (a *Agent) buildJobRunner() error {
	store, err := openJobStore(a.cfg)
	if err != nil {
		return err
	}
	// The turn log follows the store the deployment already chose, so there is
	// no second name for one decision.
	if a.turns, err = openTurnLog(a.cfg, store); err != nil {
		return err
	}
	executors, err := buildExecutingKinds(a.cfg, a.access, a.telemetry)
	if err != nil {
		return err
	}
	reporter := newDiscordJobReporter(a.session)
	a.jobs = &JobRunner{
		Store:           store,
		Telemetry:       a.telemetry,
		Executors:       executors,
		Notifier:        reporter,
		Progress:        reporter,
		Content:         reporter,
		ValidateContent: a.validateJobContent,
		Grants:          jobGrants(a.access),
	}
	return nil
}

// jobGrants returns the deployment's grant table, or nil when it declares none.
// Nil grants everything, which is only correct before a table is adopted.
func jobGrants(policy *AccessPolicy) *GrantTable {
	if policy == nil || len(policy.Grants.Principals) == 0 {
		return nil
	}
	table := policy.Grants
	return &table
}

// recoverJobs settles whatever a restart left live, so no record sits live
// forever after a crash and no requester is left waiting on one.
func (a *Agent) recoverJobs(ctx context.Context) {
	memory, ok := a.jobs.Store.(interface{ All() []Job })
	if !ok {
		return
	}
	all := memory.All()
	a.settleRestart(ctx, "stranded", StrandedJobIDs(all),
		"interrupted by a restart", RecoverStrandedJobs)
	// Queued work is dropped rather than stranded: nothing requeues it, so
	// leaving the record accurate leaves it pending forever. See sirens-echo#878.
	a.settleRestart(ctx, "dropped", DroppedJobIDs(all),
		"dropped by a restart", SettleDroppedJobs)
}

// settleRestart settles one group and tells each requester, because a Discord
// requester never reads the record. See docs/sirens-echo-jobs.md.
func (a *Agent) settleRestart(
	ctx context.Context,
	group string,
	ids []string,
	outcome string,
	settle func(JobStore, []string, string) ([]Job, error),
) {
	if len(ids) == 0 {
		return
	}
	settled, err := settle(a.jobs.Store, ids, outcome)
	if err != nil {
		a.telemetry.Error(ctx, "job.recovery.failed",
			slog.String("group", group), slog.Int("jobs", len(ids)))
		return
	}
	for _, job := range settled {
		a.jobs.notify(ctx, job)
	}
	a.telemetry.Info(ctx, "job.recovery.settled",
		slog.String("group", group), slog.Int("jobs", len(settled)))
}

// reportInterruptedTurns says what the last run did not finish. It runs at boot
// because nothing reliable runs at the death. See docs/sirens-echo-execution.md.
func (a *Agent) reportInterruptedTurns(ctx context.Context) {
	if a.turns == nil {
		return
	}
	swept, err := a.turns.Sweep(a.process)
	if err != nil {
		a.telemetry.Error(ctx, "turn.sweep.failed")
		return
	}
	if len(swept) == 0 {
		return
	}
	for _, record := range swept {
		// The same counter an ordinary turn lands on, so an interrupted turn is
		// a turn outcome rather than a separate thing to go looking for.
		a.telemetry.RecordTurn(ctx, turnOutcomeInterrupted, 0)
		a.telemetry.Info(ctx, "turn.interrupted",
			slog.String("message_id", record.MessageID),
			slog.String("channel_id", record.ChannelID),
			slog.String("process", record.Process),
			slog.Time("started_at", record.StartedAt),
		)
		a.tellChannelItWasInterrupted(ctx, record)
	}
	a.telemetry.Info(ctx, "turn.sweep.reported", slog.Int("turns", len(swept)))
}

// tellChannelItWasInterrupted stops the room waiting on a reply that is never
// coming. It never re-answers, which would risk the double answer.
func (a *Agent) tellChannelItWasInterrupted(ctx context.Context, record TurnRecord) {
	if a.session == nil || record.ChannelID == "" {
		return
	}
	_, err := a.session.ChannelMessageSendComplex(record.ChannelID, &discordgo.MessageSend{
		Content: noticeInterrupted,
		Reference: &discordgo.MessageReference{
			MessageID: record.MessageID,
			ChannelID: record.ChannelID,
		},
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse:       []discordgo.AllowedMentionType{},
			RepliedUser: false,
		},
	})
	if err != nil {
		a.telemetry.RecordFailure(ctx, "turn.interrupted.notice")
	}
}

// Run opens the Gateway session and blocks until shutdown.
func (a *Agent) Run(ctx context.Context) error {
	a.logCapabilities(ctx)
	if a.jobs != nil {
		if err := a.jobs.Start(ctx); err != nil {
			return err
		}
		defer a.jobs.Stop()
		defer a.closeToolMirror()
		a.recoverJobs(ctx)
	}
	a.reportInterruptedTurns(ctx)
	if a.session != nil {
		// Started before the gateway opens, so no summon arrives at a pool that
		// is not yet draining batches.
		if a.lane != nil {
			a.lane.start(a.drain.root())
		}
		if err := a.session.Open(); err != nil {
			return fmt.Errorf("Discord open: %w", err)
		}
		defer a.session.Close()
		// A positive signal, so a quiet guild and a stopped gateway stop
		// producing the same telemetry. See docs/sirens-echo-observability.md.
		a.beats = &heartbeat{}
		defer a.watchGateway(ctx)()
	}
	if a.tools != nil {
		// Supervised MCP connections outlive every turn, so shutdown is the only
		// thing that closes them and stops any stdio child.
		defer func() { _ = a.tools.Close() }()
	}
	// A retention policy that is configured and never fires is no policy, so
	// the sweeper starts with the service. See sirens-echo#156.
	defer a.sweepSessions(ctx)()
	httpServer := &http.Server{
		Addr:              a.cfg.HTTPListenAddr,
		Handler:           a.HTTPHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	httpErr := make(chan error, 1)
	go func() {
		httpErr <- httpServer.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		return a.drainTurns(ctx, httpServer)
	case err := <-httpErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP serve: %w", err)
	}
}

func (a *Agent) onReady(_ *discordgo.Session, ready *discordgo.Ready) {
	a.telemetry.Info(
		context.Background(),
		"discord.ready",
		slog.String("identity", a.cfg.Definition.Identity),
		slog.String("discord_user", ready.User.Username),
		slog.String("channel", a.cfg.Definition.Channel),
		slog.String("audit_role", a.cfg.Definition.AuditRole),
		// The count, never the values. See docs/sirens-echo-attributes.md.
		slog.Int("guarded_identifiers", a.identifiers.Guarded()),
		// The admission allowlist, sized at boot. It outlived the thread-prefill
		// toggle it was added for, which no longer has a default to observe.
		slog.Int("configured_channels", len(a.cfg.DiscordChannelIDs)),
		// Empty when the build carried no revision, which is the honest answer.
		slog.String("build_revision", BuildRevision()),
	)
	// Ready is the first point the application id exists, and registration is
	// what makes a declared command reachable. See sirens-echo#127.
	a.registerCommandsOnReady(context.Background(), ready)
}

// summonContext is the origin of one summon, and its key is the admission
// identity shared by everything from that guild or direct-message channel.
type summonContext struct {
	Kind      string
	GuildID   string
	ChannelID string
}

const (
	contextKindGuild = "guild"
	contextKindDM    = "dm"
)

// Key identifies the context for admission control. One guild shares one key,
// so it cannot consume every other guild's budget.
func (c summonContext) Key() string {
	if c.Kind == contextKindDM {
		return contextKindDM + ":" + c.ChannelID
	}
	return contextKindGuild + ":" + c.GuildID
}

func (a *Agent) onMessage(session *discordgo.Session, event *discordgo.MessageCreate) {
	a.admitMessage(session, event.Message)
}

// onMessageEdit answers a message that became a summon only after it was
// posted. The duplicate gate keeps an already-answered message answered once.
func (a *Agent) onMessageEdit(session *discordgo.Session, event *discordgo.MessageUpdate) {
	if !editSummons(session, event) {
		return
	}
	a.admitMessage(session, event.Message)
}

// editSummons decides whether an update is a member edit that newly names this
// service. See docs/sirens-echo-mentions.md for why each gate is here.
func editSummons(session *discordgo.Session, event *discordgo.MessageUpdate) bool {
	if event == nil || event.Message == nil {
		return false
	}
	message := event.Message
	// Discord emits this event when it resolves a link preview too, and only a
	// member edit sets the timestamp. An unfurl must never summon.
	if message.EditedTimestamp == nil {
		return false
	}
	// An edit arrives partial, so a missing guild id cannot be read as a direct
	// message. Guild-only keeps a partial payload off the direct message policy.
	if message.GuildID == "" {
		return false
	}
	// A reply reference is not re-derived from a partial payload, so an edit
	// summons on an explicit mention alone. That is what was asked for.
	return mentionsBot(session, message)
}

func (a *Agent) admitMessage(session *discordgo.Session, message *discordgo.Message) {
	// Counted before eligibility, since ingress stopping and every message
	// being ineligible are different failures.
	a.beats.observe()
	if !eligibleMessage(session, message, a.access) {
		// A misconfigured allowlist and a quiet channel looked identical, since
		// this path recorded nothing at all. See docs/sirens-echo-compose.md.
		if counterpartOf(message) == CounterpartAgent {
			a.telemetry.Info(
				context.Background(),
				"discord.agent.ignored",
				slog.String("reason", string(accessDeniedAgent)),
				slog.String("counterpart", string(CounterpartAgent)),
			)
		}
		return
	}
	// Two agents that each answer the other is a runaway, so the exchange is
	// bounded before anything else spends budget on it.
	if !a.exchanges.admit(message.ChannelID, counterpartOf(message)) {
		a.telemetry.RecordAccess(context.Background(), string(accessDeniedExchange))
		// A counter says the bound fired and never how often in a row, which is
		// the shape a runaway has.
		a.telemetry.Info(
			context.Background(),
			"discord.exchange.bounded",
			slog.String("reason", string(accessDeniedExchange)),
			slog.String("counterpart", string(CounterpartAgent)),
		)
		return
	}
	a.beats.admit()
	origin := summonContextFor(message)
	// The whole allowlist decides from the payload already in memory, so a
	// guild, member, or channel outside it costs nothing.
	decision := a.access.Evaluate(origin, message.Author.ID, memberRoles(message), nil)
	if !decision.allowed() && decision.Reason != accessNeedsThreadRef {
		a.telemetry.RecordAccess(context.Background(), string(decision.Reason))
		return
	}
	reason, referenceLookup := summonedLocally(session, message)
	if !reason.summoned() {
		// summonedLocally reads cached state alone. An archived thread, and
		// after a restart any thread GUILD_CREATE did not carry, misses it.
		if a.resolveThreadOwnership(session, origin) {
			reason, referenceLookup = summonOwnedThread, false
		}
	}
	if !reason.summoned() && !referenceLookup {
		a.telemetry.RecordSummon(context.Background(), string(reason), origin.Kind)
		return
	}
	threadLookup := false
	if decision.Reason == accessNeedsThreadRef {
		cached, known := a.scope.Get(origin.ChannelID)
		if known && !cached {
			a.telemetry.RecordAccess(context.Background(), string(accessDeniedChannel))
			return
		}
		threadLookup = !known
	}
	if threadLookup || (!reason.summoned() && referenceLookup) {
		if a.lookups.Admit(admissionRequest{ContextKey: origin.Key()}).Outcome.denied() {
			a.telemetry.RecordAdmission(context.Background(), string(admissionContext), "lookup")
			return
		}
	}
	if threadLookup && !a.resolveScope(session, origin, decision.Guild) {
		a.telemetry.RecordAccess(context.Background(), string(accessDeniedChannel))
		return
	}
	if !reason.summoned() {
		reason = summonedByReference(session, message)
		if !reason.summoned() {
			a.telemetry.RecordSummon(context.Background(), string(reason), origin.Kind)
			return
		}
	}
	if !a.seen.Add(message.ID) {
		return
	}
	// Counted after the duplicate gate, so a redelivery is one summon rather
	// than two. See docs/sirens-echo-admission.md.
	a.telemetry.RecordSummon(context.Background(), string(reason), origin.Kind)
	// Last, because a summon refused for a restart was otherwise admissible and
	// the count should say so. See docs/sirens-echo-execution.md.
	if !a.drain.enter() {
		a.telemetry.RecordAccess(context.Background(), string(accessDeniedDraining))
		a.onDraining(session, message)
		return
	}
	a.telemetry.RecordAccess(context.Background(), string(accessAllowed))
	go func() {
		defer a.drain.leave()
		a.handleMessage(session, message, origin, decision.Guild.Overrides())
	}()
}

// onDraining turns away a summon that arrived during a restart. It marks and
// says nothing, because the gateway it would reply through is closing.
func (a *Agent) onDraining(session *discordgo.Session, message *discordgo.Message) {
	ctx := context.Background()
	a.telemetry.Info(
		ctx,
		"turn.input.draining",
		slog.String("transport", transportDiscord),
		slog.String("outcome", string(accessDeniedDraining)),
	)
	a.react(ctx, &discordMessageTurn{session: session, message: message}, reactionRefused)
}

// eligibleMessage rejects payloads that can never be a summon. A bot author
// passes only when named. See docs/sirens-echo-compose.md.
func eligibleMessage(
	session *discordgo.Session,
	message *discordgo.Message,
	policy *AccessPolicy,
) bool {
	if message == nil || message.Author == nil || message.ChannelID == "" {
		return false
	}
	if session.State == nil || session.State.User == nil {
		return false
	}
	if message.Author.ID == session.State.User.ID {
		return false
	}
	if message.Author.Bot && !policy.PermitsAgent(message.Author.ID) {
		return false
	}
	return true
}

// counterpartOf reads what Discord asserted about the author. Ground truth,
// never a guess from writing style.
func counterpartOf(message *discordgo.Message) CounterpartKind {
	if message != nil && message.Author != nil && message.Author.Bot {
		return CounterpartAgent
	}
	return CounterpartHuman
}

// attachmentSources returns what the runtime may fetch. The URL is Gateway
// supplied rather than message text, and the host is bounded anyway.
func attachmentSources(message *discordgo.Message) []AttachmentSource {
	if message == nil || len(message.Attachments) == 0 {
		return nil
	}
	sources := make([]AttachmentSource, 0, len(message.Attachments))
	for _, attachment := range message.Attachments {
		if attachment == nil {
			continue
		}
		sources = append(sources, AttachmentSource{
			URL:      attachment.URL,
			Declared: attachment.ContentType,
		})
	}
	return sources
}

// attachmentTypes returns each attachment's media type. Bytes and filenames
// stay out, so nothing member-authored enters the transcript by this route.
func attachmentTypes(message *discordgo.Message) []string {
	if message == nil || len(message.Attachments) == 0 {
		return nil
	}
	kinds := make([]string, 0, len(message.Attachments))
	for _, attachment := range message.Attachments {
		if attachment == nil {
			continue
		}
		kind := attachment.ContentType
		if strings.TrimSpace(kind) == "" {
			kind = "application/octet-stream"
		}
		kinds = append(kinds, kind)
	}
	return kinds
}

// memberRoles returns the author's guild roles, which arrive on the Gateway
// payload. A role grant therefore costs no Discord API call.
func memberRoles(message *discordgo.Message) []string {
	if message.Member == nil {
		return nil
	}
	return message.Member.Roles
}

func summonContextFor(message *discordgo.Message) summonContext {
	if message.GuildID == "" {
		return summonContext{Kind: contextKindDM, ChannelID: message.ChannelID}
	}
	return summonContext{
		Kind:      contextKindGuild,
		GuildID:   message.GuildID,
		ChannelID: message.ChannelID,
	}
}

// resolveScope answers an unknown channel by looking it up and caches both
// outcomes, so a busy unscoped channel does not repeat the lookup per message.
func (a *Agent) resolveScope(
	session *discordgo.Session,
	origin summonContext,
	guild *GuildAccess,
) bool {
	channel := resolveChannel(session, origin.ChannelID)
	if channel == nil {
		// Not cached: the failure may be transient, and caching would deafen a
		// real channel until restart.
		return false
	}
	allowed := channel.IsThread() && guild.PermitsChannel(channel.ParentID)
	a.scope.Set(origin.ChannelID, allowed)
	return allowed
}

// resolveThreadOwnership answers ownership when cached state cannot, at one
// REST lookup per channel. See docs/sirens-echo-mentions.md.
func (a *Agent) resolveThreadOwnership(
	session *discordgo.Session,
	origin summonContext,
) bool {
	if session.State == nil || session.State.User == nil || origin.ChannelID == "" {
		return false
	}
	if owned, known := a.threads.Get(origin.ChannelID); known {
		return owned
	}
	botID := session.State.User.ID
	if channel, err := session.State.Channel(origin.ChannelID); err == nil && channel != nil {
		owned := channel.IsThread() && channel.OwnerID == botID
		a.threads.Set(origin.ChannelID, owned)
		return owned
	}
	// Shares the budget the other gate-forced REST calls draw on.
	if a.lookups.Admit(admissionRequest{ContextKey: origin.Key()}).Outcome.denied() {
		a.telemetry.RecordAdmission(context.Background(), string(admissionContext), "lookup")
		return false
	}
	channel, err := session.Channel(origin.ChannelID)
	if err != nil || channel == nil {
		return false
	}
	owned := channel.IsThread() && channel.OwnerID == botID
	a.threads.Set(origin.ChannelID, owned)
	return owned
}

func resolveChannel(session *discordgo.Session, channelID string) *discordgo.Channel {
	if session.State != nil {
		if channel, err := session.State.Channel(channelID); err == nil && channel != nil {
			return channel
		}
	}
	channel, err := session.Channel(channelID)
	if err != nil {
		return nil
	}
	return channel
}

// summonedLocally decides summoning from the Gateway payload, and reports
// whether an unresolved reply reference could still make it a summon.
func summonedLocally(
	session *discordgo.Session,
	message *discordgo.Message,
) (reason summonReason, referenceLookup bool) {
	// A direct message is addressed to this service by definition. See
	// docs/sirens-echo-threads.md for what that costs.
	if message.GuildID == "" {
		return summonDirect, false
	}
	if mentionsBot(session, message) {
		return summonMentioned, false
	}
	botID := session.State.User.ID
	// A thread this service opened is its own conversation, so every message in
	// it is addressed here. See docs/sirens-echo-mentions.md and sirens-echo#750.
	if threadOwnedBy(session, message.ChannelID, botID) {
		return summonOwnedThread, false
	}
	if message.ReferencedMessage != nil && message.ReferencedMessage.Author != nil {
		if message.ReferencedMessage.Author.ID == botID {
			return summonRepliedTo, false
		}
		return summonReplyToAnother, false
	}
	if message.MessageReference == nil || message.MessageReference.MessageID == "" {
		return notAddressedIn(session, message.ChannelID), false
	}
	return notAddressedIn(session, message.ChannelID), true
}

// notAddressedIn separates a thread from an ordinary channel, because a member
// reporting silence in a thread is reporting sirens-echo#750 and nothing else.
func notAddressedIn(session *discordgo.Session, channelID string) summonReason {
	if session.State == nil {
		return summonNotAddressed
	}
	// Cached state only. A miss reports the plain refusal rather than spending
	// a REST call on a message nobody is waiting for.
	channel, err := session.State.Channel(channelID)
	if err == nil && channel != nil && channel.IsThread() {
		return summonNotAddressedThread
	}
	return summonNotAddressed
}

// threadOwnedBy reports whether a channel is a thread the given account
// created. State only, so it costs no call. See docs/sirens-echo-mentions.md.
func threadOwnedBy(session *discordgo.Session, channelID, ownerID string) bool {
	if session.State == nil {
		return false
	}
	channel, err := session.State.Channel(channelID)
	if err != nil || channel == nil {
		return false
	}
	return channel.IsThread() && channel.OwnerID == ownerID
}

// mentionsBot reads an explicit mention off the Gateway payload, which is the
// one summon signal a partial edit payload still carries intact.
func mentionsBot(session *discordgo.Session, message *discordgo.Message) bool {
	if session.State == nil || session.State.User == nil {
		return false
	}
	for _, mention := range message.Mentions {
		if mention != nil && mention.ID == session.State.User.ID {
			return true
		}
	}
	return mentionsBotRole(session, message)
}

// mentionsBotRole summons on a mention of a role this account holds, because a
// member who @s the role is addressing it. See docs/sirens-echo-mentions.md.
func mentionsBotRole(session *discordgo.Session, message *discordgo.Message) bool {
	if len(message.MentionRoles) == 0 || message.GuildID == "" {
		return false
	}
	held := botRoles(session, message.GuildID)
	if len(held) == 0 {
		return false
	}
	for _, mentioned := range message.MentionRoles {
		// The everyone role's id is the guild's, and every member holds it. An
		// announcement is not addressed to this service.
		if mentioned == message.GuildID {
			continue
		}
		if _, holds := held[mentioned]; holds {
			return true
		}
	}
	return false
}

// botRoles reads this account's roles in one guild. Discord delivers its own
// member on GuildCreate without the members intent, and a miss self-heals.
func botRoles(session *discordgo.Session, guildID string) map[string]struct{} {
	botID := session.State.User.ID
	member, err := session.State.Member(guildID, botID)
	if err != nil || member == nil {
		member, err = session.GuildMember(guildID, botID)
		if err != nil || member == nil {
			return nil
		}
		// Written back so one lookup covers every later message in this guild.
		_ = session.State.MemberAdd(member)
	}
	held := make(map[string]struct{}, len(member.Roles))
	for _, role := range member.Roles {
		held[role] = struct{}{}
	}
	return held
}

func summonedByReference(session *discordgo.Session, message *discordgo.Message) summonReason {
	referenced, err := session.ChannelMessage(
		message.ChannelID,
		message.MessageReference.MessageID,
	)
	if err != nil || referenced == nil || referenced.Author == nil {
		return summonReferenceUnknown
	}
	if referenced.Author.ID == session.State.User.ID {
		return summonRepliedTo
	}
	return summonReplyToAnother
}

// resolveReplyTo fetches the message a reply answers when the Gateway did not
// deliver it inline, which is likeliest for the old ones.
func (a *Agent) resolveReplyTo(
	session *discordgo.Session,
	message *discordgo.Message,
	origin summonContext,
) *discordgo.Message {
	// Delivered inline for most replies, and a fetch for those would be a REST
	// call per message for nothing.
	if message.ReferencedMessage != nil {
		return nil
	}
	if message.MessageReference == nil || message.MessageReference.MessageID == "" {
		return nil
	}
	// Shares the budget the other gate-forced REST calls draw on, so a channel
	// of old replies cannot make this one lookup per message.
	if a.lookups.Admit(admissionRequest{ContextKey: origin.Key()}).Outcome.denied() {
		a.telemetry.RecordAdmission(context.Background(), string(admissionContext), "lookup")
		return nil
	}
	referenced, err := session.ChannelMessage(
		message.ChannelID,
		message.MessageReference.MessageID,
	)
	// A reference that cannot be read is an ordinary message, not a failure.
	if err != nil {
		return nil
	}
	return referenced
}

func (a *Agent) handleMessage(
	session *discordgo.Session,
	message *discordgo.Message,
	origin summonContext,
	override *RateLimitOverride,
) {
	// A panic in one turn must not take down a deployment serving other
	// guilds, so the handler contains its own failure.
	defer func() {
		if recovered := recover(); recovered != nil {
			a.telemetry.RecordFailure(context.Background(), "panic")
			a.telemetry.Error(
				context.Background(),
				"discord.turn.panicked",
				slog.String("error_type", "turn_panicked"),
			)
			// A crashed turn told the member nothing, which reads as being
			// ignored. See docs/sirens-echo-delivery.md.
			_ = a.notifyFailure(
				context.Background(),
				&discordMessageTurn{session: session, message: message},
				noticeTurnCrashed,
			)
		}
	}()

	decision := a.limiter.Admit(admissionRequest{
		UserKey:    message.Author.ID,
		ContextKey: origin.Key(),
		// The pending cap bounds the wait for the execution slot. The lane has
		// no slot to wait for, and its own bounded queue sheds in place of it.
		Queued:   a.lane == nil,
		Override: override,
	})
	if decision.Outcome.denied() {
		a.onDenied(session, message, origin, decision)
		return
	}
	if a.lane == nil {
		defer a.limiter.Release()
	}
	a.telemetry.RecordAdmission(context.Background(), string(admissionAccepted), transportDiscord)

	receiveCtx, receiveSpan := a.telemetry.StartSpan(
		// The drain root rather than Background, so a restart can reach the turn.
		a.drain.root(),
		"discord.receive",
		discordMessageSpanAttributes(
			"process",
			discordLocationFor(session, message),
			message.ID,
		)...,
	)
	defer receiveSpan.End()
	at := discordLocationFor(session, message)
	turn := &discordMessageTurn{
		session:   session,
		message:   message,
		limit:     a.cfg.Definition.MaxContextMessages,
		titler:    a.completions,
		replyTo:   a.resolveReplyTo(session, message, origin),
		telemetry: a.telemetry,
		// Every thread, because a thread is the conversation rather than a
		// window into one. See docs/sirens-echo-threads.md.
		wholeThread: at.ThreadID != "",
		// Offered unconditionally. summaryTarget gates on the flag, so a reply
		// registers itself only where the feature can use it.
		summaries: a.summaries,
	}
	if a.lane != nil {
		a.submitSummon(receiveCtx, turn, message)
		return
	}
	if err := a.runSerialized(receiveCtx, turn); err != nil {
		a.telemetry.MarkSpanError(receiveSpan, exceptionTurnFailed)
		a.telemetry.Error(receiveCtx, "discord.turn.failed", append(
			[]slog.Attr{slog.String("error_type", "turn_failed")},
			turnFailureAttrs(err)...,
		)...)
	}
}

// onDenied records a refused summon and tells the member at most once per
// notify window, so a flooder gains no amplifier.
func (a *Agent) onDenied(
	session *discordgo.Session,
	message *discordgo.Message,
	origin summonContext,
	decision admissionDecision,
) {
	ctx := context.Background()
	a.telemetry.RecordAdmission(ctx, string(decision.Outcome), transportDiscord)
	a.telemetry.Info(
		ctx,
		"turn.input.denied",
		slog.String("transport", transportDiscord),
		slog.String("outcome", string(decision.Outcome)),
		slog.String("context_kind", origin.Kind),
		slog.Bool("notified", decision.Notify),
	)
	turn := &discordMessageTurn{session: session, message: message}
	// A refusal is marked whether or not it also carries a notice, so a silent
	// boundary is still visible to the member.
	a.react(ctx, turn, reactionRefused)
	if !decision.Notify {
		return
	}
	if err := turn.Reply(ctx, noticeWithTrace(ctx, cooldownNotice(decision.RetryAfter))); err != nil {
		a.telemetry.RecordFailure(ctx, "reply")
	}
}

// runSerialized waits for an execution slot, then runs the turn. The request
// budget starts after admission, not on arrival.
func (a *Agent) runSerialized(ctx context.Context, turn turnIO) error {
	queueCtx, cancelQueue := context.WithTimeout(ctx, a.cfg.QueueTimeout)
	defer cancelQueue()
	select {
	case a.slots <- struct{}{}:
	case <-queueCtx.Done():
		a.telemetry.RecordAdmission(ctx, string(admissionSlotWait), turn.Transport())
		a.replyQueueTimeout(ctx, turn)
		return fmt.Errorf("turn waited longer than %s for an execution slot", a.cfg.QueueTimeout)
	}
	defer func() { <-a.slots }()
	return a.runAdmitted(ctx, turn)
}

// runAdmitted runs one turn under the request budget. It is what an execution
// slot guards, and what a coalescing worker runs in place of taking one.
func (a *Agent) runAdmitted(ctx context.Context, turn turnIO) error {
	turnCtx, cancel := context.WithTimeout(ctx, a.cfg.RequestTimeout)
	defer cancel()
	// Both transports reach here, so the mirror keys on one id rather than on
	// whichever the caller happened to have. See docs/sirens-echo-tool-markup.md.
	turnCtx = ContextWithRequestID(turnCtx, turn.RequestID())
	// Typing starts when the turn runs. Started at queue time it would expire
	// before the reply.
	if notifier, ok := turn.(typingNotifier); ok {
		stopTyping := startTyping(turnCtx, notifier)
		defer stopTyping()
	}
	progress := a.progressFor(turn)
	defer progress.Finish(context.WithoutCancel(turnCtx))
	// Written before the first model call and cleared after the last, so what
	// survives is exactly what the process did not finish.
	defer a.finishTurnRecord(turnCtx, turn)
	a.beginTurnRecord(turnCtx, turn)
	return a.runTurn(turnCtx, turn, progress)
}

// interruptible is an optional turn capability. A transport whose summon can
// still be named after the process dies declares it.
type interruptible interface {
	InterruptRecord() TurnRecord
}

// beginTurnRecord marks the turn in progress. A failed write is reported and
// never fails the turn, the record being a report rather than a gate.
func (a *Agent) beginTurnRecord(ctx context.Context, turn turnIO) {
	target, ok := turn.(interruptible)
	if !ok || a.turns == nil {
		return
	}
	record := target.InterruptRecord()
	record.Process = a.process
	record.StartedAt = time.Now()
	if err := a.turns.Begin(record); err != nil {
		a.telemetry.RecordFailure(ctx, "turn.record")
	}
}

// finishTurnRecord clears the record on success and on handled failure alike,
// because both of those are the process reaching the end of the turn.
func (a *Agent) finishTurnRecord(ctx context.Context, turn turnIO) {
	target, ok := turn.(interruptible)
	if !ok || a.turns == nil {
		return
	}
	// Detached, so a cancelled turn still clears the record it would otherwise
	// leave for the next boot to report as interrupted.
	if err := a.turns.Finish(target.InterruptRecord().MessageID); err != nil {
		a.telemetry.RecordFailure(context.WithoutCancel(ctx), "turn.record")
	}
}

// replyQueueTimeout tells the caller its turn gave up waiting. Returning
// silently left a queued member with no reply at all.
func (a *Agent) replyQueueTimeout(ctx context.Context, turn turnIO) {
	// Marked before the throttle, the way a denial is, so a member who gets no
	// notice still gets something. See docs/sirens-echo-progress.md.
	if target, ok := turn.(reactor); ok {
		a.react(ctx, target, reactionFailed)
	}
	// NOT THROTTLED, unlike a denial. Admission already said yes to this turn,
	// so silence here reads as being ignored rather than as being refused.
	if err := turn.Reply(ctx, noticeWithTrace(ctx, noticeQueueTimeout)); err != nil {
		a.telemetry.RecordFailure(ctx, "reply")
	}
}

// typingNotifier is implemented by transports that can show progress.
type typingNotifier interface {
	Typing() error
}

// startTyping holds the indicator for the turn. Discord expires it after
// roughly ten seconds, so a long model call needs it refreshed.
func startTyping(ctx context.Context, notifier typingNotifier) func() {
	_ = notifier.Typing()
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(8 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = notifier.Typing()
			}
		}
	}()
	return func() { close(done) }
}

type turnIO interface {
	RequestID() string
	// Requester is the principal the turn is attributed to. Capabilities that
	// partition per account read it from the turn context.
	Requester() string
	Transport() string
	Current() TranscriptEntry
	History(ctx context.Context) ([]TranscriptEntry, error)
	Reply(ctx context.Context, content string) error
}

// replyBudget is an optional turn capability, like the reactor: a transport
// with a send ceiling declares it. See docs/sirens-echo-tool-markup.md.
type replyBudget interface {
	ReplyLimit() int
}

// replyLimitOf returns zero for a transport with no ceiling, as HTTP has none.
func replyLimitOf(turn turnIO) int {
	if bounded, ok := turn.(replyBudget); ok {
		return bounded.ReplyLimit()
	}
	return 0
}

// prefillReporter is an optional turn capability: a transport that read a whole
// thread reports what the context budget made it drop.
type prefillReporter interface {
	PrefillNote() prefillNote
}

// prefillNoteOf returns the zero note for a transport that reads no thread,
// which renders nothing.
func prefillNoteOf(turn turnIO) prefillNote {
	if reporter, ok := turn.(prefillReporter); ok {
		return reporter.PrefillNote()
	}
	return prefillNote{}
}

// spanTagger is an optional turn capability, asserted like the reactor is: a
// transport with identifiers of its own contributes them to the turn span.
type spanTagger interface {
	SpanAttributes() []attribute.KeyValue
}

// progressFor gives a Discord turn a progress line. Other transports answer
// synchronously, so there is nothing to narrate to.
func (a *Agent) progressFor(turn turnIO) *turnProgress {
	discord, ok := turn.(*discordMessageTurn)
	if !ok || discord.session == nil {
		return nil
	}
	return newReportingTurnProgress(discordTurnProgress{
		session: discord.session,
		channel: discord.message.ChannelID,
		message: discord.message,
	}, a.telemetry, nil)
}

func (a *Agent) runTurn(
	ctx context.Context,
	turn turnIO,
	progress *turnProgress,
) (turnErr error) {
	started := time.Now()
	turnCtx, turnSpan := a.telemetry.StartSpan(
		ctx,
		"community.turn",
		attribute.String("sirens_echo.transport", turn.Transport()),
		attribute.String("sirens_echo.request_id", turn.RequestID()),
		attribute.String("agent.role", a.cfg.Definition.AuditRole),
	)
	if turn.Transport() == transportDiscord {
		turnSpan.SetAttributes(attribute.String("discord.channel", a.cfg.Definition.Channel))
	}
	// So a trace id in a member's hands resolves to the message that produced
	// it. See docs/sirens-echo-turn-stages.md.
	if tagger, ok := turn.(spanTagger); ok {
		turnSpan.SetAttributes(tagger.SpanAttributes()...)
	}
	turnCtx = WithRequester(turnCtx, turn.Requester())
	turnCtx = WithSession(turnCtx, sessionOf(turn))
	// The only bound that sees a turn as one thing rather than as each of the
	// completions it makes. See docs/sirens-echo-turn-stages.md.
	turnCtx = withTurnBudget(turnCtx, a.cfg.Definition.ModelBudget.resolved().TurnModelCalls)
	// The tool loop narrates from behind the completion boundary, and the
	// watcher narrates a stage that is waiting rather than changing.
	turnCtx = WithTurnProgress(turnCtx, progress)
	defer progress.Watch(turnCtx)()
	// Recorded before the fetch exists, so the demand is measured rather than
	// assumed. See docs/sirens-echo-rate.md.
	a.recordTraceLookup(turnCtx, turn)
	// The mark lands before any model call, so a turn that dies silently is
	// still visible. See docs/sirens-echo-progress.md.
	if target, ok := turn.(reactor); ok {
		turnCtx = WithReactor(turnCtx, target)
		a.react(turnCtx, target, reactionAccepted)
	}
	if uploader, ok := turn.(attachmentBearer); ok {
		turnCtx = WithAttachments(turnCtx, uploader.Attachments())
	}
	outcome := turnOutcomeOK
	defer func() {
		if turnErr != nil {
			outcome = turnOutcomeError
			a.telemetry.MarkSpanError(turnSpan, exceptionTurnFailed)
		}
		a.telemetry.RecordTurn(turnCtx, outcome, time.Since(started))
		turnSpan.End()
	}()

	receiveCtx, receiveSpan := a.telemetry.StartSpan(turnCtx, "community.input")
	current := turn.Current()
	a.telemetry.Info(
		receiveCtx,
		"turn.input.accepted",
		slog.String("transport", turn.Transport()),
		slog.String("request_id", turn.RequestID()),
		slog.Int("input_bytes", len(current.Content)),
	)
	receiveSpan.SetAttributes(attribute.Int("input.bytes", len(current.Content)))
	receiveSpan.End()

	progress.Stage(turnCtx, stagePhraseHistory)
	historyCtx, historySpan := a.telemetry.StartSpan(turnCtx, "community.history")
	history, err := turn.History(historyCtx)
	if err != nil {
		a.telemetry.MarkSpanError(historySpan, exceptionHistoryFailed)
		historySpan.End()
		return a.failTurn(turnCtx, turn, stageHistory, err)
	}
	historySpan.SetAttributes(attribute.Int("history.count", len(history)))
	// The prefill size a long thread produces, which is what bounds the cost of
	// turning this on anywhere. See docs/sirens-echo-threads.md.
	if note := prefillNoteOf(turn); note.Read > 0 {
		historySpan.SetAttributes(
			attribute.Int("history.thread.read", note.Read),
			attribute.Int("history.thread.dropped", note.Dropped),
			attribute.Bool("history.thread.capped", note.Capped),
		)
	}
	historySpan.End()

	contextCtx, contextSpan := a.telemetry.StartSpan(turnCtx, "context.assemble")
	prompt := BuildTurnPrompt(a.systemPrompt, history, current)
	prompt.Context = withTurnLocation(prompt.Context, turn)
	a.telemetry.Info(
		contextCtx,
		"context.rendered",
		slog.Int("history_count", len(history)),
		slog.Int("system_prompt_bytes", len(prompt.System)),
		slog.Int("context_prompt_bytes", len(prompt.Context)),
		slog.Int("user_prompt_bytes", len(prompt.Message)),
	)
	contextSpan.SetAttributes(
		attribute.Int("history.count", len(history)),
		attribute.Int("prompt.system.bytes", len(prompt.System)),
		attribute.Int("prompt.context.bytes", len(prompt.Context)),
		attribute.Int("prompt.user.bytes", len(prompt.Message)),
	)
	contextSpan.End()

	verdict, gateFailure, err := a.classifyTurn(turnCtx, current, turn.RequestID())
	if err != nil {
		// A broken gate is not a denial. See docs/sirens-echo-content-gate.md.
		a.recordContentGateFailure(turnCtx, gateFailure, err)
	}
	recordContentVerdict(turnSpan, verdict)
	if verdict.Blocked {
		a.telemetry.Info(turnCtx, "content.blocked", slog.String("class", verdict.Class.ID))
		// The boundary mark exists for exactly this and fired on nothing.
		reactFromContext(turnCtx, reactionRefused)
		// The same wording every stop gets, so a block is not tellable from a
		// failure by the element. See docs/sirens-echo-worklog.md.
		stopFromContext(turnCtx)
		blocked := turn.Reply(turnCtx, BlockResponse(verdict.Class, "", a.cfg.Principal))
		a.clearTurnMarks(turnCtx)
		return blocked
	}

	progress.Stage(turnCtx, stagePhraseThinking)
	result, err := a.completions.Complete(turnCtx, prompt, turn.RequestID())
	if err != nil {
		return a.failTurn(turnCtx, turn, stageModel, err)
	}

	progress.Stage(turnCtx, stagePhraseChecking)
	validateCtx, validateSpan := a.telemetry.StartSpan(turnCtx, "response.validate")
	reply, err := ParseReply(result.Content)
	refused := replyCheckParse
	// Silence a turn did not earn is the parse failure it always was, so the
	// stage, the check name, and the notice are unchanged for it.
	if err == nil && unchosenSilence(reply, result.ToolCalls) {
		err = ErrReplySilent
	}
	if err == nil {
		reply, refused, err = a.runReplyChecks(reply, prompt, result)
	}
	redacted := 0
	if err != nil {
		// The last rung: repair could not fix the block, so the block goes and
		// the rest is delivered. See docs/sirens-echo-reply-assembly.md.
		if kept, blocks, ok := a.redactRefusedBlocks(reply, refused, prompt, result); ok {
			a.telemetry.Info(
				validateCtx,
				"response.check.redacted",
				slog.String("check", refused),
				slog.String("refused", err.Error()),
				slog.Int("blocks", blocks),
			)
			reply, redacted, err = kept, blocks, nil
		}
	}
	// A quality rule annotates rather than gates: a correct answer discarded over
	// voice is the worse outcome. See docs/sirens-echo-delivery.md.
	shipped := ""
	if err != nil && !checkGates(refused) {
		validateSpan.SetAttributes(
			attribute.String("response.check.shipped", refused),
			attribute.String("response.check.reason", err.Error()),
		)
		a.telemetry.Info(
			validateCtx,
			"response.check.shipped",
			slog.String("check", refused),
			slog.String("refused", err.Error()),
			slog.Int("reply_bytes", len(reply)),
		)
		shipped, err = refused, nil
	}
	if err != nil {
		// The rule and its sentence. The catalog owns the exception fields, so
		// the sentence stays beside them. See docs/sirens-echo-delivery.md.
		validateSpan.SetAttributes(
			attribute.String("response.check", refused),
			attribute.String("response.check.reason", err.Error()),
		)
		a.telemetry.MarkSpanError(validateSpan, exceptionResponseValidationFailed)
		a.telemetry.Info(
			validateCtx,
			"response.check.refused",
			slog.String("check", refused),
			slog.String("refused", err.Error()),
			slog.Int("reply_bytes", len(reply)),
		)
		validateSpan.End()
		return a.failTurn(turnCtx, turn, stageValidation, err)
	}
	// A redacted reply names the rule it lost a block to, and a shipped one names
	// the rule it broke. Always set, so absence needs no interpreting.
	passed := replyCheckNone
	if redacted > 0 || shipped != "" {
		passed = refused
	}
	validateSpan.SetAttributes(
		attribute.String("response.check", passed),
		attribute.Int("response.redacted.blocks", redacted),
	)
	validateSpan.End()

	// An agent that already answered through a tool declines to answer twice,
	// and nothing else can express that. See docs/sirens-echo-reply-assembly.md.
	if reply == "" {
		return a.finishSilently(turnCtx, progress, result)
	}

	// A mark is the whole answer for a turn that needs no words. See
	// docs/sirens-echo-progress.md and docs/sirens-echo-phrases.md.
	if reactInvoked(reply) {
		glyph, err := a.resolveReaction(turnCtx, reply)
		if err != nil {
			return a.failTurn(turnCtx, turn, stageValidation, err)
		}
		facts := serviceFacts{executed: result.ToolCalls, prefill: prefillNoteOf(turn)}
		target, markable := turn.(reactor)
		// A receipt outranks a mark, and a transport that cannot mark has only
		// words. Both send the glyph instead. See docs/sirens-echo-phrases.md.
		if markable && !hasServiceSuffix(facts) {
			return a.finishByReacting(turnCtx, turn, progress, target, glyph)
		}
		reply = glyph
	}

	// A canonical phrase is a deployment artifact rather than model prose, so it
	// renders after the checks. See docs/sirens-echo-phrases.md.
	if reply, err = a.renderPhrases(turnCtx, reply); err != nil {
		return a.failTurn(turnCtx, turn, stageValidation, err)
	}

	// Service-authored, so it runs after the checks rather than through them.
	// One step, one budget. See docs/sirens-echo-issues.md and sirens-echo#413.
	reply, whole := fitWithOverflow(reply, replyLimitOf(turn), serviceFacts{
		executed: result.ToolCalls,
		prefill:  prefillNoteOf(turn),
	})

	// A line that just went up should be readable before the reply replaces it.
	// See docs/sirens-echo-progress.md.
	a.settleWithSpan(turnCtx, progress.settleDelay(), progress.Settle)

	if err := a.deliverOrReport(turnCtx, turn, reply, whole); err != nil {
		return err
	}
	// The answer is the outcome, so nothing is left to describe work in flight.
	a.clearTurnMarks(turnCtx)
	a.beats.reply()
	return nil
}

// finishSilently ends a turn that chose to produce no final text. The choice is
// recorded, so chosen silence and a broken turn stay apart. See sirens-echo#895.
func (a *Agent) finishSilently(
	ctx context.Context,
	progress *turnProgress,
	result CompletionResult,
) error {
	a.telemetry.Info(
		ctx,
		"turn.reply.silent",
		slog.Int("tool_calls", len(result.ToolCalls)),
	)
	a.settleWithSpan(ctx, progress.settleDelay(), progress.Settle)
	a.clearTurnMarks(ctx)
	a.beats.reply()
	return nil
}

// deliverOrReport sends the reply and, when that fails, records the notice
// outcome separately. The turn's verdict is the send. See sirens-echo#675.
func (a *Agent) deliverOrReport(ctx context.Context, turn turnIO, reply, whole string) error {
	err := a.sendReply(ctx, turn, reply, whole)
	if err == nil {
		return nil
	}
	// Recorded, not joined. One verdict cannot say whether the member got
	// nothing or got the answer and no apology.
	_ = a.reportUndelivered(ctx, turn)
	// Marked as the send, so the turn event classifies a Discord verdict only
	// where one exists. See #292.
	return &undeliveredReply{err: err}
}

// reportUndelivered tells a member the answer existed and did not arrive. One
// attempt, never a retry. See docs/sirens-echo-delivery.md.
func (a *Agent) reportUndelivered(ctx context.Context, turn turnIO) error {
	if target, ok := turn.(reactor); ok {
		a.react(ctx, target, reactionFailed)
	}
	// A short notice survives the length refusal that is the likeliest cause,
	// and costs one call when it does not.
	noticeErr := a.notifyFailure(ctx, turn, noticeUndelivered)
	a.telemetry.Error(
		ctx,
		"turn.reply.undelivered",
		append(
			[]slog.Attr{
				slog.String("error_type", "reply_undelivered"),
				slog.String("transport", turn.Transport()),
			},
			discordFailureAttrs(noticeErr)...,
		)...,
	)
	return noticeErr
}

func (a *Agent) failTurn(
	ctx context.Context,
	turn turnIO,
	stage string,
	cause error,
) error {
	// A drained turn's stage error is context.Canceled, which is what every
	// other cancellation also looks like. Only the cause separates them.
	if errors.Is(context.Cause(ctx), errShuttingDown) {
		cause = errShuttingDown
	}
	notice := turnFailureNotice(stage, cause)
	// Resolved rather than deleted, because an element that merely vanishes is
	// the #137 silence wearing a costume. See docs/sirens-echo-worklog.md.
	stopFromContext(ctx)
	if target, ok := turn.(reactor); ok {
		a.react(ctx, target, reactionFailed)
	}
	a.telemetry.RecordFailure(ctx, stage)
	a.telemetry.Error(
		ctx,
		"turn.stage.failed",
		slog.String("stage", stage),
		slog.String("error_type", stage+"_failed"),
		slog.String("failure_cause", failureCause(cause)),
		slog.String("notice", notice),
	)
	a.settleWithSpan(ctx, settleDelayFromContext(ctx), settleFromContext)
	noticeErr := a.notifyFailure(ctx, turn, notice)
	// The send lost, so the line already in the channel becomes the answer.
	// Deleting it leaves less than the acknowledgement. See sirens-echo#624.
	if noticeErr != nil {
		carryFromContext(context.WithoutCancel(ctx), notice)
	}
	failure := errors.Join(cause, noticeErr)
	// The outcome mark stays. The in-flight ones have stopped being true.
	a.clearTurnMarks(ctx)
	return failure
}

// notifyFailure sends a notice on a context detached from the turn deadline. A
// turn that failed by expiring has no budget left to say so otherwise.
func (a *Agent) notifyFailure(ctx context.Context, turn turnIO, notice string) error {
	noticeCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		failureNoticeTimeout,
	)
	defer cancel()
	return a.sendReply(
		withoutThreading(noticeCtx), turn, noticeWithTrace(ctx, notice), nothingWithheld,
	)
}

// settleWithSpan names the deliberate hold before a reply lands. Without it the
// wait reads as unexplained latency. See sirens-echo#652.
func (a *Agent) settleWithSpan(ctx context.Context, hold time.Duration, settle func(context.Context)) {
	if hold <= 0 {
		settle(ctx)
		return
	}
	settleCtx, span := a.telemetry.StartSpan(
		ctx,
		"turn.progress.settle",
		attribute.Int64("turn.progress.hold_ms", hold.Milliseconds()),
		attribute.Int64("turn.progress.beat_ms", turnProgressEvery.Milliseconds()),
	)
	settle(settleCtx)
	span.End()
}

// withoutThreading drops the turn's progress so a notice cannot take the
// threading path. A notice is known bytes and needs no title. See #619.
func withoutThreading(ctx context.Context) context.Context {
	return context.WithValue(ctx, turnProgressKey{}, (*turnProgress)(nil))
}

// sendReply delivers one message. whole is the complete reply when the
// transport's budget cut it. See docs/sirens-echo-reply-assembly.md.
func (a *Agent) sendReply(ctx context.Context, turn turnIO, content, whole string) error {
	replyCtx, replySpan := a.telemetry.StartSpan(ctx, "community.reply")
	a.telemetry.Info(
		replyCtx,
		"turn.reply.ready",
		slog.String("transport", turn.Transport()),
		slog.Int("reply_bytes", len(content)),
		// Zero rather than absent, so no attachment and an old pod stay apart.
		slog.Int("attached_bytes", len(whole)),
	)
	// Every reply funnels through here, so this is where "nothing to say" stops
	// being a message. See docs/sirens-echo-reply-assembly.md and #1035.
	if strings.TrimSpace(content) == "" {
		err := a.finishBlankReply(replyCtx, turn)
		replySpan.End()
		return err
	}
	var err error
	if turn.Transport() == transportDiscord {
		discordCtx, discordSpan := a.telemetry.StartSpan(replyCtx, "discord.reply")
		// A thread carrying a living answer revises it rather than adding to a
		// run of replies. See docs/sirens-echo-threads.md.
		if target := a.summaryTarget(turn); target != "" {
			err = a.reviseSummary(discordCtx, turn, target, content)
		} else {
			err = deliverWithOverflow(discordCtx, turn, content, whole)
		}
		if err != nil {
			a.telemetry.MarkSpanError(discordSpan, exceptionDiscordReplyFailed)
			// The reply was composed and paid for, so why it did not land is the
			// whole diagnosis. See docs/sirens-echo-delivery.md.
			a.telemetry.Error(discordCtx, "discord.reply.failed", append(
				[]slog.Attr{slog.Int("reply_bytes", len(content))},
				discordFailureAttrs(err)...,
			)...)
		}
		discordSpan.End()
	} else {
		err = turn.Reply(replyCtx, content)
	}
	if err != nil {
		a.telemetry.RecordFailure(replyCtx, "reply")
		a.telemetry.MarkSpanError(replySpan, exceptionReplyFailed)
		replySpan.End()
		return err
	}
	// Delivery is recorded rather than inferred from the absence of an error,
	// so a reply that arrived can be counted. See sirens-echo#652.
	a.telemetry.Info(
		replyCtx,
		"turn.reply.delivered",
		slog.String("transport", turn.Transport()),
		slog.Int("reply_bytes", len(content)),
	)
	replySpan.End()
	return nil
}

type discordMessageTurn struct {
	session *discordgo.Session
	message *discordgo.Message
	limit   int
	// mentions is built from this turn's transcript, so only people already in
	// the conversation can be reached. See docs/sirens-echo-mentions.md.
	mentions mentionRoster
	// titler names a thread. Nil keeps the derived name.
	titler CompletionClient
	// replyTo is a reference the Gateway did not deliver inline, resolved before
	// the turn ran. See docs/sirens-echo-prompt.md.
	replyTo *discordgo.Message
	// telemetry records a thread title that had to be trimmed, so a generator
	// that keeps overrunning is visible. See sirens-echo#753.
	telemetry *Telemetry
	// wholeThread opts this turn into reading its whole thread rather than the
	// partial window. Off is the shipped default. See sirens-echo#769.
	wholeThread bool
	prefill     prefillNote
	// folded are the member's earlier comments this turn also answers, oldest
	// first. Empty unless the lane built it. See docs/sirens-echo-admission.md.
	folded []*discordgo.Message
	// marker substitutes the session half a reaction uses. Nil is the session.
	marker messageMarker
	// summaries is the agent's thread-summary cache, offered so a reply can
	// register itself. Nil outside the feature.
	summaries *threadSummaries
}

// messageMarker is the half of a session a reaction needs, so the marks the
// coalescing lane applies are testable without one. See sirens-echo#988.
type messageMarker interface {
	MessageReactionAdd(
		channelID, messageID, emojiID string, options ...discordgo.RequestOption,
	) error
	MessageReactionRemove(
		channelID, messageID, emojiID, userID string, options ...discordgo.RequestOption,
	) error
}

// LocationLabel names the room this turn is in, from cached state alone so it
// costs no call. See docs/sirens-echo-prompt.md.
func (t *discordMessageTurn) LocationLabel() string {
	if t.session == nil || t.session.State == nil || t.message == nil {
		return ""
	}
	if t.message.GuildID == "" {
		return "a direct message"
	}
	here, err := t.session.State.Channel(t.message.ChannelID)
	if err != nil || here == nil || here.Name == "" {
		return ""
	}
	if !here.IsThread() || here.ParentID == "" {
		return "#" + cleanTranscriptText(here.Name, 80)
	}
	name := cleanTranscriptText(here.Name, 80)
	parent, err := t.session.State.Channel(here.ParentID)
	if err != nil || parent == nil || parent.Name == "" {
		return "a thread named " + name
	}
	return "a thread named " + name +
		" in #" + cleanTranscriptText(parent.Name, 80)
}

// InterruptRecord names this summon well enough for a later boot to say which
// message went unanswered, without carrying any of its text.
func (t *discordMessageTurn) InterruptRecord() TurnRecord {
	record := TurnRecord{MessageID: t.message.ID, ChannelID: t.message.ChannelID}
	if t.message.Author != nil {
		record.Author = t.message.Author.ID
	}
	return record
}

// PrefillNote reports what the context budget dropped, which is zero for every
// turn that read the ordinary window.
func (t *discordMessageTurn) PrefillNote() prefillNote { return t.prefill }

// Attachments lets the completion layer reach a turn's uploads without taking
// a transport argument, the same route the reactions take.
func (t *discordMessageTurn) Attachments() []AttachmentSource {
	sources := make([]AttachmentSource, 0, len(t.folded)+1)
	for _, folded := range t.folded {
		sources = append(sources, attachmentSources(folded)...)
	}
	return append(sources, attachmentSources(t.message)...)
}

func (t *discordMessageTurn) RequestID() string {
	return t.message.ID
}

func (t *discordMessageTurn) Requester() string {
	if t.message == nil || t.message.Author == nil {
		return ""
	}
	return t.message.Author.ID
}

func (t *discordMessageTurn) Transport() string { return transportDiscord }

// SessionID shares a workspace with everyone in the thread, and falls back to
// the channel pairing outside one. See docs/sirens-echo-scratchpad.md.
func (t *discordMessageTurn) SessionID() SessionID {
	if t.message == nil {
		return SessionID{}
	}
	if t.session != nil && t.session.State != nil {
		channel, err := t.session.State.Channel(t.message.ChannelID)
		if err == nil && channel != nil && channel.IsThread() {
			return ThreadSession(t.message.ChannelID)
		}
	}
	return DirectSession(t.message.ChannelID, t.Requester())
}

// SpanAttributes places a turn. The account id is here by an explicit
// reversal, recorded in docs/sirens-echo-turn-stages.md.
func (t *discordMessageTurn) SpanAttributes() []attribute.KeyValue {
	// A DM contributes nothing, because a DM never enters the turn logger and
	// this is not the change that should be its first exception.
	if t.message == nil || t.message.GuildID == "" {
		return nil
	}
	attributes := discordMessageSpanAttributes(
		"receive",
		discordLocationFor(t.session, t.message),
		t.message.ID,
	)
	if requester := t.Requester(); requester != "" {
		attributes = append(attributes, attribute.String("discord.user.id", requester))
	}
	return attributes
}

// discordLocation is where a turn happened, with the thread separated from the
// channel it hangs under. See docs/sirens-echo-turn-stages.md.
type discordLocation struct {
	GuildID   string
	ChannelID string
	ThreadID  string
}

// discordLocationFor separates the two, because reporting a thread as its own
// channel hides the turn from a query for the channel it hangs under.
func discordLocationFor(
	session *discordgo.Session, message *discordgo.Message,
) discordLocation {
	at := discordLocation{GuildID: message.GuildID, ChannelID: message.ChannelID}
	if session == nil || session.State == nil {
		return at
	}
	// Cached state only. A turn is not worth a Discord API call, and a thread
	// this service can answer in arrived over the gateway to begin with.
	channel, err := session.State.Channel(message.ChannelID)
	if err != nil || channel == nil || !channel.IsThread() {
		return at
	}
	at.ChannelID = channel.ParentID
	at.ThreadID = message.ChannelID
	return at
}

// TraceLookup reads the referenced message too, because the id a member wants
// is usually in the notice they replied to rather than in what they typed.
func (t *discordMessageTurn) TraceLookup() (traceLookup, bool) {
	if t.message == nil {
		return traceLookup{}, false
	}
	referenced := ""
	if t.message.ReferencedMessage != nil {
		referenced = t.message.ReferencedMessage.Content
	}
	return detectTraceLookup(t.message.Content, referenced)
}

func (t *discordMessageTurn) Current() TranscriptEntry {
	return TranscriptEntry{
		Author:      displayName(t.message),
		Content:     t.currentContent(),
		Counterpart: counterpartOf(t.message),
		Attachments: t.currentAttachmentTypes(),
		ReplyTo:     replyTarget(t.message, t.replyTo),
		Comments:    len(t.folded) + 1,
	}
}

// currentContent is every comment this turn answers, in arrival order and in
// the member's own words. A batched ask reads as somebody saying several things.
func (t *discordMessageTurn) currentContent() string {
	parts := make([]string, 0, len(t.folded)+1)
	for _, folded := range t.folded {
		parts = append(parts, folded.ContentWithMentionsReplaced())
	}
	parts = append(parts, t.message.ContentWithMentionsReplaced())
	return strings.Join(parts, "\n\n")
}

// currentAttachmentTypes covers the whole batch, so an upload on a folded
// comment is as visible as one on the comment that carried the reply.
func (t *discordMessageTurn) currentAttachmentTypes() []string {
	types := make([]string, 0, len(t.folded)+1)
	for _, folded := range t.folded {
		types = append(types, attachmentTypes(folded)...)
	}
	return append(types, attachmentTypes(t.message)...)
}

// historyBefore anchors the transcript at the oldest comment this turn answers,
// so a folded comment is not read as context for itself.
func (t *discordMessageTurn) historyBefore() string {
	if len(t.folded) > 0 {
		return t.folded[0].ID
	}
	return t.message.ID
}

// replyTarget is the message a reply answers. Discord supplies it inline for
// most replies, and resolved carries the ones it did not. See sirens-echo#630.
func replyTarget(message, resolved *discordgo.Message) *ReplySubject {
	if message == nil {
		return nil
	}
	referenced := message.ReferencedMessage
	if referenced == nil {
		referenced = resolved
	}
	if referenced == nil {
		return nil
	}
	return &ReplySubject{
		Author:      displayName(referenced),
		Content:     referenced.ContentWithMentionsReplaced(),
		Counterpart: counterpartOf(referenced),
		Attachments: attachmentTypes(referenced),
	}
}

func (t *discordMessageTurn) History(_ context.Context) ([]TranscriptEntry, error) {
	messages, capped, err := readTurnHistory(
		t.session, t.wholeThread, t.message.ChannelID, t.historyBefore(), t.limit,
	)
	if err != nil {
		return nil, err
	}
	history := make([]TranscriptEntry, 0, len(messages))
	for _, message := range messages {
		if message.Author == nil {
			continue
		}
		history = append(history, TranscriptEntry{
			Author:      displayName(message),
			Content:     message.ContentWithMentionsReplaced(),
			Counterpart: counterpartOf(message),
			Attachments: attachmentTypes(message),
		})
		t.recordMentionable(message)
	}
	for _, folded := range t.folded {
		t.recordMentionable(folded)
	}
	t.recordMentionable(t.message)
	if !t.wholeThread {
		return history, nil
	}
	kept, dropped := dropOldestToFit(history, threadPrefillBytes)
	t.prefill = prefillNote{Dropped: dropped, Read: len(history), Capped: capped}
	return kept, nil
}

// recordMentionable adds a message's author and anyone it mentioned, which is
// the whole source: no membership lookup and no API call.
func (t *discordMessageTurn) recordMentionable(message *discordgo.Message) {
	if message == nil {
		return
	}
	if t.mentions == nil {
		t.mentions = mentionRoster{}
	}
	if message.Author != nil {
		t.mentions.add(displayName(message), message.Author.ID)
	}
	for _, mentioned := range message.Mentions {
		if mentioned != nil {
			t.mentions.add(mentioned.GlobalName, mentioned.ID)
			t.mentions.add(mentioned.Username, mentioned.ID)
		}
	}
}

// Typing shows the Discord indicator for this turn's channel.
// React marks the member's own message. Discord takes the emoji verbatim.
func (t *discordMessageTurn) React(_ context.Context, emoji string) error {
	return t.marks().MessageReactionAdd(t.message.ChannelID, t.message.ID, emoji)
}

// marks is the session half a reaction uses. Nil takes the session, so only a
// test sets it. See docs/sirens-echo-admission.md.
func (t *discordMessageTurn) marks() messageMarker {
	if t.marker != nil {
		return t.marker
	}
	return t.session
}

func (t *discordMessageTurn) Typing() error {
	return t.session.ChannelTyping(t.message.ChannelID)
}

// ReplyLimit is the Discord send budget, declared so a service-authored suffix
// can be kept inside it rather than truncated away.
func (t *discordMessageTurn) ReplyLimit() int { return discordReplyLimit }

// Unreact removes a mark the harness applied. Scoped to this identity, so a
// member's own reaction on the same message is never touched.
func (t *discordMessageTurn) Unreact(_ context.Context, emoji string) error {
	return t.marks().MessageReactionRemove(
		t.message.ChannelID, t.message.ID, emoji, "@me")
}

func (t *discordMessageTurn) Reply(ctx context.Context, content string) error {
	return t.send(ctx, content, nil)
}

// ReplyWithOverflow sends the message with the whole reply beside it. See
// docs/sirens-echo-reply-assembly.md.
func (t *discordMessageTurn) ReplyWithOverflow(ctx context.Context, content, whole string) error {
	return t.send(ctx, content, []*discordgo.File{{
		Name:        overflowFileName,
		ContentType: "text/plain; charset=utf-8",
		Reader:      strings.NewReader(whole),
	}})
}

func (t *discordMessageTurn) send(
	ctx context.Context, content string, files []*discordgo.File,
) error {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(discordMessageSpanAttributes(
		"send",
		discordLocationFor(t.session, t.message),
		"",
	)...)
	target := t.message.ChannelID
	reference := t.message.SoftReference()
	// Only ids the harness resolved, never Parse. A mention is something the
	// harness decided to deliver. See docs/sirens-echo-mentions.md.
	content, mentioned := t.mentions.resolveMentions(content)
	// A thread hangs off the member's message, so a reference inside it would
	// point at its own parent. See docs/sirens-echo-threads.md.
	if turnLongReply(ctx) {
		title := threadTitle(ctx, t.titler, t.message, t.RequestID(), t.telemetry)
		if threadID, threaded := threadForReply(t.session, t.message, title); threaded {
			target, reference = threadID, nil
			span.SetAttributes(attribute.String("discord.thread.id", threadID))
		}
	}
	reply, err := t.session.ChannelMessageSendComplex(target, &discordgo.MessageSend{
		Content:   truncateRunes(content, discordReplyLimit),
		Files:     files,
		Reference: reference,
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse:       []discordgo.AllowedMentionType{},
			Users:       mentioned,
			RepliedUser: false,
		},
	})
	if reply != nil && reply.ID != "" {
		span.SetAttributes(attribute.String("messaging.message.id", reply.ID))
		t.remember(target, reply.ID)
	}
	return err
}

// remember offers this reply as the thread's living answer. Nil summaries is
// every turn outside the feature. See docs/sirens-echo-threads.md.
func (t *discordMessageTurn) remember(channelID, messageID string) {
	if t.summaries == nil || channelID == "" {
		return
	}
	t.summaries.Remember(channelID, messageID)
}

// discordMessageSpanAttributes takes a location rather than loose identifiers,
// so no span can disagree with another about what a channel is.
func discordMessageSpanAttributes(
	operation string, at discordLocation, messageID string,
) []attribute.KeyValue {
	attributes := []attribute.KeyValue{
		attribute.String("messaging.system", "discord"),
		attribute.String("messaging.operation.name", operation),
		attribute.String("messaging.operation.type", operation),
		attribute.String("discord.guild.id", at.GuildID),
		attribute.String("discord.channel.id", at.ChannelID),
	}
	if at.ThreadID != "" {
		attributes = append(
			attributes,
			attribute.String("discord.thread.id", at.ThreadID),
		)
	}
	if messageID != "" {
		attributes = append(
			attributes,
			attribute.String("messaging.message.id", messageID),
		)
	}
	return attributes
}

func displayName(message *discordgo.Message) string {
	if message.Member != nil && strings.TrimSpace(message.Member.Nick) != "" {
		return message.Member.Nick
	}
	if message.Author != nil {
		if strings.TrimSpace(message.Author.GlobalName) != "" {
			return message.Author.GlobalName
		}
		return message.Author.Username
	}
	return "member"
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}

type channelScope struct {
	mu       sync.Mutex
	capacity int
	order    []string
	values   map[string]bool
}

func newChannelScope(capacity int) *channelScope {
	return &channelScope{
		capacity: capacity,
		order:    make([]string, 0, capacity),
		values:   make(map[string]bool, capacity),
	}
}

func (c *channelScope) Get(id string) (bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	allowed, known := c.values[id]
	return allowed, known
}

func (c *channelScope) Set(id string, allowed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.values[id]; exists {
		return
	}
	c.values[id] = allowed
	c.order = append(c.order, id)
	if len(c.order) > c.capacity {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.values, oldest)
	}
}

type seenMessages struct {
	mu       sync.Mutex
	capacity int
	order    []string
	values   map[string]struct{}
}

func newSeenMessages(capacity int) *seenMessages {
	return &seenMessages{
		capacity: capacity,
		order:    make([]string, 0, capacity),
		values:   make(map[string]struct{}, capacity),
	}
}

func (s *seenMessages) Add(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.values[id]; exists {
		return false
	}
	s.values[id] = struct{}{}
	s.order = append(s.order, id)
	if len(s.order) > s.capacity {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.values, oldest)
	}
	return true
}
