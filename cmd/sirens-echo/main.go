package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coilyco/sirens-echo/internal/community"
)

// startupLogger matches Telemetry's JSON shape and stream, for the two
// failures that land before Telemetry exists. See docs/sirens-echo-delivery.md.
func startupLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

func main() {
	defer community.RecoverCrash()
	startup := startupLogger()
	if _, err := community.InitCrashReporting(); err != nil {
		// The type only: a DSN parse error can carry the DSN.
		startup.Warn("startup.crash_reporting.failed", slog.String("error_type", fmt.Sprintf("%T", err)))
	}
	cfg, err := community.LoadConfig()
	if err != nil {
		startup.Error("startup.config.failed", slog.String("error", err.Error()))
		community.ReportCrash(err)
		os.Exit(1)
	}
	community.SetCrashService(cfg.InstanceName)
	telemetry, err := community.NewTelemetry(context.Background(), cfg)
	if err != nil {
		startup.Error("startup.telemetry.failed", slog.String("error", err.Error()))
		community.ReportCrash(err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := telemetry.Close(shutdownCtx); err != nil {
			telemetry.Error(
				shutdownCtx,
				"shutdown.telemetry.failed",
				slog.String("error", err.Error()),
			)
		}
	}()
	agent, err := community.NewAgent(cfg, telemetry)
	if err != nil {
		telemetry.Error(
			context.Background(),
			"startup.agent.failed",
			slog.String("error", err.Error()),
		)
		community.ReportCrash(err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := agent.Run(ctx); err != nil {
		telemetry.Error(ctx, "run.failed", slog.String("error", err.Error()))
		community.ReportCrash(err)
		os.Exit(1)
	}
}
