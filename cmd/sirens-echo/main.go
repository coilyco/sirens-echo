package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/coilyco-bridge/sirens-discord-ops/internal/community"
)

func main() {
	cfg, err := community.LoadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	agent, err := community.NewAgent(cfg)
	if err != nil {
		log.Fatalf("agent: %v", err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := agent.Run(ctx); err != nil {
		log.Fatalf("run: %v", err)
	}
}
