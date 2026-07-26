package community

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefinition(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "agent", "sirens-echo.json")
	definition, err := LoadDefinition(path)
	if err != nil {
		t.Fatalf("LoadDefinition: %v", err)
	}
	if definition.Identity != "Sirens Echo" {
		t.Fatalf("identity = %q", definition.Identity)
	}
	if definition.Seat != "siren community host" {
		t.Fatalf("seat = %q", definition.Seat)
	}
	if definition.Role != "community" {
		t.Fatalf("role = %q", definition.Role)
	}
	if len(definition.IntentRoutes) != 2 {
		t.Fatalf("intent routes = %#v", definition.IntentRoutes)
	}
	if definition.IntentRoutes[0].Intent != "knowledge-retrieval" ||
		definition.IntentRoutes[0].Harness != "rasa" {
		t.Fatalf("knowledge route = %#v", definition.IntentRoutes[0])
	}
	if definition.IntentRoutes[1].Intent != "conversation-management" ||
		definition.IntentRoutes[1].Harness != "elizaos" {
		t.Fatalf("conversation route = %#v", definition.IntentRoutes[1])
	}
	if definition.Channel != "#bots" {
		t.Fatalf("channel = %q", definition.Channel)
	}
	if len(definition.SkillRoots) != 1 {
		t.Fatalf("skill roots = %#v", definition.SkillRoots)
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
