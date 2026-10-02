package main

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/coilyco/sirens-echo/internal/coalesce"
)

// bridgeScope names the instrumentation, matching the runtime's own convention.
const bridgeScope = "coilyco-gaming/sirens-echo/cmd/sirens-echo-bridge"

// observer emits the series a dashboard needs, every label a closed set. A nil
// observer is a deployment with no receiver configured, so every method guards.
type observer struct {
	depth        metric.Int64Gauge
	asks         metric.Int64Counter
	batchSize    metric.Int64Histogram
	turnDuration metric.Float64Histogram
	turns        metric.Int64Counter
	escalations  metric.Int64Counter
	deadLetters  metric.Int64Counter
}

func newObserver() (*observer, error) {
	meter := otel.Meter(bridgeScope)
	depth, err := meter.Int64Gauge("sirens_echo.coalesce.queue.depth")
	if err != nil {
		return nil, err
	}
	asks, err := meter.Int64Counter("sirens_echo.coalesce.asks")
	if err != nil {
		return nil, err
	}
	batchSize, err := meter.Int64Histogram("sirens_echo.coalesce.batch.size")
	if err != nil {
		return nil, err
	}
	turnDuration, err := meter.Float64Histogram(
		"sirens_echo.coalesce.turn.duration",
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}
	turns, err := meter.Int64Counter("sirens_echo.coalesce.turns")
	if err != nil {
		return nil, err
	}
	escalations, err := meter.Int64Counter("sirens_echo.coalesce.escalations")
	if err != nil {
		return nil, err
	}
	deadLetters, err := meter.Int64Counter("sirens_echo.coalesce.dead_letters")
	if err != nil {
		return nil, err
	}
	return &observer{
		depth:        depth,
		asks:         asks,
		batchSize:    batchSize,
		turnDuration: turnDuration,
		turns:        turns,
		escalations:  escalations,
		deadLetters:  deadLetters,
	}, nil
}

func (o *observer) Depth(ctx context.Context, depth int) {
	if o == nil {
		return
	}
	o.depth.Record(ctx, int64(depth))
}

func (o *observer) Batch(ctx context.Context, size int) {
	if o == nil {
		return
	}
	o.batchSize.Record(ctx, int64(size))
}

func (o *observer) Turn(ctx context.Context, outcome string, tier coalesce.Tier, took time.Duration) {
	if o == nil {
		return
	}
	options := metric.WithAttributes(
		attribute.String("outcome", outcome),
		attribute.String("tier", string(tier)),
	)
	o.turns.Add(ctx, 1, options)
	o.turnDuration.Record(ctx, float64(took.Microseconds())/1000, options)
}

func (o *observer) Escalated(ctx context.Context) {
	if o != nil {
		o.escalations.Add(ctx, 1)
	}
}

func (o *observer) DeadLettered(ctx context.Context) {
	if o != nil {
		o.deadLetters.Add(ctx, 1)
	}
}

// Accepted and Shed satisfy the ingress observer on the same instrument, so a
// dashboard reads arrivals and losses off one series.
func (o *observer) Accepted(ctx context.Context, surface string) {
	if o == nil {
		return
	}
	o.asks.Add(ctx, 1, metric.WithAttributes(
		attribute.String("outcome", "accepted"),
		attribute.String("surface", surface),
	))
}

func (o *observer) Shed(ctx context.Context, surface string) {
	if o == nil {
		return
	}
	o.asks.Add(ctx, 1, metric.WithAttributes(
		attribute.String("outcome", "shed"),
		attribute.String("surface", surface),
	))
}
