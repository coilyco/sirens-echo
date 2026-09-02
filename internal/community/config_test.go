package community

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadDefinitionAcceptsTrackedHarnessConfiguration(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"agents/echo/definition.yaml", "agents/deep/definition.yaml"} {
		path := filepath.Join("..", "..", name)
		if _, err := LoadDefinition(path); err != nil {
			t.Fatalf("LoadDefinition(%s): %v", name, err)
		}
	}
}

// The Sirens Deep profile no longer asserts an empty roster. What stays guarded
// is that every surface is named here and addressed by deployment.
func TestSirensDeepDefinitionSelectsDeploymentResolvedSurfaces(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "agents", "deep", "definition.yaml")
	definition, err := LoadDefinition(path)
	if err != nil {
		t.Fatalf("LoadDefinition: %v", err)
	}
	if definition.Identity != "Sirens Deep of Coilyco" || definition.AuditRole != "general" {
		t.Fatalf(
			"identity = %q, audit role = %q",
			definition.Identity,
			definition.AuditRole,
		)
	}
	if definition.Channel != "" {
		t.Fatalf("channel = %q", definition.Channel)
	}
	// Deep ends a turn by writing, same as Echo, and moved to Teable with it.
	// Asserted empty until Kai reversed it on 2026-08-15.
	if definition.IssueTracker != "teable" {
		t.Fatalf("issue tracker = %q", definition.IssueTracker)
	}
	// coilyco-org joined on sirens-echo#806 and is shared with Echo, which is
	// the point of it. Deep's own policy root is still the general one.
	roots := strings.Join(definition.LocalSkillRoots, " ")
	if !strings.Contains(roots, "coilyco-general") ||
		!strings.Contains(roots, "coilyco-org") {
		t.Fatalf("local skill roots = %#v", definition.LocalSkillRoots)
	}
}

func TestLoadConfigAllowsHTTPOnlyDeploymentWithoutDiscordSecrets(t *testing.T) {
	path := filepath.Join("..", "..", "agents", "deep", "definition.yaml")
	t.Setenv("SIRENS_ECHO_DEFINITION", path)
	useFixtureBundles(t, "creator")
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
	path := filepath.Join("..", "..", "agents", "deep", "definition.yaml")
	t.Setenv("SIRENS_ECHO_DEFINITION", path)
	useFixtureBundles(t, "creator")
	t.Setenv("SIRENS_ECHO_STEAM_MCP_URL", "http://sirens-deep-steam-mcp:9112/mcp")
	t.Setenv("SIRENS_ECHO_FORGEJO_MCP_URL", "http://sirens-deep-forgejo-mcp:8080/mcp")
	t.Setenv("SIRENS_ECHO_DISCORD_ENABLED", "true")
	t.Setenv("SIRENS_ECHO_INSTANCE", "sirens-deep")
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

// One signal identifies nobody, so a half-configured principal must stop the
// process rather than render a sentence with an empty half.
func TestLoadConfigRejectsAHalfConfiguredPrincipal(t *testing.T) {
	path := filepath.Join("..", "..", "agents", "deep", "definition.yaml")
	t.Setenv("SIRENS_ECHO_DEFINITION", path)
	useFixtureBundles(t, "creator")
	t.Setenv("SIRENS_ECHO_STEAM_MCP_URL", "http://sirens-deep-steam-mcp:9112/mcp")
	t.Setenv("SIRENS_ECHO_FORGEJO_MCP_URL", "http://sirens-deep-forgejo-mcp:8080/mcp")
	t.Setenv("SIRENS_ECHO_DISCORD_ENABLED", "false")
	t.Setenv("SIRENS_ECHO_INSTANCE", "sirens-deep")
	t.Setenv("AGENT_PROXY_MODEL", "model")
	t.Setenv("SIRENS_ECHO_PRINCIPAL_HANDLE", "example_handle")
	t.Setenv("SIRENS_ECHO_PRINCIPAL_USER_ID", "")

	if _, err := LoadConfig(); err == nil ||
		!strings.Contains(err.Error(), "must be set together") {
		t.Fatalf("LoadConfig error = %v", err)
	}

	t.Setenv("SIRENS_ECHO_PRINCIPAL_USER_ID", "not-a-snowflake")
	if _, err := LoadConfig(); err == nil ||
		!strings.Contains(err.Error(), "numeric snowflake") {
		t.Fatalf("LoadConfig error = %v", err)
	}

	t.Setenv("SIRENS_ECHO_PRINCIPAL_USER_ID", PlaceholderPrincipal.UserID)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.Principal.Configured() {
		t.Fatalf("Principal = %#v", cfg.Principal)
	}

	// Neither set is the supported state for a deployment that names nobody.
	t.Setenv("SIRENS_ECHO_PRINCIPAL_HANDLE", "")
	t.Setenv("SIRENS_ECHO_PRINCIPAL_USER_ID", "")
	if cfg, err = LoadConfig(); err != nil || cfg.Principal.Configured() {
		t.Fatalf("LoadConfig = %#v, %v", cfg.Principal, err)
	}
}

func TestLoadConfigRejectsChannelNamesInPlaceOfIDs(t *testing.T) {
	path := filepath.Join("..", "..", "agents", "deep", "definition.yaml")
	t.Setenv("SIRENS_ECHO_DEFINITION", path)
	useFixtureBundles(t, "creator")
	t.Setenv("SIRENS_ECHO_DISCORD_ENABLED", "true")
	t.Setenv("SIRENS_ECHO_INSTANCE", "sirens-deep")
	t.Setenv("DISCORD_TOKEN", "discord-token")
	t.Setenv("DISCORD_CHANNEL_ID", "#bots")
	t.Setenv("AGENT_PROXY_MODEL", "model")

	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "numeric snowflakes") {
		t.Fatalf("LoadConfig error = %v", err)
	}
}

