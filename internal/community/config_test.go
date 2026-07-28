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
	t.Setenv("FORGEJO_TOKEN", "forgejo-token")
	t.Setenv("AGENT_PROXY_MODEL", "")

	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "AGENT_PROXY_MODEL") {
		t.Fatalf("LoadConfig error = %v", err)
	}
}
