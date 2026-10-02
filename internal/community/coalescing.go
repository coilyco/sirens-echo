package community

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/coilyco/sirens-echo/internal/coalesce"
	"github.com/coilyco/sirens-echo/internal/ingest"
)

// The Discord half of the coalescing lane, off until SIRENS_ECHO_COALESCE_ENABLED:
// ingress builds and marks, a pool worker answers. See docs/sirens-echo-admission.md.

// coalesceScope names the lane's instrumentation, kept apart from the runtime's
// own so a dashboard reads the lane without filtering the turn series.
const coalesceScope = telemetryScope + "/coalesce"

// coalesceLane holds the pieces one deployment runs. Nil on the Agent is the
// single execution slot, which is the shipped behaviour.
type coalesceLane struct {
	queue     *ingest.Queue
	ingress   *ingest.Ingress
	coalescer *coalesce.Coalescer
	pool      *coalesce.Pool
	started   sync.Once
	stopped   sync.Once
}

// buildCoalesceLane wires the lane onto this agent, reading every number it
// takes off the knobs.
func (a *Agent) buildCoalesceLane() {
	policy := coalescePolicy(a.cfg.RequestTimeout)
	metrics := newCoalesceMetrics(a.telemetry)
	queue := ingest.NewQueue(coalesceCapacity)
	a.lane = &coalesceLane{
		queue:     queue,
		ingress:   ingest.NewIngress(queue, &discordAck{agent: a}, a.telemetry, metrics, nil),
		coalescer: coalesce.NewCoalescer(policy, queue, nil),
		pool: coalesce.NewPool(
			policy, &batchRunner{agent: a}, &batchShelf{agent: a}, a.telemetry, metrics,
		),
	}
}

// coalescePolicy is the lane's tuning as the knobs stand. The deadline is the
// request budget, because a shorter one would cut a turn the service allows.
func coalescePolicy(requestTimeout time.Duration) coalesce.Policy {
	return coalesce.Policy{
		Window:     coalesceWindow,
		Batch:      coalesceBatch,
		WideWindow: coalesceWideWindow,
		WideBatch:  coalesceWideBatch,
		HighWater:  coalesceHighWater,
		AgeCap:     coalesceAgeCap,
		Workers:    coalesceWorkers,
		Deadline:   requestTimeout,
	}
}

// start launches the window and the workers from the drain root, so a batch in
// flight holds a restart open exactly as a serial turn does.
func (l *coalesceLane) start(root context.Context) {
	l.started.Do(func() {
		// The returned wait is not held: every ask keeps a drain slot until it
		// is answered or shed, and the shutdown grace already waits on those.
		_ = l.pool.Start(root, l.coalescer.Batches())
		go l.coalescer.Run(root)
	})
}

// stop closes the queue. What it already holds still becomes batches, because
// shutting down is not a reason to drop admitted work.
func (l *coalesceLane) stop() {
	l.stopped.Do(func() { l.queue.Close() })
}

// discordSummon is the transport handle one ask carries. The lane reads only
// its identity, and the worker reads the turn ingress already prepared.
type discordSummon struct {
	turn *discordMessageTurn
	// ctx descends from the span that received the comment, so the turn that
	// eventually answers it joins the trace ingress opened.
	ctx     context.Context
	settled sync.Once
	leave   func()
}

// ID names the comment inside Discord, which is what a dedupe and a done mark
// each need to say exactly what they covered.
func (d *discordSummon) ID() string { return d.turn.message.ID }

// release gives back this summon's hold on shutdown, once: an ask is answered
// or shed and never both.
func (d *discordSummon) release() { d.settled.Do(d.leave) }

// submitSummon hands one comment to the lane and returns as soon as it is
// acknowledged and queued, so a gateway handler never waits on a turn.
func (a *Agent) submitSummon(
	ctx context.Context, turn *discordMessageTurn, message *discordgo.Message,
) {
	// A hold of its own, because the one admitMessage took is released when
	// this returns and the ask outlives it by a whole window.
	if !a.drain.enter() {
		a.telemetry.RecordAccess(ctx, string(accessDeniedDraining))
		a.onDraining(turn.session, message)
		return
	}
	a.lane.ingress.Submit(
		ctx,
		ingest.Tenant{
			Surface: ingest.SurfaceDiscord,
			Guild:   message.GuildID,
			Channel: message.ChannelID,
			Author:  message.Author.ID,
		},
		message.ChannelID,
		message.ContentWithMentionsReplaced(),
		&discordSummon{turn: turn, ctx: ctx, leave: a.drain.leave},
	)
}

// discordAck marks each comment on arrival and retracts the mark on a shed ask.
// Folding the work is allowed, folding the ack is not.
type discordAck struct{ agent *Agent }

