package community

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	defaultDefinitionPath = "agent/sirens-echo.json"
	defaultForgejoURL    = "https://forgejo.coilysiren.me"
	defaultTrackerOwner  = "coilyco-gaming"
	defaultTrackerRepo   = "sirens-discord-ops"
)

// DefaultAgentProxyURL is the meaningful tailnet service name used by the
// deployment. Operators can override it without changing the agent definition.
const DefaultAgentProxyURL = "http://ser8:8080"

// Definition is the source-controlled identity, model, and skillpack selection.
type Definition struct {
	Schema             string   `json:"schema"`
	Identity           string   `json:"identity"`
	Seat               string   `json:"seat"`
	Role               string   `json:"role"`
	Model              string   `json:"model"`
	Channel            string   `json:"channel"`
	MaxContextMessages int      `json:"max_context_messages"`
	SkillRoots         []string `json:"skill_roots"`
}

// Config combines the source-controlled definition with deployment secrets.
type Config struct {
	Definition       Definition
	DefinitionPath   string
	DiscordToken     string
	DiscordChannelID string
	AgentProxyURL    string
	ForgejoBaseURL   string
	ForgejoToken     string
	TrackerOwner     string
	TrackerRepo      string
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
		ForgejoBaseURL:   valueOrDefault(os.Getenv("FORGEJO_BASE_URL"), defaultForgejoURL),
		ForgejoToken:     strings.TrimSpace(os.Getenv("FORGEJO_TOKEN")),
		TrackerOwner:     valueOrDefault(os.Getenv("FORGEJO_OWNER"), defaultTrackerOwner),
		TrackerRepo:      valueOrDefault(os.Getenv("FORGEJO_REPO"), defaultTrackerRepo),
		RequestTimeout:   2 * time.Minute,
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
	if definition.Schema != "sirens-discord-ops.agent.v1" {
		return Definition{}, fmt.Errorf("unsupported agent definition schema %q", definition.Schema)
	}
	if definition.Identity == "" || definition.Seat == "" || definition.Role == "" {
		return Definition{}, fmt.Errorf("agent definition requires identity, seat, and role")
	}
	if definition.Model == "" {
		return Definition{}, fmt.Errorf("agent definition requires model")
	}
	if definition.Channel != "#bots" {
		return Definition{}, fmt.Errorf("agent definition channel must be #bots")
	}
	if definition.MaxContextMessages < 1 || definition.MaxContextMessages > 50 {
		return Definition{}, fmt.Errorf("max_context_messages must be between 1 and 50")
	}
	if len(definition.SkillRoots) == 0 {
		return Definition{}, fmt.Errorf("agent definition requires at least one skill root")
	}
	return definition, nil
}

func valueOrDefault(value, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}