func TestLoadConfigAcceptsANonLoopbackListener(t *testing.T) {
	path := filepath.Join("..", "..", "agents", "deep", "definition.yaml")
	t.Setenv("SIRENS_ECHO_DEFINITION", path)
	useFixtureBundles(t, "creator")
	t.Setenv("SIRENS_ECHO_STEAM_MCP_URL", "http://sirens-deep-steam-mcp:9112/mcp")
	t.Setenv("SIRENS_ECHO_FORGEJO_MCP_URL", "http://sirens-deep-forgejo-mcp:8080/mcp")
	t.Setenv("SIRENS_ECHO_DISCORD_ENABLED", "false")
	t.Setenv("SIRENS_ECHO_INSTANCE", "sirens-deep")
	t.Setenv("AGENT_PROXY_MODEL", "model")
	// The bind address no longer gates startup. Reachability is decided at the
	// network layer, so the process does not reason about it.
	t.Setenv("SIRENS_ECHO_HTTP_ADDR", "0.0.0.0:8080")

	if _, err := LoadConfig(); err != nil {
		t.Fatalf("LoadConfig on a non-loopback listener: %v", err)
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
	path := filepath.Join("..", "..", "agents", "echo", "definition.yaml")
	t.Setenv("SIRENS_ECHO_DEFINITION", path)
	useFixtureBundles(t, "ops")
	t.Setenv("SIRENS_ECHO_DISCORD_ENABLED", "sometimes")

	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "must be true or false") {
		t.Fatalf("LoadConfig error = %v", err)
	}
}

func TestLoadConfigRequiresSelectedAgentProxyModel(t *testing.T) {
	path := filepath.Join("..", "..", "agents", "echo", "definition.yaml")
	t.Setenv("SIRENS_ECHO_DEFINITION", path)
	useFixtureBundles(t, "ops")
	t.Setenv("DISCORD_TOKEN", "discord-token")
	t.Setenv("DISCORD_CHANNEL_ID", "1024000000000000001")
	t.Setenv("SIRENS_ECHO_FORGEJO_MCP_URL", "http://forgejo-mcp:8080/mcp")
	t.Setenv("AGENT_PROXY_MODEL", "")

	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "AGENT_PROXY_MODEL") {
		t.Fatalf("LoadConfig error = %v", err)
	}
}

