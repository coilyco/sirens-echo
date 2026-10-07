package community

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	logglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

const telemetryScope = "coilyco-gaming/sirens-echo/internal/community"

// Telemetry joins JSON logs, traces, and metrics for one Echo process.
// Kubernetes stdout and OTLP export carry them to the same SigNoZ stack.
type Telemetry struct {
	logger               *slog.Logger
	tracer               trace.Tracer
	turns                metric.Int64Counter
	turnDuration         metric.Float64Histogram
	modelCalls           metric.Int64Counter
	toolCalls            metric.Int64Counter
	commands             metric.Int64Counter
	attachments          metric.Int64Counter
	admissions           metric.Int64Counter
	accessChecks         metric.Int64Counter
	discordEvents        metric.Int64Counter
	summons              metric.Int64Counter
	phraseInvocations    metric.Int64Counter
	failures             metric.Int64Counter
	jobs                 metric.Int64Counter
	healthRequests       metric.Int64Counter
	readinessDuration    metric.Float64Histogram
	readinessState       metric.Int64Gauge
	readinessLastSuccess metric.Int64Gauge
	traceSDK             *sdktrace.TracerProvider
	logSDK               *sdklog.LoggerProvider
	metricSDK            *sdkmetric.MeterProvider
	traceProvider        trace.TracerProvider
	propagator           propagation.TextMapPropagator
}

// logSink chooses where structured logs go. Nil means stdout; a runner writing
// a dataset to stdout passes stderr instead. See sirens-echo#313.
func logSink(configured io.Writer) io.Writer {
	if configured == nil {
		return os.Stdout
	}
	return configured
}

// NewTelemetry initializes OTLP/HTTP export and a JSON logger, whose
// destination is chosen by logSink.
func NewTelemetry(ctx context.Context, cfg Config) (*Telemetry, error) {
	traceEndpoint, err := otlpSignalURL(cfg.OTLPEndpoint, "traces")
	if err != nil {
		return nil, err
	}
	metricEndpoint, err := otlpSignalURL(cfg.OTLPEndpoint, "metrics")
	if err != nil {
		return nil, err
	}
	traceExporter, err := otlptracehttp.New(
		ctx,
		otlptracehttp.WithEndpointURL(traceEndpoint),
	)
	if err != nil {
		return nil, err
	}
	metricExporter, err := newMetricExporter(ctx, metricEndpoint)
	if err != nil {
		_ = traceExporter.Shutdown(ctx)
		return nil, err
	}
	logEndpoint, err := otlpSignalURL(cfg.OTLPEndpoint, "logs")
	if err != nil {
		_ = traceExporter.Shutdown(ctx)
		_ = metricExporter.Shutdown(ctx)
		return nil, err
	}
	logExporter, err := otlploghttp.New(
		ctx,
		otlploghttp.WithEndpointURL(logEndpoint),
	)
	if err != nil {
		_ = traceExporter.Shutdown(ctx)
		_ = metricExporter.Shutdown(ctx)
		return nil, err
	}
	res, err := resource.New(
		ctx,
		resource.WithAttributes(
			attribute.String("service.name", valueOrDefault(cfg.InstanceName, defaultInstanceName)),
			attribute.String("service.namespace", "sirens"),
			attribute.String("agent.role", cfg.Definition.AuditRole),
			attribute.String("agent.attribution", cfg.Definition.Identity),
		),
	)
	if err != nil {
		_ = traceExporter.Shutdown(ctx)
		_ = metricExporter.Shutdown(ctx)
		_ = logExporter.Shutdown(ctx)
		return nil, err
	}
	traceSDK := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	metricSDK := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(
			metricExporter,
			sdkmetric.WithInterval(15*time.Second),
		)),
		sdkmetric.WithResource(res),
		sdkmetric.WithView(histogramViews()...),
	)
	logSDK := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
		sdklog.WithResource(res),
	)
	otel.SetTracerProvider(traceSDK)
	otel.SetMeterProvider(metricSDK)
	logglobal.SetLoggerProvider(logSDK)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	// Both destinations. Stdout keeps kubectl logs working when SigNoz is the
	// thing that is down. See docs/sirens-echo-rate.md.
	logger := slog.New(multiHandler{handlers: []slog.Handler{
		slog.NewJSONHandler(logSink(cfg.LogWriter), &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}),
		otelslog.NewHandler(
			telemetryScope,
			otelslog.WithLoggerProvider(logSDK),
		),
		// Breadcrumbs for the next Sentry crash event; it never raises one itself.
		crashBreadcrumbHandler{},
	}})
	telemetry, err := newTelemetry(logger, traceSDK, metricSDK)
	if err != nil {
		_ = logSDK.Shutdown(ctx)
		_ = metricSDK.Shutdown(ctx)
		_ = traceSDK.Shutdown(ctx)
		return nil, err
	}
	telemetry.traceSDK = traceSDK
	telemetry.metricSDK = metricSDK
	telemetry.logSDK = logSDK
	return telemetry, nil
}

