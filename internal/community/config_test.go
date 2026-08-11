package community

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// The CoilyCo profile no longer asserts an empty roster. What stays guarded is
// that every surface is named here and addressed by deployment.
func TestCoilyCoDefinitionSelectsDeploymentResolvedSurfaces(t *testing.T) {
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
	want := map[string]string{
		"steam":   "SIRENS_ECHO_STEAM_MCP_URL",
		"forgejo": "SIRENS_ECHO_FORGEJO_MCP_URL",
	}
	if len(definition.MCPServers) != len(want) {
		t.Fatalf("MCP servers = %#v", definition.MCPServers)
	}
	for _, server := range definition.MCPServers {
		// Deployment owns each address, so a literal URL would pin one cluster.
		if server.URLEnv != want[server.Name] || server.URL != "" {
			t.Fatalf("server = %#v", server)
		}
	}
	// Forgejo carries bounded writes, so the automatic tracker stays absent.
	// See docs/sirens-echo-tools.md.
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
	t.Setenv("SIRENS_ECHO_STEAM_MCP_URL", "http://sirens-deep-steam-mcp:9112/mcp")
	t.Setenv("SIRENS_ECHO_FORGEJO_MCP_URL", "http://sirens-deep-forgejo-mcp:8080/mcp")
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

// The CoilyCo definition names no channel, yet must still be deployable to
// Discord, which the previous #bots requirement prevented outright.
func TestLoadConfigAllowsDiscordWithChannelNeutralDefinition(t *testing.T) {
	path := filepath.Join("..", "..", "agent", "sirens-deep.yaml")
	t.Setenv("SIRENS_ECHO_DEFINITION", path)
	t.Setenv("SIRENS_ECHO_STEAM_MCP_URL", "http://sirens-deep-steam-mcp:9112/mcp")
	t.Setenv("SIRENS_ECHO_FORGEJO_MCP_URL", "http://sirens-deep-forgejo-mcp:8080/mcp")
	t.Setenv("SIRENS_ECHO_DISCORD_ENABLED", "true")
	t.Setenv("DISCORD_TOKEN", "discord-token")
	t.Setenv("DISCORD_CHANNEL_ID", "1024000000000000001,1024000000000000002")
	t.Setenv("DISCORD_GUILD_IDS", "2048000000000000001")
	t.Setenv("AGENT_PROXY_MODEL", "model")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.DiscordChannelIDs) != 2 {
		t.Fatalf("DiscordChannelIDs = %#v", cfg.DiscordChannelIDs)
	}
	if len(cfg.DiscordGuildIDs) != 1 {
		t.Fatalf("DiscordGuildIDs = %#v", cfg.DiscordGuildIDs)
	}
	if cfg.DiscordDMEnabled {
		t.Fatal("direct messages must stay opt-in")
	}
}

func TestLoadConfigRejectsChannelNamesInPlaceOfIDs(t *testing.T) {
	path := filepath.Join("..", "..", "agent", "sirens-deep.yaml")
	t.Setenv("SIRENS_ECHO_DEFINITION", path)
	t.Setenv("SIRENS_ECHO_DISCORD_ENABLED", "true")
	t.Setenv("DISCORD_TOKEN", "discord-token")
	t.Setenv("DISCORD_CHANNEL_ID", "#bots")
	t.Setenv("AGENT_PROXY_MODEL", "model")

	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "numeric snowflakes") {
		t.Fatalf("LoadConfig error = %v", err)
	}
}

func TestLoadConfigRequiresHTTPTokenOffLoopback(t *testing.T) {
	path := filepath.Join("..", "..", "agent", "sirens-deep.yaml")
	t.Setenv("SIRENS_ECHO_DEFINITION", path)
	t.Setenv("SIRENS_ECHO_STEAM_MCP_URL", "http://sirens-deep-steam-mcp:9112/mcp")
	t.Setenv("SIRENS_ECHO_FORGEJO_MCP_URL", "http://sirens-deep-forgejo-mcp:8080/mcp")
	t.Setenv("SIRENS_ECHO_DISCORD_ENABLED", "false")
	t.Setenv("AGENT_PROXY_MODEL", "model")
	t.Setenv("SIRENS_ECHO_HTTP_ADDR", "0.0.0.0:8080")
	t.Setenv("SIRENS_ECHO_HTTP_TOKEN", "")

	if _, err := LoadConfig(); err == nil ||
		!strings.Contains(err.Error(), "SIRENS_ECHO_HTTP_TOKEN is required") {
		t.Fatalf("LoadConfig error = %v", err)
	}

	t.Setenv("SIRENS_ECHO_HTTP_TOKEN", "shared-secret")
	if _, err := LoadConfig(); err != nil {
		t.Fatalf("LoadConfig with token: %v", err)
	}

	// Loopback keeps the token optional, so local development is unchanged.
	t.Setenv("SIRENS_ECHO_HTTP_ADDR", "127.0.0.1:8080")
	t.Setenv("SIRENS_ECHO_HTTP_TOKEN", "")
	if _, err := LoadConfig(); err != nil {
		t.Fatalf("LoadConfig on loopback: %v", err)
	}
}

func TestLoadRateLimitPolicyOverridesAndDisables(t *testing.T) {
	t.Setenv("SIRENS_ECHO_RATE_USER", "7/45s")
	t.Setenv("SIRENS_ECHO_RATE_CONTEXT", "off")
	t.Setenv("SIRENS_ECHO_MAX_PENDING", "3")

	policy, err := loadRateLimitPolicy()
	if err != nil {
		t.Fatalf("loadRateLimitPolicy: %v", err)
	}
	if policy.PerUser.Burst != 7 || policy.PerUser.Every != 45*time.Second {
		t.Fatalf("PerUser = %#v", policy.PerUser)
	}
	if policy.PerContext.enabled() {
		t.Fatalf("PerContext = %#v, want disabled", policy.PerContext)
	}
	if !policy.Global.enabled() {
		t.Fatal("an unset tier must keep its packaged default")
	}
	if policy.MaxPending != 3 {
		t.Fatalf("MaxPending = %d", policy.MaxPending)
	}

	t.Setenv("SIRENS_ECHO_RATE_USER", "3 per minute")
	if _, err := loadRateLimitPolicy(); err == nil {
		t.Fatal("malformed rate limit was accepted")
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
	t.Setenv("DISCORD_CHANNEL_ID", "1024000000000000001")
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
	t.Setenv("DISCORD_CHANNEL_ID", "1024000000000000001")
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
	t.Setenv("DISCORD_CHANNEL_ID", "1024000000000000001")
	t.Setenv("AGENT_PROXY_MODEL", "model")
	t.Setenv("SIRENS_ECHO_FORGEJO_MCP_URL", "")

	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "SIRENS_ECHO_FORGEJO_MCP_URL") {
		t.Fatalf("LoadConfig error = %v", err)
	}
}