func (d *discordAck) Queued(ctx context.Context, ask ingest.Ask) error {
	if summon, ok := ask.Subject.(*discordSummon); ok {
		d.agent.react(ctx, summon.turn, reactionAccepted)
	}
	return nil
}

func (d *discordAck) Shed(ctx context.Context, ask ingest.Ask) error {
	summon, ok := ask.Subject.(*discordSummon)
	if !ok {
		return nil
	}
	defer summon.release()
	// The arrival mark promised an answer nobody will now produce, so it is
	// replaced by the outcome rather than left standing.
	d.agent.clearArrivalMark(ctx, summon.turn)
	d.agent.react(ctx, summon.turn, reactionFailed)
	return nil
}

// clearArrivalMark removes the mark ingress applied to a comment with no turn of
// its own to clear it. Swallowed, as every tidy-up here is.
func (a *Agent) clearArrivalMark(ctx context.Context, turn unreactor) {
	clearCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx), reactionClearTimeout,
	)
	defer cancel()
	if err := turn.Unreact(clearCtx, reactionAccepted); err != nil {
		a.telemetry.Info(
			clearCtx,
			"discord.reaction.remove.failed",
			slog.String("reaction", reactionAccepted),
			slog.String("error", err.Error()),
		)
	}
}

// batchRunner answers one batch as one Discord turn. Every comment in a batch
// came from one member in one channel, so one reply covers them.
type batchRunner struct{ agent *Agent }

func (r *batchRunner) Run(_ context.Context, batch coalesce.Batch) error {
	covered := summonsIn(batch.Asks())
	distinct := summonsIn(distinctAsks(batch))
	if len(distinct) == 0 {
		return nil
	}
	turn := foldTurn(distinct)
	newest := distinct[len(distinct)-1]
	// A panic in one batch must not take down a deployment serving other
	// guilds, the way the serial handler contains its own failure.
	defer r.recoverTurn(newest.ctx, turn)
	defer r.settle(newest.ctx, distinct, covered)
	if err := r.agent.runAdmitted(newest.ctx, turn); err != nil {
		r.agent.telemetry.Error(newest.ctx, "discord.turn.failed", append(
			[]slog.Attr{slog.String("error_type", "turn_failed")},
			turnFailureAttrs(err)...,
		)...)
	}
	// The turn owns its own model retry and told the member itself what
	// happened, so the pool's ladder is not stacked on top of it.
	return nil
}

// recoverTurn contains a crashed batch and tells the member, because a turn
// that died silently reads as being ignored. See docs/sirens-echo-delivery.md.
func (r *batchRunner) recoverTurn(ctx context.Context, turn turnIO) {
	recovered := recover()
	if recovered == nil {
		return
	}
	r.agent.telemetry.RecordFailure(ctx, "panic")
	r.agent.telemetry.Error(
		ctx,
		"discord.turn.panicked",
		slog.String("error_type", "turn_panicked"),
	)
	_ = r.agent.notifyFailure(context.WithoutCancel(ctx), turn, noticeTurnCrashed)
}

// settle clears the arrival marks the folded comments still carry and gives
// back every hold the batch held on shutdown.
func (r *batchRunner) settle(ctx context.Context, distinct, covered []*discordSummon) {
	for _, summon := range distinct[:len(distinct)-1] {
		// The newest comment's marks are cleared by the turn that answered on
		// it. A folded comment has no turn of its own.
		r.agent.clearArrivalMark(ctx, summon.turn)
	}
	for _, summon := range covered {
		summon.release()
	}
}

// batchShelf takes a batch the pool gave up on, which here is a shutdown. It
// marks and says nothing: the gateway a reply needs is what is closing.
type batchShelf struct{ agent *Agent }

func (s *batchShelf) Shelve(ctx context.Context, batch coalesce.Batch, _ error) {
	for _, summon := range summonsIn(batch.Asks()) {
		s.agent.react(ctx, summon.turn, reactionFailed)
		summon.release()
	}
}

// summonsIn resolves the transport handles the lane carried, skipping an ask
// that came from anywhere else.
func summonsIn(asks []ingest.Ask) []*discordSummon {
	summons := make([]*discordSummon, 0, len(asks))
	for _, ask := range asks {
		if summon, ok := ask.Subject.(*discordSummon); ok {
			summons = append(summons, summon)
		}
	}
	return summons
}

// distinctAsks is the first ask carrying each distinct request. Dedupe collapsed
// the work rather than the asks, so marks and holds still cover every one.
func distinctAsks(batch coalesce.Batch) []ingest.Ask {
	asks := make([]ingest.Ask, 0, len(batch.Items))
	for _, item := range batch.Items {
		if len(item.Covers) > 0 {
			asks = append(asks, item.Covers[0])
		}
	}
	sort.Slice(asks, func(i, j int) bool { return asks[i].Seq < asks[j].Seq })
	return asks
}

