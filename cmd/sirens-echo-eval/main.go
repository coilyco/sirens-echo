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

const defaultEvaluationOTLPEndpoint = "http://ser8:30418"

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
	intent := valueOrDefault(
		os.Getenv("SIRENS_ECHO_INTENT"),
		definition.DefaultIntent,
	)
	route, err := definition.LogicalRoute(intent)
	if err != nil {
		log.Fatalf("intent: %v", err)
	}
	timeout := 5 * time.Minute
	telemetry, err := community.NewTelemetry(context.Background(), community.Config{
		Definition:   definition,
		OTLPEndpoint: evaluationOTLPEndpoint(),
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
		Route:      route,
		Intent:     intent,
		Harness:    definition.Harness,
		HTTPClient: httpClient,
		Tools: community.MCPProvider{
			Servers:    definition.MCPServers,
			HTTPClient: httpClient,
		},
		Telemetry: telemetry,
	}
	if err := community.RunEvaluation(
		context.Background(),
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

func evaluationOTLPEndpoint() string {
	return valueOrDefault(
		os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		defaultEvaluationOTLPEndpoint,
	)
}

func valueOrDefault(value, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}
