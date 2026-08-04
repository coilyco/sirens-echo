package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"forgejo.coilysiren.me/coilyco-gaming/sirens-echo/internal/community"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const defaultEvaluationOTLPEndpoint = "http://ser8:30418"

func main() {
	definition, err := community.LoadDefinition("agent/sirens-echo.json")
	if err != nil {
		log.Fatalf("definition: %v", err)
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
		BaseURL:     proxyURL,
		Model:       proxyModel,
		RouteRole:   definition.RouteRole,
		Attribution: definition.Identity,
		HTTPClient:  httpClient,
		Tools: community.MCPProvider{
			Servers:    staticMCPServers(definition.MCPServers),
			HTTPClient: httpClient,
		},
		Telemetry: telemetry,
	}
	if err := community.RunEvaluation(
		context.Background(),
		definition,
		localSkillpack,
		pack,
		client,
		os.Stdout,
	); err != nil {
		log.Fatalf("evaluation: %v", err)
	}
}

func staticMCPServers(servers []community.MCPServerDefinition) []community.MCPServerDefinition {
	static := make([]community.MCPServerDefinition, 0, len(servers))
	for _, server := range servers {
		if server.URL != "" {
			static = append(static, server)
		}
	}
	return static
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
