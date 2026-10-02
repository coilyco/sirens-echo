// Command sirens-echo-bridge runs the coalescing lane: acknowledged ingress,
// a bounded window, and a worker pool that answers one batch per turn.
//
// It exists so the lane is runnable and measurable on its own. The smoke feed
// drives the real pipeline with no proxy and no Discord token, which is the
// acceptance evidence; stdin mode pipes real asks through the same code.
// See docs/sirens-echo-admission.md.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/coilyco/sirens-echo/internal/coalesce"
	"github.com/coilyco/sirens-echo/internal/community"
	"github.com/coilyco/sirens-echo/internal/ingest"
)

// The bridge takes its tuning as flags. The service takes the same numbers as
// SIRENS_ECHO_COALESCE_* knobs, so neither reads the other's configuration.
type options struct {
	smoke    time.Duration
	rate     time.Duration
	turn     time.Duration
	workers  int
	window   time.Duration
	batch    int
	capacity int
	system   string
}

func parseOptions() options {
	var o options
	flag.DurationVar(&o.smoke, "smoke", 0,
		"run the synthetic feed for this long instead of reading stdin")
	flag.DurationVar(&o.rate, "rate", 30*time.Second, "smoke feed arrival interval")
	flag.DurationVar(&o.turn, "turn", 12*time.Second, "smoke feed turn latency")
	flag.IntVar(&o.workers, "workers", coalesce.DefaultWorkers, "batch workers")
	flag.DurationVar(&o.window, "window", coalesce.DefaultWindow, "narrow window span")
	flag.IntVar(&o.batch, "batch", coalesce.DefaultBatch, "narrow window ask count")
	flag.IntVar(&o.capacity, "capacity", ingest.DefaultCapacity, "bounded queue capacity")
	flag.StringVar(&o.system, "system", "Answer exactly the comments listed.",
		"system prompt for the proxy runner")
	flag.Parse()
	return o
}

func (o options) policy() coalesce.Policy {
	policy := coalesce.DefaultPolicy()
	policy.Workers = o.workers
	policy.Window = o.window
	policy.Batch = o.batch
	return policy
}

func main() {
	o := parseOptions()
	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	out := newStream(os.Stdout)
	bridge := &bridgeLogger{log: log}
	metrics := resolveObserver(ctx, bridge)

	queue := ingest.NewQueue(o.capacity)
	in := ingest.NewIngress(queue, out, bridge, metrics, nil)
	coalescer := coalesce.NewCoalescer(o.policy(), queue, nil)
	pool := coalesce.NewPool(o.policy(), resolveRunner(o, out), out, bridge, metrics)

	wait := pool.Start(ctx, coalescer.Batches())
	go coalescer.Run(ctx)

	if o.smoke > 0 {
		feedSmoke(ctx, in, o)
	} else {
		feedStdin(ctx, in, bridge)
	}
	queue.Close()
	wait()
	log.Info("bridge.drained")
}

// resolveObserver exports metrics only when a receiver is configured, so the
// smoke feed runs on a laptop with no collector in reach.
func resolveObserver(ctx context.Context, log *bridgeLogger) *observer {
	if strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) == "" {
		return nil
	}
	cfg, err := community.LoadConfig()
	if err != nil {
		log.Error(ctx, "bridge.telemetry.config.failed", slog.String("error", err.Error()))
		return nil
	}
	if _, err := community.NewTelemetry(ctx, cfg); err != nil {
		log.Error(ctx, "bridge.telemetry.failed", slog.String("error", err.Error()))
		return nil
	}
	metrics, err := newObserver()
	if err != nil {
		log.Error(ctx, "bridge.metrics.failed", slog.String("error", err.Error()))
		return nil
	}
	return metrics
}

// resolveRunner picks the stub for the smoke feed and the proxy otherwise. The
// proxy values skip LoadConfig, whose role bundle a criteria prompt never uses.
func resolveRunner(o options, out *stream) coalesce.Runner {
	if o.smoke > 0 {
		return &stubRunner{takes: o.turn, sink: out}
	}
	return &proxyRunner{
		base: community.ProxyClient{
			BaseURL: valueOr(os.Getenv("AGENT_PROXY_URL"), community.DefaultAgentProxyURL),
			Model:   strings.TrimSpace(os.Getenv("AGENT_PROXY_MODEL")),
			// Required, not optional: a nil client is dereferenced rather than
			// defaulted, so leaving it unset panics on the first turn.
			HTTPClient: &http.Client{
				Transport: otelhttp.NewTransport(http.DefaultTransport),
			},
		},
		direct: os.Getenv("SIRENS_ECHO_BRIDGE_MODEL_DIRECT"),
		pro:    os.Getenv("SIRENS_ECHO_BRIDGE_MODEL_PRO"),
		system: o.system,
		sink:   out,
	}
}

// feedSmoke is the acceptance feed: steady arrivals with irregular bursts, run
// against the real pipeline rather than a simulation of it.
func feedSmoke(ctx context.Context, in *ingest.Ingress, o options) {
	deadline := time.Now().Add(o.smoke)
	ticker := time.NewTicker(o.rate)
	defer ticker.Stop()
	burst := 0
	for time.Now().Before(deadline) {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
		in.Submit(ctx, smokeTenant("ana"), "thread-1", "steady ask", nil)
		burst++
		if burst%4 != 0 {
			continue
		}
		// An even feed never exercises K, and a burst is the shape that does.
		for i := 0; i < 8; i++ {
			in.Submit(ctx, smokeTenant("bo"), "thread-2", "burst ask", nil)
		}
	}
}

func smokeTenant(author string) ingest.Tenant {
	return ingest.Tenant{
		Surface: ingest.SurfaceDiscord,
		Guild:   "smoke",
		Channel: "smoke",
		Author:  author,
	}
}

// inbound is one ask on the wire, which is what a Discord dump normalizes to.
type inbound struct {
	Surface string `json:"surface"`
	Guild   string `json:"guild"`
	Channel string `json:"channel"`
	Author  string `json:"author"`
	Locus   string `json:"locus"`
	Text    string `json:"text"`
}

// feedStdin reads asks until the input ends, so the bridge composes with the
// dump and normalization pipelines this repository already ships.
func feedStdin(ctx context.Context, in *ingest.Ingress, log *bridgeLogger) {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ask inbound
		if err := json.Unmarshal([]byte(line), &ask); err != nil {
			log.Error(ctx, "bridge.input.unreadable", slog.String("error", err.Error()))
			continue
		}
		if ask.Surface == "" {
			ask.Surface = ingest.SurfaceDiscord
		}
		in.Submit(ctx, ingest.Tenant{
			Surface: ask.Surface,
			Guild:   ask.Guild,
			Channel: ask.Channel,
			Author:  ask.Author,
		}, ask.Locus, ask.Text, nil)
		if ctx.Err() != nil {
			return
		}
	}
}

// valueOr keeps an unset environment name on the packaged fallback.
func valueOr(value, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}

// bridgeLogger adapts slog to the shape both packages declare.
type bridgeLogger struct{ log *slog.Logger }

func (b *bridgeLogger) Info(ctx context.Context, message string, attrs ...slog.Attr) {
	b.log.LogAttrs(ctx, slog.LevelInfo, message, attrs...)
}

func (b *bridgeLogger) Error(ctx context.Context, message string, attrs ...slog.Attr) {
	b.log.LogAttrs(ctx, slog.LevelError, message, attrs...)
}
