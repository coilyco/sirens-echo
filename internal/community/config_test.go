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

func TestLoadConfigRequiresSelectedAgentProxyModel(t *testing.T) {
	path := filepath.Join("..", "..", "agent", "sirens-echo.json")
	t.Setenv("SIRENS_ECHO_DEFINITION", path)
	t.Setenv("DISCORD_TOKEN", "discord-token")
	t.Setenv("DISCORD_CHANNEL_ID", "bots-channel")
	t.Setenv("SIRENS_ECHO_FORGEJO_MCP_URL", "http://forgejo-mcp:8080/mcp")
	t.Setenv("AGENT_PROXY_MODEL", "")

	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "AGENT_PROXY_MODEL") {
		t.Fatalf("LoadConfig error = %v", err)
	}
}

func TestLoadConfigResolvesMCPURLFromEnvironment(t *testing.T) {
	path := filepath.Join("..", "..", "agent", "sirens-echo.json")
	t.Setenv("SIRENS_ECHO_DEFINITION", path)
	t.Setenv("DISCORD_TOKEN", "discord-token")
	t.Setenv("DISCORD_CHANNEL_ID", "bots-channel")
	t.Setenv("AGENT_PROXY_MODEL", "model")
	t.Setenv("SIRENS_ECHO_FORGEJO_MCP_URL", "http://forgejo-mcp:8080/mcp")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	url, err := mcpServerURL(cfg.Definition, "forgejo")
	if err != nil {
		t.Fatalf("mcpServerURL: %v", err)
	}
	if url != "http://forgejo-mcp:8080/mcp" {
		t.Fatalf("Forgejo MCP URL = %q", url)
	}
}

func TestLoadConfigRequiresEnvironmentBackedMCPURL(t *testing.T) {
	path := filepath.Join("..", "..", "agent", "sirens-echo.json")
	t.Setenv("SIRENS_ECHO_DEFINITION", path)
	t.Setenv("DISCORD_TOKEN", "discord-token")
	t.Setenv("DISCORD_CHANNEL_ID", "bots-channel")
	t.Setenv("AGENT_PROXY_MODEL", "model")
	t.Setenv("SIRENS_ECHO_FORGEJO_MCP_URL", "")

	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "SIRENS_ECHO_FORGEJO_MCP_URL") {
		t.Fatalf("LoadConfig error = %v", err)
	}
}
