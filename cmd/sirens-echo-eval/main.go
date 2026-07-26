package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coilyco-bridge/sirens-discord-ops/internal/community"
)

func main() {
	definition, err := community.LoadDefinition("agent/sirens-echo.json")
	if err != nil {
		log.Fatalf("definition: %v", err)
	}
	skillpack, err := community.LoadSkillpack(definition.SkillRoots)
	if err != nil {
		log.Fatalf("skillpack: %v", err)
	}
	pack, err := community.LoadEvaluationPack("agent/evaluation.json")
	if err != nil {
		log.Fatalf("evaluation pack: %v", err)
	}
	proxyURL := strings.TrimSpace(os.Getenv("AGENT_PROXY_URL"))
	if proxyURL == "" {
		proxyURL = community.DefaultAgentProxyURL
	}
	timeout := 5 * time.Minute
	client := community.ProxyClient{
		BaseURL:    proxyURL,
		Model:      definition.Model,
		HTTPClient: &http.Client{Timeout: timeout},
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := community.RunEvaluation(ctx, definition, skillpack, pack, client, os.Stdout); err != nil {
		log.Fatalf("evaluation: %v", err)
	}
}
