// Command sirens-echo-intake holds a Discord gateway session and writes every
// event it receives to the Postgres queue the worker answers from. Several run
// at once. See docs/sirens-echo-jobs.md.
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

func main() {
	defer community.RecoverCrash()
	startup := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if _, err := community.InitCrashReporting(); err != nil {
		// The type only: a DSN parse error can carry the DSN.
		startup.Warn("startup.crash_reporting.failed", slog.String("error_type", fmt.Sprintf("%T", err)))
	}
	cfg, err := community.LoadIntakeConfig()
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
		_ = telemetry.Close(shutdownCtx)
	}()
	intake, err := community.NewIntake(cfg, telemetry)
	if err != nil {
		telemetry.Error(context.Background(), "startup.intake.failed", slog.String("error", err.Error()))
		community.ReportCrash(err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := intake.Run(ctx); err != nil {
		telemetry.Error(ctx, "run.failed", slog.String("error", err.Error()))
		community.ReportCrash(err)
		os.Exit(1)
	}
}