// The SDK default boundaries top out at 10s, and a turn's p50 is above that, so
// every turn landed in the overflow bucket. See docs/sirens-echo-telemetry.md.
var turnDurationBoundaries = []float64{
	250, 500, 1000, 2500, 5000, 10000, 15000, 30000,
	45000, 60000, 90000, 120000, 180000, 240000, 300000,
}

// A batch is bounded by the narrow and wide batch sizes, so every batch fell in
// the default first bucket and 1 was indistinguishable from 4.
var batchSizeBoundaries = []float64{1, 2, 3, 4, 5, 6, 8, 10, 16}

// histogramViews replaces the default boundaries where the recorded range does
// not fit them. An instrument named here keeps the defaults deliberately.
func histogramViews() []sdkmetric.View {
	views := make([]sdkmetric.View, 0, 3)
	for name, boundaries := range map[string][]float64{
		"sirens_echo.turn.duration":          turnDurationBoundaries,
		"sirens_echo.coalesce.turn.duration": turnDurationBoundaries,
		"sirens_echo.coalesce.batch.size":    batchSizeBoundaries,
	} {
		views = append(views, sdkmetric.NewView(
			sdkmetric.Instrument{Name: name},
			sdkmetric.Stream{
				Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
					Boundaries: boundaries,
				},
			},
		))
	}
	return views
}

func newMetricExporter(
	ctx context.Context,
	endpoint string,
) (*otlpmetrichttp.Exporter, error) {
	return otlpmetrichttp.New(
		ctx,
		otlpmetrichttp.WithEndpointURL(endpoint),
		otlpmetrichttp.WithTemporalitySelector(sdkmetric.DeltaTemporalitySelector),
	)
}

func otlpSignalURL(base, signal string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" {
		return "", fmt.Errorf("invalid OTLP endpoint %q", base)
	}
	parsed.Path = path.Join(parsed.Path, "v1", signal)
	return parsed.String(), nil
}

func newTelemetry(
	logger *slog.Logger,
	tracerProvider trace.TracerProvider,
	meterProvider metric.MeterProvider,
) (*Telemetry, error) {
	meter := meterProvider.Meter(telemetryScope)
	turns, err := meter.Int64Counter("sirens_echo.turns")
	if err != nil {
		return nil, err
	}
	turnDuration, err := meter.Float64Histogram(
		"sirens_echo.turn.duration",
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}
	modelCalls, err := meter.Int64Counter("sirens_echo.model.calls")
	if err != nil {
		return nil, err
	}
	toolCalls, err := meter.Int64Counter("sirens_echo.tool.calls")
	if err != nil {
		return nil, err
	}
	commands, err := meter.Int64Counter("sirens_echo.commands")
	if err != nil {
		return nil, err
	}
	attachments, err := meter.Int64Counter("sirens_echo.attachments")
	if err != nil {
		return nil, err
	}
	admissions, err := meter.Int64Counter("sirens_echo.admissions")
	if err != nil {
		return nil, err
	}
	phraseInvocations, err := meter.Int64Counter("sirens_echo.phrase.invocations")
	if err != nil {
		return nil, err
	}
	accessChecks, err := meter.Int64Counter("sirens_echo.access.checks")
	if err != nil {
		return nil, err
	}
	discordEvents, err := meter.Int64Counter("sirens_echo.discord.events")
	if err != nil {
		return nil, err
	}
	summons, err := meter.Int64Counter("sirens_echo.summons")
	if err != nil {
		return nil, err
	}
	failures, err := meter.Int64Counter("sirens_echo.failures")
	if err != nil {
		return nil, err
	}
	jobs, err := meter.Int64Counter("sirens_echo.jobs")
	if err != nil {
		return nil, err
	}
	healthRequests, err := meter.Int64Counter("sirens_echo.health.requests")
	if err != nil {
		return nil, err
	}
	readinessDuration, err := meter.Float64Histogram(
		"sirens_echo.readiness.duration",
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}
	readinessState, err := meter.Int64Gauge(
		"sirens_echo.readiness.state",
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}
	readinessLastSuccess, err := meter.Int64Gauge(
		"sirens_echo.readiness.last_success",
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}
	return &Telemetry{
		logger:               logger,
		tracer:               tracerProvider.Tracer(telemetryScope),
		turns:                turns,
		turnDuration:         turnDuration,
		modelCalls:           modelCalls,
		toolCalls:            toolCalls,
		commands:             commands,
		attachments:          attachments,
		admissions:           admissions,
		accessChecks:         accessChecks,
		discordEvents:        discordEvents,
		summons:              summons,
		phraseInvocations:    phraseInvocations,
		failures:             failures,
		jobs:                 jobs,
		healthRequests:       healthRequests,
		readinessDuration:    readinessDuration,
		readinessState:       readinessState,
		readinessLastSuccess: readinessLastSuccess,
		traceProvider:        tracerProvider,
		propagator: propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	}, nil
}

