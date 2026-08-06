package community

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultDefinitionPath = "agent/sirens-echo.json"
	defaultHTTPListenAddr = "127.0.0.1:8080"
	defaultInstanceName   = "sirens-echo"

	ResponseStyleNeutral = "neutral"
	ResponseStyleSocial  = "social"
)

// DefaultAgentProxyURL is the meaningful tailnet service name used by the
// deployment. Operators can override it without changing the agent definition.
const DefaultAgentProxyURL = "http://ser8:8080"

// DefaultOTLPEndpoint is the existing in-cluster SigNoZ collector.
const DefaultOTLPEndpoint = "http://signoz-otel-collector.observability.svc.cluster.local:4318"

var (
	mcpServerNamePattern   = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	environmentNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
)

// MCPServerDefinition is one model tool surface selected for Sirens Echo.
type MCPServerDefinition struct {
	Name   string `json:"name"`
	URL    string `json:"url,omitempty"`
	URLEnv string `json:"url_env,omitempty"`
}

// Definition is the source-controlled attribution, route, and policy selection.
type Definition struct {
	Schema             string                `json:"schema"`
	Identity           string                `json:"identity"`
	AuditRole          string                `json:"audit_role"`
	ResponseStyle      string                `json:"response_style"`
	Channel            string                `json:"channel"`
	MaxContextMessages int                   `json:"max_context_messages"`
	LocalSkillRoots    []string              `json:"local_skill_roots"`
	MCPServers         []MCPServerDefinition `json:"mcp_servers"`
	IssueTracker       string                `json:"issue_tracker,omitempty"`
}

// Config combines the source-controlled definition with deployment secrets.
type Config struct {
	Definition       Definition
	DefinitionPath   string
	InstanceName     string
	DiscordEnabled   bool
	DiscordToken     string
	DiscordChannelID string
	AgentProxyURL    string
	AgentProxyModel  string
	OTLPEndpoint     string
	HTTPListenAddr   string
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
	discordEnabled, err := boolOrDefault(os.Getenv("SIRENS_ECHO_DISCORD_ENABLED"), true)
	if err != nil {
		return Config{}, fmt.Errorf("SIRENS_ECHO_DISCORD_ENABLED: %w", err)
	}
	cfg := Config{
		Definition:       definition,
		DefinitionPath:   definitionPath,
		InstanceName:     valueOrDefault(os.Getenv("SIRENS_ECHO_INSTANCE"), defaultInstanceName),
		DiscordEnabled:   discordEnabled,
		DiscordToken:     strings.TrimSpace(os.Getenv("DISCORD_TOKEN")),
		DiscordChannelID: strings.TrimSpace(os.Getenv("DISCORD_CHANNEL_ID")),
		AgentProxyURL:    valueOrDefault(os.Getenv("AGENT_PROXY_URL"), DefaultAgentProxyURL),
		AgentProxyModel:  strings.TrimSpace(os.Getenv("AGENT_PROXY_MODEL")),
		OTLPEndpoint:     valueOrDefault(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"), DefaultOTLPEndpoint),
		HTTPListenAddr:   valueOrDefault(os.Getenv("SIRENS_ECHO_HTTP_ADDR"), defaultHTTPListenAddr),
		RequestTimeout:   3 * time.Minute,
	}
	if !mcpServerNamePattern.MatchString(cfg.InstanceName) {
		return Config{}, fmt.Errorf("SIRENS_ECHO_INSTANCE must be a lowercase service name")
	}
	if cfg.DiscordEnabled && cfg.Definition.Channel != "#bots" {
		return Config{}, fmt.Errorf("Discord-enabled definition channel must be #bots")
	}
	missing := make([]string, 0, 5)
	if cfg.DiscordEnabled {
		if cfg.DiscordToken == "" {
			missing = append(missing, "DISCORD_TOKEN")
		}
		if cfg.DiscordChannelID == "" {
			missing = append(missing, "DISCORD_CHANNEL_ID")
		}
	}
	if cfg.AgentProxyModel == "" {
		missing = append(missing, "AGENT_PROXY_MODEL")
	}
	for index := range cfg.Definition.MCPServers {
		server := &cfg.Definition.MCPServers[index]
		if server.URLEnv == "" {
			continue
		}
		server.URL = strings.TrimSpace(os.Getenv(server.URLEnv))
		if server.URL == "" {
			missing = append(missing, server.URLEnv)
			continue
		}
		if !validHTTPURL(server.URL) {
			return Config{}, fmt.Errorf("MCP server %q has invalid URL from %s", server.Name, server.URLEnv)
		}
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
	if definition.Schema != "coilyco-harness.agent.v1" {
		return Definition{}, fmt.Errorf("unsupported agent definition schema %q", definition.Schema)
	}
	if definition.Identity == "" || definition.AuditRole == "" {
		return Definition{}, fmt.Errorf("agent definition requires identity and audit_role")
	}
	if definition.Channel != "" && definition.Channel != "#bots" {
		return Definition{}, fmt.Errorf("agent definition channel must be empty or #bots")
	}
	if definition.ResponseStyle != ResponseStyleNeutral &&
		definition.ResponseStyle != ResponseStyleSocial {
		return Definition{}, fmt.Errorf("unsupported response_style %q", definition.ResponseStyle)
	}
	if definition.MaxContextMessages < 1 || definition.MaxContextMessages > 50 {
		return Definition{}, fmt.Errorf("max_context_messages must be between 1 and 50")
	}
	if len(definition.LocalSkillRoots) == 0 {
		return Definition{}, fmt.Errorf("agent definition requires at least one local skill root")
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
		hasURL := strings.TrimSpace(server.URL) != ""
		hasURLEnv := strings.TrimSpace(server.URLEnv) != ""
		if hasURL == hasURLEnv {
			return Definition{}, fmt.Errorf("MCP server %q requires exactly one of url or url_env", server.Name)
		}
		if hasURL && !validHTTPURL(server.URL) {
			return Definition{}, fmt.Errorf("MCP server %q has invalid URL", server.Name)
		}
		if hasURLEnv && !environmentNamePattern.MatchString(server.URLEnv) {
			return Definition{}, fmt.Errorf("MCP server %q has invalid url_env", server.Name)
		}
	}
	if definition.IssueTracker != "" {
		if _, exists := seenServers[definition.IssueTracker]; !exists {
			return Definition{}, fmt.Errorf(
				"issue_tracker %q does not name a configured MCP server",
				definition.IssueTracker,
			)
		}
	}
	return definition, nil
}

func validHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func valueOrDefault(value, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}

func boolOrDefault(value string, fallback bool) (bool, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(trimmed)
	if err != nil {
		return false, fmt.Errorf("must be true or false")
	}
	return parsed, nil
}
