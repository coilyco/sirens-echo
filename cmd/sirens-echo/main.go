package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"forgejo.coilysiren.me/coilyco-gaming/sirens-echo/internal/community"
)

func main() {
	cfg, err := community.LoadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	telemetry, err := community.NewTelemetry(context.Background(), cfg)
	if err != nil {
		log.Fatalf("telemetry: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := telemetry.Close(shutdownCtx); err != nil {
			log.Printf("telemetry shutdown: %v", err)
		}
	}()
	agent, err := community.NewAgent(cfg, telemetry)
	if err != nil {
		log.Fatalf("agent: %v", err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := agent.Run(ctx); err != nil {
		log.Fatalf("run: %v", err)
	}
}
