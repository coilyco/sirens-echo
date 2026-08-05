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
	defaultHTTPListenAddr = "127.0.0.1:8080"
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
	Channel            string                `json:"channel"`
	MaxContextMessages int                   `json:"max_context_messages"`
	LocalSkillRoots    []string              `json:"local_skill_roots"`
	MCPServers         []MCPServerDefinition `json:"mcp_servers"`
}

// Config combines the source-controlled definition with deployment secrets.
type Config struct {
	Definition       Definition
	DefinitionPath   string
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
	cfg := Config{
		Definition:       definition,
		DefinitionPath:   definitionPath,
		DiscordToken:     strings.TrimSpace(os.Getenv("DISCORD_TOKEN")),
		DiscordChannelID: strings.TrimSpace(os.Getenv("DISCORD_CHANNEL_ID")),
		AgentProxyURL:    valueOrDefault(os.Getenv("AGENT_PROXY_URL"), DefaultAgentProxyURL),
		AgentProxyModel:  strings.TrimSpace(os.Getenv("AGENT_PROXY_MODEL")),
		OTLPEndpoint:     valueOrDefault(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"), DefaultOTLPEndpoint),
		HTTPListenAddr:   valueOrDefault(os.Getenv("SIRENS_ECHO_HTTP_ADDR"), defaultHTTPListenAddr),
		RequestTimeout:   3 * time.Minute,
	}
	missing := make([]string, 0, 4)
	if cfg.DiscordToken == "" {
		missing = append(missing, "DISCORD_TOKEN")
	}
	if cfg.DiscordChannelID == "" {
		missing = append(missing, "DISCORD_CHANNEL_ID")
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
	if definition.Schema != "sirens-discord-ops.agent.v4" {
		return Definition{}, fmt.Errorf("unsupported agent definition schema %q", definition.Schema)
	}
	if definition.Identity == "" || definition.AuditRole == "" {
		return Definition{}, fmt.Errorf("agent definition requires identity and audit_role")
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