func telemetryOrNoop(telemetry *Telemetry) *Telemetry {
	if telemetry != nil {
		return telemetry
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	noop, _ := newTelemetry(
		logger,
		tracenoop.NewTracerProvider(),
		metricnoop.NewMeterProvider(),
	)
	return noop
}

// StartSpan starts a child span using the service instrumentation scope. A
// span under a job carries the job id, so every span is queryable by it.
func (t *Telemetry) StartSpan(
	ctx context.Context,
	name string,
	attributes ...attribute.KeyValue,
) (context.Context, trace.Span) {
	if id := JobIDFromContext(ctx); id != "" {
		attributes = append(attributes, attribute.String("sirens_echo.job.id", id))
	}
	return t.tracer.Start(ctx, name, trace.WithAttributes(attributes...))
}

// Info emits one trace-correlated metadata log.
func (t *Telemetry) Info(ctx context.Context, message string, attrs ...slog.Attr) {
	t.log(ctx, slog.LevelInfo, message, attrs...)
}

// Warn emits one trace-correlated log for an expected, survivable event.
func (t *Telemetry) Warn(ctx context.Context, message string, attrs ...slog.Attr) {
	t.log(ctx, slog.LevelWarn, message, attrs...)
}

// Error emits one trace-correlated JSON error log.
func (t *Telemetry) Error(ctx context.Context, message string, attrs ...slog.Attr) {
	t.log(ctx, slog.LevelError, message, attrs...)
}

func (t *Telemetry) log(
	ctx context.Context,
	level slog.Level,
	message string,
	attrs ...slog.Attr,
) {
	spanContext := trace.SpanContextFromContext(ctx)
	if spanContext.IsValid() {
		attrs = append(
			attrs,
			slog.String("trace_id", spanContext.TraceID().String()),
			slog.String("span_id", spanContext.SpanID().String()),
		)
	}
	// A job id joins trace_id and span_id as a row field, so a job's whole log
	// history is retrievable by id. See docs/sirens-echo-jobs.md.
	if id := JobIDFromContext(ctx); id != "" {
		attrs = append(attrs, slog.String("job_id", id))
	}
	t.logger.LogAttrs(ctx, level, message, attrs...)
}

// RecordJob counts a job reaching a state. Kind and state are closed sets, so
// neither is an unbounded label.
func (t *Telemetry) RecordJob(ctx context.Context, kind, state string) {
	t.jobs.Add(ctx, 1, metric.WithAttributes(
		attribute.String("kind", kind),
		attribute.String("state", state),
	))
}

// StartJobSpan opens the span every one of a job's spans descends from, and
// puts the job id in the context so logs carry it too.
func (t *Telemetry) StartJobSpan(
	ctx context.Context,
	job Job,
) (context.Context, trace.Span) {
	ctx = ContextWithJobID(ctx, job.ID)
	return t.StartSpan(
		ctx,
		"job.execute",
		attribute.String("sirens_echo.job.id", job.ID),
		attribute.String("sirens_echo.job.kind", job.Kind),
		attribute.String("sirens_echo.job.transport", job.Origin.Transport),
		attribute.Int("sirens_echo.job.attempt", job.Attempts),
	)
}

// RecordTurn records the terminal state and latency of one accepted summon.
func (t *Telemetry) RecordTurn(
	ctx context.Context,
	outcome string,
	duration time.Duration,
) {
	options := metric.WithAttributes(attribute.String("outcome", outcome))
	t.turns.Add(ctx, 1, options)
	t.turnDuration.Record(ctx, float64(duration.Microseconds())/1000, options)
}

// RecordModelCall records one Agent Proxy round.
func (t *Telemetry) RecordModelCall(ctx context.Context, outcome string) {
	t.modelCalls.Add(
		ctx,
		1,
		metric.WithAttributes(attribute.String("outcome", outcome)),
	)
}

// RecordToolCall records one completed or failed MCP invocation.
func (t *Telemetry) RecordToolCall(
	ctx context.Context,
	server, tool, outcome string,
	elapsed time.Duration,
) {
	t.toolCalls.Add(
		ctx,
		1,
		metric.WithAttributes(
			attribute.String("mcp.server.name", server),
			attribute.String("mcp.tool.name", tool),
			attribute.String("outcome", outcome),
		),
	)
}

// RecordCommand records one workspace execution. The verb is this repository's
// own, so it is a label. Arguments carry a clone URL and never reach here.
func (t *Telemetry) RecordCommand(ctx context.Context, verb, outcome string) {
	t.commands.Add(
		ctx,
		1,
		metric.WithAttributes(
			attribute.String("command.verb", verb),
			attribute.String("outcome", outcome),
		),
	)
}

// RecordAttachment records one ingest attempt, including the refusals that used
// to return silently. The filename and the bytes never reach here.
func (t *Telemetry) RecordAttachment(ctx context.Context, outcome string) {
	t.attachments.Add(
		ctx,
		1,
		metric.WithAttributes(attribute.String("outcome", outcome)),
	)
}

// traceIDOf reads the correlation id off the span, or empty outside one.
func traceIDOf(ctx context.Context) string {
	span := trace.SpanContextFromContext(ctx)
	if !span.IsValid() {
		return ""
	}
	return span.TraceID().String()
}

// RecordAdmission records one admission decision. Both labels are closed sets,
// so a flood cannot expand metric cardinality through member-supplied values.
func (t *Telemetry) RecordAdmission(ctx context.Context, outcome, transport string) {
	t.admissions.Add(
		ctx,
		1,
		metric.WithAttributes(
			attribute.String("outcome", outcome),
			attribute.String("transport", transport),
		),
	)
}

// RecordAccess records one allowlist verdict. The reason is a closed set, so
// no guild, channel, or member identifier reaches a metric label.
func (t *Telemetry) RecordAccess(ctx context.Context, reason string) {
	t.accessChecks.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", reason)))
}

// RecordDiscordEvent counts one queue step for a gateway event. Kind and outcome
// are closed sets, so no Discord identifier reaches a label.
func (t *Telemetry) RecordDiscordEvent(ctx context.Context, kind, outcome string) {
	t.discordEvents.Add(ctx, 1, metric.WithAttributes(
		attribute.String("kind", kind),
		attribute.String("outcome", outcome),
	))
}

// RecordSummon counts one summon-gate decision. Both labels are closed sets,
// so a channel a flooder can open cannot expand cardinality.
func (t *Telemetry) RecordSummon(ctx context.Context, reason, contextKind string) {
	t.summons.Add(ctx, 1, metric.WithAttributes(
		attribute.String("reason", reason),
		attribute.String("context_kind", contextKind),
	))
}

// RecordPhrase records one canonical phrase a reply invoked. The key comes
// from the tracked registry, so no member value reaches a metric label.
func (t *Telemetry) RecordPhrase(ctx context.Context, key string) {
	t.phraseInvocations.Add(
		ctx,
		1,
		metric.WithAttributes(attribute.String("phrase.key", key)),
	)
}

// RecordFailure records the stage where an accepted turn failed.
func (t *Telemetry) RecordFailure(ctx context.Context, stage string) {
	t.failures.Add(
		ctx,
		1,
		metric.WithAttributes(attribute.String("stage", stage)),
	)
}

// RecordHealth counts one metrics-only health request with closed-set labels.
func (t *Telemetry) RecordHealth(ctx context.Context, endpoint, outcome string) {
	t.healthRequests.Add(
		ctx,
		1,
		metric.WithAttributes(
			attribute.String("endpoint", endpoint),
			attribute.String("outcome", outcome),
		),
	)
}

// RecordReadiness updates structural route readiness without creating an event.
func (t *Telemetry) RecordReadiness(
	ctx context.Context,
	outcome readinessOutcome,
	duration time.Duration,
) {
	options := metric.WithAttributes(attribute.String("outcome", string(outcome)))
	t.readinessDuration.Record(ctx, float64(duration.Microseconds())/1000, options)
	state := int64(0)
	if outcome == readinessReady {
		state = 1
		t.readinessLastSuccess.Record(ctx, time.Now().Unix())
	}
	t.readinessState.Record(ctx, state)
}

// Close flushes logs, metrics, and traces before process exit.
func (t *Telemetry) Close(ctx context.Context) error {
	var errs []error
	if t.metricSDK != nil {
		errs = append(errs, t.metricSDK.Shutdown(ctx))
	}
	if t.traceSDK != nil {
		errs = append(errs, t.traceSDK.Shutdown(ctx))
	}
	if t.logSDK != nil {
		errs = append(errs, t.logSDK.Shutdown(ctx))
	}
	return errors.Join(errs...)
}
