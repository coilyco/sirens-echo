package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coilyco-bridge/sirens-discord-ops/internal/community"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	definition, err := community.LoadDefinition("agent/sirens-echo.json")
	if err != nil {
		log.Fatalf("definition: %v", err)
	}
	bundlePath := valueOrDefault(
		os.Getenv("SIRENS_ECHO_AOS_BUNDLE"),
		community.DefaultAOSBundlePath,
	)
	bundle, err := community.LoadAOSBundle(bundlePath, definition.Role)
	if err != nil {
		log.Fatalf("bundle: %v", err)
	}
	localSkillpack, err := community.LoadSkillpack(definition.LocalSkillRoots)
	if err != nil {
		log.Fatalf("skillpack: %v", err)
	}
	pack, err := community.LoadEvaluationPack("agent/evaluation.json")
	if err != nil {
		log.Fatalf("evaluation pack: %v", err)
	}
	proxyURL := valueOrDefault(os.Getenv("AGENT_PROXY_URL"), community.DefaultAgentProxyURL)
	proxyModel := strings.TrimSpace(os.Getenv("AGENT_PROXY_MODEL"))
	if proxyModel == "" {
		log.Fatal("AGENT_PROXY_MODEL is required from the selected AOSH route")
	}
	timeout := 5 * time.Minute
	telemetry, err := community.NewTelemetry(context.Background(), community.Config{
		Definition:   definition,
		OTLPEndpoint: valueOrDefault(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"), community.DefaultOTLPEndpoint),
	})
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
	httpClient := &http.Client{
		Timeout:   timeout,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
	client := community.ProxyClient{
		BaseURL:    proxyURL,
		Model:      proxyModel,
		Role:       definition.Role,
		Seat:       definition.Seat,
		HTTPClient: httpClient,
		Tools: community.MCPProvider{
			Servers:    definition.MCPServers,
			HTTPClient: httpClient,
		},
		Telemetry: telemetry,
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := community.RunEvaluation(
		ctx,
		definition,
		bundle.Context,
		localSkillpack,
		pack,
		client,
		os.Stdout,
	); err != nil {
		log.Fatalf("evaluation: %v", err)
	}
}

func valueOrDefault(value, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}
