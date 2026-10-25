package community

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/getsentry/sentry-go"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// These share the process-wide Sentry client with crash_test.go, so none of
// them runs in parallel.

func sentLogs(transport *captureTransport) []sentry.Log {
	sentry.Flush(crashFlushTimeout)
	var logs []sentry.Log
	for _, event := range transport.sent() {
		logs = append(logs, event.Logs...)
	}
	return logs
}

func TestAnErrorLogReachesSentryLogsLinkedToItsTrace(t *testing.T) {
	transport := withCapturedCrashes(t)
	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	ctx, span := provider.Tracer("test").Start(context.Background(), "community.turn")
	defer span.End()

	logger := slog.New(sentryLogHandler{})
	logger.ErrorContext(ctx, "turn.stage.failed", slog.String("stage", "model"), slog.Int("round", 3))

	logs := sentLogs(transport)
	if len(logs) != 1 {
		t.Fatalf("sent %d logs, want 1", len(logs))
	}
	log := logs[0]
	if log.Body != "turn.stage.failed" || log.Level != sentry.LogLevelError {
		t.Errorf("log = %q at %s", log.Body, log.Level)
	}
	if want := span.SpanContext().TraceID().String(); log.TraceID.String() != want {
		t.Errorf("trace id = %s, want the turn's %s", log.TraceID, want)
	}
	if got := log.Attributes["stage"].AsString(); got != "model" {
		t.Errorf("stage attribute = %q", got)
	}
	if got := log.Attributes["round"].AsInt64(); got != 3 {
		t.Errorf("round attribute = %d", got)
	}
}

func TestOnlyErrorLevelRecordsGoToSentryLogs(t *testing.T) {
	transport := withCapturedCrashes(t)
	logger := slog.New(sentryLogHandler{})
	logger.Info("turn.admitted")
	logger.Warn("mcp.warm.slow")
	if logs := sentLogs(transport); len(logs) != 0 {
		t.Fatalf("sent %d logs below error level", len(logs))
	}
}

func TestASentryLogLeavesOutMemberTextAndErrorText(t *testing.T) {
	transport := withCapturedCrashes(t)
	secret := strings.Join([]string{"MEMBER", "TEXT"}, "-")
	logger := slog.New(sentryLogHandler{}).With(slog.String("content", secret))
	logger.Error("discord.commands.registration_failed",
		slog.String("error", "dial tcp 10.0.0.1: token abc123 refused"),
		slog.Any("cause", errors.New("token abc123 refused")),
		slog.String("error_type", "registration_failed"))

	logs := sentLogs(transport)
	if len(logs) != 1 {
		t.Fatalf("sent %d logs, want 1", len(logs))
	}
	for key, value := range logs[0].Attributes {
		if strings.Contains(value.AsString(), "abc123") || strings.Contains(value.AsString(), secret) {
			t.Errorf("attribute %s leaked %q", key, value.AsString())
		}
	}
	if got := logs[0].Attributes["error_type"].AsString(); got != "registration_failed" {
		t.Errorf("closed-set error_type = %q, want it kept", got)
	}
}

func TestNoDSNMeansNoSentryLogs(t *testing.T) {
	if _, err := initCrashReporting("", nil); err != nil {
		t.Fatal(err)
	}
	if slog.New(sentryLogHandler{}).Enabled(context.Background(), slog.LevelError) {
		t.Fatal("the handler was enabled with Sentry off")
	}
}
