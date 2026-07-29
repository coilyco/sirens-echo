package community

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefinitionAcceptsTrackedHarnessConfiguration(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "agent", "sirens-echo.json")
	if _, err := LoadDefinition(path); err != nil {
		t.Fatalf("LoadDefinition: %v", err)
	}
}

func TestLoadConfigDerivesModelOpaqueAgentProxyRoute(t *testing.T) {
	path := filepath.Join("..", "..", "agent", "sirens-echo.json")
	t.Setenv("SIRENS_ECHO_DEFINITION", path)
	t.Setenv("DISCORD_TOKEN", "discord-token")
	t.Setenv("DISCORD_CHANNEL_ID", "bots-channel")
	t.Setenv("FORGEJO_TOKEN", "forgejo-token")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.AgentProxyRoute != "community/conversation-management" {
		t.Fatalf("AgentProxyRoute = %q", cfg.AgentProxyRoute)
	}
	if cfg.Intent != "conversation-management" {
		t.Fatalf("Intent = %q", cfg.Intent)
	}
	if strings.Contains(cfg.AgentProxyRoute, "selected-model") {
		t.Fatalf("AgentProxyRoute exposes a physical model: %q", cfg.AgentProxyRoute)
	}
}

func TestLoadConfigAcceptsOnlyDeclaredHarnessIntents(t *testing.T) {
	path := filepath.Join("..", "..", "agent", "sirens-echo.json")
	t.Setenv("SIRENS_ECHO_DEFINITION", path)
	t.Setenv("SIRENS_ECHO_INTENT", "knowledge-retrieval")
	t.Setenv("DISCORD_TOKEN", "discord-token")
	t.Setenv("DISCORD_CHANNEL_ID", "bots-channel")
	t.Setenv("FORGEJO_TOKEN", "forgejo-token")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.AgentProxyRoute != "community/knowledge-retrieval" {
		t.Fatalf("AgentProxyRoute = %q", cfg.AgentProxyRoute)
	}

	t.Setenv("SIRENS_ECHO_INTENT", "autonomous-coding")
	if _, err := LoadConfig(); err == nil ||
		!strings.Contains(err.Error(), "not supported") {
		t.Fatalf("LoadConfig unsupported intent error = %v", err)
	}
}
