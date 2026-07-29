package community

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	defaultDefinitionPath = "agent/sirens-echo.json"
	defaultForgejoURL     = "https://forgejo.coilysiren.me"
	defaultTrackerOwner   = "coilyco-gaming"
	defaultTrackerRepo    = "sirens-discord-ops"
)

// DefaultAgentProxyURL is the meaningful tailnet service name used by the
// deployment. Operators can override it without changing the agent definition.
const DefaultAgentProxyURL = "http://ser8:8080"

// DefaultAOSBundlePath is where the production image installs the verified
// compiled Community bundle.
const DefaultAOSBundlePath = "agent/community-bundle"

// DefaultOTLPEndpoint is the existing in-cluster SigNoZ collector.
const DefaultOTLPEndpoint = "http://signoz-otel-collector.observability.svc.cluster.local:4318"

var mcpServerNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
var routePartPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// MCPServerDefinition is one model tool surface selected for Sirens Echo.
type MCPServerDefinition struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Definition is the source-controlled identity, role, and skillpack selection.
type Definition struct {
	Schema             string                `json:"schema"`
	Harness            string                `json:"harness"`
	Identity           string                `json:"identity"`
	Seat               string                `json:"seat"`
	Role               string                `json:"role"`
	Intents            []string              `json:"intents"`
	DefaultIntent      string                `json:"default_intent"`
	Channel            string                `json:"channel"`
	MaxContextMessages int                   `json:"max_context_messages"`
	LocalSkillRoots    []string              `json:"local_skill_roots"`
	MCPServers         []MCPServerDefinition `json:"mcp_servers"`
}

// Config combines the source-controlled definition with deployment secrets.
type Config struct {
	Definition       Definition
	DefinitionPath   string
	AOSBundlePath    string
	DiscordToken     string
	DiscordChannelID string
	AgentProxyURL    string
	AgentProxyRoute  string
	Intent           string
	ForgejoBaseURL   string
	ForgejoToken     string
	TrackerOwner     string
	TrackerRepo      string
	OTLPEndpoint     string
	RequestTimeout   time.Duration
}

// LoadConfig loads the Sirens Echo deployment from environment and its
// source-controlled definition. Secrets never have source defaults.
func LoadConfig() (Config, error) {
	definitionPath := valueOrDefault(os.Getenv("SIRENS_ECHO_DEFINITION"), defaultDefinitionPath)
	definition, err := LoadDefinition(definitionPath)
	if err != nil {
		return Config{}, err
	}
	intent := valueOrDefault(
		os.Getenv("SIRENS_ECHO_INTENT"),
		definition.DefaultIntent,
	)
	route, err := definition.LogicalRoute(intent)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Definition:       definition,
		DefinitionPath:   definitionPath,
		AOSBundlePath:    valueOrDefault(os.Getenv("SIRENS_ECHO_AOS_BUNDLE"), DefaultAOSBundlePath),
		DiscordToken:     strings.TrimSpace(os.Getenv("DISCORD_TOKEN")),
		DiscordChannelID: strings.TrimSpace(os.Getenv("DISCORD_CHANNEL_ID")),
		AgentProxyURL:    valueOrDefault(os.Getenv("AGENT_PROXY_URL"), DefaultAgentProxyURL),
		AgentProxyRoute:  route,
		Intent:           intent,
		ForgejoBaseURL:   valueOrDefault(os.Getenv("FORGEJO_BASE_URL"), defaultForgejoURL),
		ForgejoToken:     strings.TrimSpace(os.Getenv("FORGEJO_TOKEN")),
		TrackerOwner:     valueOrDefault(os.Getenv("FORGEJO_OWNER"), defaultTrackerOwner),
		TrackerRepo:      valueOrDefault(os.Getenv("FORGEJO_REPO"), defaultTrackerRepo),
		OTLPEndpoint:     valueOrDefault(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"), DefaultOTLPEndpoint),
		RequestTimeout:   3 * time.Minute,
	}
	missing := make([]string, 0, 3)
	if cfg.DiscordToken == "" {
		missing = append(missing, "DISCORD_TOKEN")
	}
	if cfg.DiscordChannelID == "" {
		missing = append(missing, "DISCORD_CHANNEL_ID")
	}
	if cfg.ForgejoToken == "" {
		missing = append(missing, "FORGEJO_TOKEN")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required env: %v", missing)
	}
	return cfg, nil
}

// LoadDefinition reads and validates the repository-owned agent definition.
func LoadDefinition(path string) (Definition, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, fmt.Errorf("read agent definition: %w", err)
	}
	var definition Definition
	if err := json.Unmarshal(raw, &definition); err != nil {
		return Definition{}, fmt.Errorf("parse agent definition: %w", err)
	}
	if definition.Schema != "sirens-discord-ops.agent.v3" {
		return Definition{}, fmt.Errorf("unsupported agent definition schema %q", definition.Schema)
	}
	if definition.Harness != "sirens-discord-ops" {
		return Definition{}, fmt.Errorf("agent definition harness must be sirens-discord-ops")
	}
	if definition.Identity == "" || definition.Seat == "" || definition.Role == "" {
		return Definition{}, fmt.Errorf("agent definition requires identity, seat, and role")
	}
	if definition.Role != "community" {
		return Definition{}, fmt.Errorf("agent definition role must be community")
	}
	if len(definition.Intents) == 0 {
		return Definition{}, fmt.Errorf("agent definition requires at least one intent")
	}
	seenIntents := make(map[string]struct{}, len(definition.Intents))
	for _, intent := range definition.Intents {
		if !routePartPattern.MatchString(intent) {
			return Definition{}, fmt.Errorf("invalid harness intent %q", intent)
		}
		if _, exists := seenIntents[intent]; exists {
			return Definition{}, fmt.Errorf("duplicate harness intent %q", intent)
		}
		seenIntents[intent] = struct{}{}
	}
	if _, exists := seenIntents[definition.DefaultIntent]; !exists {
		return Definition{}, fmt.Errorf("default_intent must be a declared harness intent")
	}
	if definition.Channel != "#bots" {
		return Definition{}, fmt.Errorf("agent definition channel must be #bots")
	}
	if definition.MaxContextMessages < 1 || definition.MaxContextMessages > 50 {
		return Definition{}, fmt.Errorf("max_context_messages must be between 1 and 50")
	}
	if len(definition.LocalSkillRoots) == 0 {
		return Definition{}, fmt.Errorf("agent definition requires at least one local skill root")
	}
	if len(definition.MCPServers) == 0 {
		return Definition{}, fmt.Errorf("agent definition requires at least one MCP server")
	}
	seenServers := make(map[string]struct{}, len(definition.MCPServers))
	for _, server := range definition.MCPServers {
		if !mcpServerNamePattern.MatchString(server.Name) {
			return Definition{}, fmt.Errorf("invalid MCP server name %q", server.Name)
		}
		if _, exists := seenServers[server.Name]; exists {
			return Definition{}, fmt.Errorf("duplicate MCP server %q", server.Name)
		}
		seenServers[server.Name] = struct{}{}
		parsed, err := url.Parse(server.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return Definition{}, fmt.Errorf("MCP server %q has invalid URL", server.Name)
		}
	}
	return definition, nil
}

// LogicalRoute returns the model-opaque Agent Proxy key for a supported lane.
func (d Definition) LogicalRoute(intent string) (string, error) {
	for _, supported := range d.Intents {
		if intent == supported {
			return d.Role + "/" + intent, nil
		}
	}
	return "", fmt.Errorf("intent %q is not supported by harness %s", intent, d.Harness)
}

func valueOrDefault(value, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}