func TestLoadConfigCarriesTheRosterPath(t *testing.T) {
	path := filepath.Join("..", "..", "agents", "echo", "definition.yaml")
	t.Setenv("SIRENS_ECHO_DEFINITION", path)
	useFixtureBundles(t, "ops")
	t.Setenv("DISCORD_TOKEN", "discord-token")
	t.Setenv("DISCORD_CHANNEL_ID", "1024000000000000001")
	t.Setenv("AGENT_PROXY_MODEL", "model")
	t.Setenv("SIRENS_ECHO_MCP_ROSTER", "/etc/sirens-echo/mcp.json")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.MCPRosterPath != "/etc/sirens-echo/mcp.json" {
		t.Fatalf("roster path = %q", cfg.MCPRosterPath)
	}
}

func TestLoadConfigAcceptsNoRoster(t *testing.T) {
	path := filepath.Join("..", "..", "agents", "echo", "definition.yaml")
	t.Setenv("SIRENS_ECHO_DEFINITION", path)
	useFixtureBundles(t, "ops")
	t.Setenv("DISCORD_TOKEN", "discord-token")
	t.Setenv("DISCORD_CHANNEL_ID", "1024000000000000001")
	t.Setenv("AGENT_PROXY_MODEL", "model")
	t.Setenv("SIRENS_ECHO_MCP_ROSTER", "")

	// An absent roster is a valid no-tool boundary. Echo names no server, so it
	// cannot require one either.
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig without a roster: %v", err)
	}
	if cfg.MCPRosterPath != "" {
		t.Fatalf("roster path = %q", cfg.MCPRosterPath)
	}
}

func TestValidateMCPServerChecksShapePerTransport(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name   string
		server MCPServerDefinition
		valid  bool
	}{
		{
			name:   "empty transport defaults to streamable",
			server: MCPServerDefinition{Name: "eco", URL: "https://eco:9000/mcp"},
			valid:  true,
		},
		{
			name: "sse takes a url",
			server: MCPServerDefinition{
				Name: "eco", Transport: MCPTransportSSE, URL: "https://eco:9000/sse",
			},
			valid: true,
		},
		{
			name: "stdio takes a command",
			server: MCPServerDefinition{
				Name: "local", Transport: MCPTransportStdio, Command: "/usr/bin/mcp",
			},
			valid: true,
		},
		{
			name: "stdio forwards named env",
			server: MCPServerDefinition{
				Name: "local", Transport: MCPTransportStdio,
				Command: "/usr/bin/mcp", Env: map[string]string{"SOME_TOKEN": "value"},
			},
			valid: true,
		},
		{
			name:   "stdio without a command",
			server: MCPServerDefinition{Name: "local", Transport: MCPTransportStdio},
		},
		{
			name: "stdio carrying a url",
			server: MCPServerDefinition{
				Name: "local", Transport: MCPTransportStdio,
				Command: "/usr/bin/mcp", URL: "https://eco:9000/mcp",
			},
		},
		{
			name: "stdio with an invalid env name",
			server: MCPServerDefinition{
				Name: "local", Transport: MCPTransportStdio,
				Command: "/usr/bin/mcp", Env: map[string]string{"not-an-env-name": "value"},
			},
		},
		{
			name: "url transport carrying a command",
			server: MCPServerDefinition{
				Name: "eco", URL: "https://eco:9000/mcp", Command: "/usr/bin/mcp",
			},
		},
		{
			name:   "url transport with no baseUrl",
			server: MCPServerDefinition{Name: "eco"},
		},
		{
			name: "unsupported transport",
			server: MCPServerDefinition{
				Name: "eco", Transport: "carrier-pigeon", URL: "https://eco:9000/mcp",
			},
		},
	} {
		err := validateMCPServer(testCase.server)
		if testCase.valid && err != nil {
			t.Errorf("%s: unexpected error %v", testCase.name, err)
		}
		if !testCase.valid && err == nil {
			t.Errorf("%s: expected an error", testCase.name)
		}
	}
}
