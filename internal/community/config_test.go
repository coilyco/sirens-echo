package community

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefinitionAcceptsTrackedHarnessConfiguration(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"sirens-echo.yaml", "sirens-deep.yaml"} {
		path := filepath.Join("..", "..", "agent", name)
		if _, err := LoadDefinition(path); err != nil {
			t.Fatalf("LoadDefinition(%s): %v", name, err)
		}
	}
}

func TestCoilyCoDefinitionHasNoDomainSpecificSurface(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "agent", "sirens-deep.yaml")
	definition, err := LoadDefinition(path)
	if err != nil {
		t.Fatalf("LoadDefinition: %v", err)
	}
	if definition.Identity != "CoilyCo" || definition.AuditRole != "general" {
		t.Fatalf(
			"identity = %q, audit role = %q",
			definition.Identity,
			definition.AuditRole,
		)
	}
	if definition.Channel != "" {
		t.Fatalf("channel = %q", definition.Channel)
	}
	if len(definition.MCPServers) != 0 {
		t.Fatalf("MCP servers = %#v", definition.MCPServers)
	}
	if definition.IssueTracker != "" {
		t.Fatalf("issue tracker = %q", definition.IssueTracker)
	}
	if len(definition.LocalSkillRoots) != 1 ||
		!strings.HasSuffix(definition.LocalSkillRoots[0], "coilyco-general") {
		t.Fatalf("local skill roots = %#v", definition.LocalSkillRoots)
	}
}

func TestLoadConfigAllowsHTTPOnlyDeploymentWithoutDiscordSecrets(t *testing.T) {
	path := filepath.Join("..", "..", "agent", "sirens-deep.yaml")
	t.Setenv("SIRENS_ECHO_DEFINITION", path)
	t.Setenv("SIRENS_ECHO_DISCORD_ENABLED", "false")
	t.Setenv("SIRENS_ECHO_INSTANCE", "sirens-deep")
	t.Setenv("DISCORD_TOKEN", "")
	t.Setenv("DISCORD_CHANNEL_ID", "")
	t.Setenv("AGENT_PROXY_MODEL", "sirens-echo/deepseek")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.DiscordEnabled {
		t.Fatal("DiscordEnabled = true")
	}
	if cfg.InstanceName != "sirens-deep" {
		t.Fatalf("InstanceName = %q", cfg.InstanceName)
	}
}

func TestLoadConfigRejectsDiscordWithHTTPOnlyDefinition(t *testing.T) {
	path := filepath.Join("..", "..", "agent", "sirens-deep.yaml")
	t.Setenv("SIRENS_ECHO_DEFINITION", path)
	t.Setenv("SIRENS_ECHO_DISCORD_ENABLED", "true")
	t.Setenv("DISCORD_TOKEN", "discord-token")
	t.Setenv("DISCORD_CHANNEL_ID", "bots-channel")
	t.Setenv("AGENT_PROXY_MODEL", "model")

	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "channel must be #bots") {
		t.Fatalf("LoadConfig error = %v", err)
	}
}

func TestLoadConfigRejectsInvalidDiscordSwitch(t *testing.T) {
	path := filepath.Join("..", "..", "agent", "sirens-echo.yaml")
	t.Setenv("SIRENS_ECHO_DEFINITION", path)
	t.Setenv("SIRENS_ECHO_DISCORD_ENABLED", "sometimes")

	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "must be true or false") {
		t.Fatalf("LoadConfig error = %v", err)
	}
}

func TestLoadConfigRequiresSelectedAgentProxyModel(t *testing.T) {
	path := filepath.Join("..", "..", "agent", "sirens-echo.yaml")
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
	path := filepath.Join("..", "..", "agent", "sirens-echo.yaml")
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
	path := filepath.Join("..", "..", "agent", "sirens-echo.yaml")
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
