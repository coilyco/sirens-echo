package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/coilyco/sirens-echo/internal/community"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Neutral fallback only. Native evaluation runs set OTEL_EXPORTER_OTLP_ENDPOINT
// to the receiver they actually target.
const defaultEvaluationOTLPEndpoint = "http://localhost:4318"

const (
	defaultEvaluationDefinition = "agents/echo/definition.yaml"
	defaultEvaluationPack       = "agents/echo/packs/evaluation.yaml"
	// An evaluation run is not the deployment. Left unset it reported as
	// sirens-echo and mixed into production. See sirens-echo#533.
	evaluationInstanceName = "sirens-echo-eval"
)

func main() {
	definition, err := community.LoadDefinition(evaluationDefinitionPath())
	if err != nil {
		log.Fatalf("definition: %v", err)
	}
	localSkillpack, err := community.LoadSkillpack(definition.LocalSkillRoots)
	if err != nil {
		log.Fatalf("skillpack: %v", err)
	}
	packPath := evaluationPackPath()
	packSchema, err := community.PackSchema(packPath)
	if err != nil {
		log.Fatalf("pack schema: %v", err)
	}
	preserveOutOfRepoPack(packPath)
	proxyURL := valueOrDefault(os.Getenv("AGENT_PROXY_URL"), community.DefaultAgentProxyURL)
	proxyModel := strings.TrimSpace(os.Getenv("AGENT_PROXY_MODEL"))
	if proxyModel == "" {
		log.Fatal("AGENT_PROXY_MODEL is required from the selected AOSH route")
	}
	timeout := 5 * time.Minute
	// The dataset goes to stdout, so logs must not. See sirens-echo#313.
	telemetry, err := community.NewTelemetry(context.Background(), community.Config{
		Definition:   definition,
		InstanceName: evaluationInstanceName,
		OTLPEndpoint: evaluationOTLPEndpoint(),
		LogWriter:    os.Stderr,
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
	rosterPath := strings.TrimSpace(os.Getenv("SIRENS_ECHO_MCP_ROSTER"))
	client := community.ProxyClient{
		BaseURL:       proxyURL,
		Model:         proxyModel,
		AuditRole:     definition.AuditRole,
		Attribution:   definition.Identity,
		ResponseStyle: definition.ResponseStyle,
		HTTPClient:    httpClient,
		Tools:         evaluationTools(rosterPath, httpClient),
		Telemetry:     telemetry,
	}
	if packSchema == community.BoardSchema {
		runBoardPack(definition, localSkillpack, packPath, proxyURL, proxyModel, rosterPath, client)
		return
	}
	if packSchema == community.RateSchema {
		runRatePack(definition, localSkillpack, packPath, proxyURL, proxyModel, rosterPath, client)
		return
	}
	pack, err := community.LoadEvaluationPack(packPath)
	if err != nil {
		log.Fatalf("evaluation pack: %v", err)
	}
	warnUnservedRequiredTools(pack.Cases, rosterPath)
	if err := community.RunEvaluation(
		context.Background(),
		definition,
		community.PlaceholderPrincipal,
		localSkillpack,
		pack,
		client,
		os.Stdout,
	); err != nil {
		log.Fatalf("evaluation: %v", err)
	}
}

// runBoardPack emits the annotation dataset. It reports no verdict, so a
// non-zero exit here means the run did not happen rather than that Deep failed.
func runBoardPack(
	definition community.Definition,
	localSkillpack string,
	packPath string,
	proxyURL string,
	proxyModel string,
	rosterPath string,
	client community.CompletionClient,
) {
	pack, err := community.LoadBoardPack(packPath)
	if err != nil {
		log.Fatalf("board pack: %v", err)
	}
	provenance := community.BoardProvenance{
		Definition:  evaluationDefinitionPath(),
		Pack:        packPath,
		Model:       proxyModel,
		Transport:   proxyURL,
		Roster:      valueOrDefault(rosterPath, "empty"),
		Epochs:      boardEpochs(),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := community.RunBoard(
		context.Background(),
		definition,
		community.PlaceholderPrincipal,
		localSkillpack,
		pack,
		provenance,
		client,
		os.Stdout,
	); err != nil {
		log.Fatalf("board: %v", err)
	}
}

// runRatePack measures intermittent behavior. A non-zero exit here means a
// case beat its declared ceiling or could not be measured at all.
func runRatePack(
	definition community.Definition,
	localSkillpack string,
	packPath string,
	proxyURL string,
	proxyModel string,
	rosterPath string,
	client community.CompletionClient,
) {
	pack, err := community.LoadRatePack(packPath)
	if err != nil {
		log.Fatalf("rate pack: %v", err)
	}
	provenance := rateProvenance(packPath, proxyURL, proxyModel, rosterPath)
	// Interrupt cancels the run so the dataset is still written. Killing the
	// process loses every attempt, which is sirens-echo#324.
	rateCtx, stopRate := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer stopRate()
	if err := community.RunRate(
		rateCtx,
		definition,
		community.PlaceholderPrincipal,
		localSkillpack,
		pack,
		provenance,
		client,
		os.Stdout,
	); err != nil {
		log.Fatalf("rate: %v", err)
	}
}

// boardEpochs repeats each case so the grader reads epoch 1 and the rest stay
// in the dataset as a failure-spread estimate.
func boardEpochs() int {
	raw := strings.TrimSpace(os.Getenv("SIRENS_ECHO_BOARD_EPOCHS"))
	if raw == "" {
		return community.DefaultBoardEpochs
	}
	epochs, err := strconv.Atoi(raw)
	if err != nil || epochs < 1 {
		log.Fatalf("SIRENS_ECHO_BOARD_EPOCHS must be a positive integer, got %q", raw)
	}
	return epochs
}

// preserveOutOfRepoPack copies a pack run from outside the repository into the
// running agent's probes/, so a committed dataset stays re-derivable. Issue 423.
func preserveOutOfRepoPack(packPath string) {
	if !filepath.IsAbs(packPath) || strings.HasPrefix(packPath, "agents/") {
		return
	}
	body, err := os.ReadFile(packPath)
	if err != nil {
		return
	}
	// Written at run time rather than at commit time, because the window where
	// the file still exists is the run itself.
	target := filepath.Join(
		filepath.Dir(evaluationDefinitionPath()), "probes", filepath.Base(packPath),
	)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		log.Printf("could not preserve %s: %v", packPath, err)
		return
	}
	if err := os.WriteFile(target, body, 0o600); err != nil {
		log.Printf("could not preserve %s: %v", packPath, err)
		return
	}
	log.Printf(
		"pack %s is outside the repository, copied to %s so a committed dataset "+
			"stays re-derivable", packPath, target,
	)
}

// warnUnservedRequiredTools names the cases that cannot pass before the run
// spends a completion on them. See sirens-echo#357.
func warnUnservedRequiredTools(cases []community.EvaluationCase, rosterPath string) {
	if rosterPath != "" || strings.TrimSpace(os.Getenv("SIRENS_ECHO_TOOL_FIXTURE")) != "" {
		return
	}
	unservable := make([]string, 0, len(cases))
	for _, evaluationCase := range cases {
		if evaluationCase.RequiredTool != "" {
			unservable = append(unservable, evaluationCase.ID+" needs "+evaluationCase.RequiredTool)
		}
	}
	if len(unservable) == 0 {
		return
	}
	// The failure reads as the model ignoring a tool it had. It had none.
	log.Printf(
		"no roster and no fixture, so these cases cannot pass and their failures "+
			"describe the run rather than the agent: %s",
		strings.Join(unservable, ", "),
	)
}

// evaluationTools serves declared results when a fixture is named, so a case
// can place a payload inside tool output. See docs/sirens-echo-testing.md.
func evaluationTools(rosterPath string, httpClient *http.Client) community.ToolProvider {
	fixturePath := strings.TrimSpace(os.Getenv("SIRENS_ECHO_TOOL_FIXTURE"))
	if fixturePath == "" {
		return &community.MCPProvider{
			Servers:    evaluationMCPServers(rosterPath),
			HTTPClient: httpClient,
		}
	}
	// A fixture replaces the roster rather than joining it. A run that reached
	// both could not say which surface answered.
	if rosterPath != "" {
		log.Fatal("SIRENS_ECHO_TOOL_FIXTURE and SIRENS_ECHO_MCP_ROSTER are exclusive")
	}
	pack, err := community.LoadFixturePack(fixturePath)
	if err != nil {
		log.Fatalf("tool fixture: %v", err)
	}
	return community.FixtureProvider{Pack: pack}
}

// evaluationMCPServers uses the deployment roster when one is named, and no
// tools otherwise, so an offline run needs no MCP endpoint or secret.
func evaluationMCPServers(path string) []community.MCPServerDefinition {
	if path == "" {
		return nil
	}
	servers, err := community.LoadMCPRoster(path)
	if err != nil {
		log.Fatalf("evaluation roster: %v", err)
	}
	return servers
}

// A case addresses its agent by name and expects that profile's tools, so the
// pack travels with the definition rather than being selected independently.
func evaluationDefinitionPath() string {
	return valueOrDefault(
		os.Getenv("SIRENS_ECHO_DEFINITION"),
		defaultEvaluationDefinition,
	)
}

func evaluationPackPath() string {
	return valueOrDefault(
		os.Getenv("SIRENS_ECHO_EVALUATION_PACK"),
		defaultEvaluationPack,
	)
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

// rateProvenance records what produced a dataset, extracted so its fields are
// covered by test. See docs/sirens-echo-rate.md.
func rateProvenance(
	packPath string,
	proxyURL string,
	proxyModel string,
	rosterPath string,
) community.RateProvenance {
	return community.RateProvenance{
		Definition: evaluationDefinitionPath(),
		Pack:       packPath,
		Model:      proxyModel,
		Transport:  proxyURL,
		Roster:     valueOrDefault(rosterPath, "empty"),
		Fixture: valueOrDefault(
			strings.TrimSpace(os.Getenv("SIRENS_ECHO_TOOL_FIXTURE")),
			community.FixtureNone,
		),
		Substrate: valueOrDefault(
			os.Getenv("SIRENS_ECHO_SUBSTRATE"),
			community.SubstrateUnrecorded,
		),
		Runner: valueOrDefault(
			valueOrDefault(
				community.BuildRevision(),
				strings.TrimSpace(os.Getenv("SIRENS_ECHO_RUNNER")),
			),
			community.RunnerUnrecorded,
		),
		Image: valueOrDefault(
			os.Getenv("SIRENS_ECHO_IMAGE"),
			community.ImageUnrecorded,
		),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}
}