// foldTurn builds the one turn that answers the batch. The newest comment
// carries the reply, and the earlier ones fold into what it asks.
func foldTurn(summons []*discordSummon) *discordMessageTurn {
	turn := summons[len(summons)-1].turn
	folded := make([]*discordgo.Message, 0, len(summons)-1)
	for _, summon := range summons[:len(summons)-1] {
		folded = append(folded, summon.turn.message)
	}
	turn.folded = folded
	return turn
}

// coalesceMetrics reports what the lane did, every label a closed set. Nil
// records nothing, which is a deployment whose instruments would not build.
type coalesceMetrics struct {
	depth        metric.Int64Gauge
	asks         metric.Int64Counter
	batchSize    metric.Int64Histogram
	turns        metric.Int64Counter
	turnDuration metric.Float64Histogram
	escalations  metric.Int64Counter
	deadLetters  metric.Int64Counter
}

func newCoalesceMetrics(telemetry *Telemetry) *coalesceMetrics {
	meter := otel.Meter(coalesceScope)
	metrics := &coalesceMetrics{}
	var err error
	if metrics.depth, err = meter.Int64Gauge("sirens_echo.coalesce.queue.depth"); err != nil {
		return failedCoalesceMetrics(telemetry, err)
	}
	if metrics.asks, err = meter.Int64Counter("sirens_echo.coalesce.asks"); err != nil {
		return failedCoalesceMetrics(telemetry, err)
	}
	if metrics.batchSize, err = meter.Int64Histogram("sirens_echo.coalesce.batch.size"); err != nil {
		return failedCoalesceMetrics(telemetry, err)
	}
	if metrics.turns, err = meter.Int64Counter("sirens_echo.coalesce.turns"); err != nil {
		return failedCoalesceMetrics(telemetry, err)
	}
	metrics.turnDuration, err = meter.Float64Histogram(
		"sirens_echo.coalesce.turn.duration", metric.WithUnit("ms"),
	)
	if err != nil {
		return failedCoalesceMetrics(telemetry, err)
	}
	if metrics.escalations, err = meter.Int64Counter("sirens_echo.coalesce.escalations"); err != nil {
		return failedCoalesceMetrics(telemetry, err)
	}
	if metrics.deadLetters, err = meter.Int64Counter("sirens_echo.coalesce.dead_letters"); err != nil {
		return failedCoalesceMetrics(telemetry, err)
	}
	return metrics
}

// failedCoalesceMetrics names the instrument that would not build and goes
// silent: a metric is not worth refusing to serve over.
func failedCoalesceMetrics(telemetry *Telemetry, err error) *coalesceMetrics {
	telemetry.Error(
		context.Background(),
		"coalesce.metrics.failed",
		slog.String("error_type", "metrics_unavailable"),
		slog.String("error", err.Error()),
	)
	return nil
}

func (m *coalesceMetrics) Depth(ctx context.Context, depth int) {
	if m != nil {
		m.depth.Record(ctx, int64(depth))
	}
}

func (m *coalesceMetrics) Batch(ctx context.Context, size int) {
	if m != nil {
		m.batchSize.Record(ctx, int64(size))
	}
}

func (m *coalesceMetrics) Turn(
	ctx context.Context, outcome string, tier coalesce.Tier, took time.Duration,
) {
	if m == nil {
		return
	}
	options := metric.WithAttributes(
		attribute.String("outcome", outcome),
		attribute.String("tier", string(tier)),
	)
	m.turns.Add(ctx, 1, options)
	m.turnDuration.Record(ctx, float64(took.Microseconds())/1000, options)
}

func (m *coalesceMetrics) Escalated(ctx context.Context) {
	if m != nil {
		m.escalations.Add(ctx, 1)
	}
}

func (m *coalesceMetrics) DeadLettered(ctx context.Context) {
	if m != nil {
		m.deadLetters.Add(ctx, 1)
	}
}

// Accepted and Shed satisfy the ingress observer on one instrument, so a
// dashboard reads arrivals and losses off the same series.
func (m *coalesceMetrics) Accepted(ctx context.Context, surface string) {
	m.countAsk(ctx, "accepted", surface)
}

func (m *coalesceMetrics) Shed(ctx context.Context, surface string) {
	m.countAsk(ctx, "shed", surface)
}

func (m *coalesceMetrics) countAsk(ctx context.Context, outcome, surface string) {
	if m == nil {
		return
	}
	m.asks.Add(ctx, 1, metric.WithAttributes(
		attribute.String("outcome", outcome),
		attribute.String("surface", surface),
	))
}
