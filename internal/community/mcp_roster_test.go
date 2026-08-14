package community

import (
	"os"
	"path/filepath"
	"testing"
)

func writeRoster(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write roster: %v", err)
	}
	return path
}

func TestLoadMCPRosterReadsTheSharedMCPServersShape(t *testing.T) {
	t.Setenv("ROSTER_TEST_FORGEJO", "http://forgejo-mcp:8080/mcp")
	// The shape mcporter, Claude Code, and Codex already use, including keys
	// Echo does not read. An unknown key is another tool's extension.
	path := writeRoster(t, `{
	  "imports": [],
	  "mcpServers": {
	    "eco": {"baseUrl": "https://eco-app.example/mcp", "description": "eco"},
	    "forgejo": {"url": "${ROSTER_TEST_FORGEJO}", "x-codex": {"a": 1}},
	    "local": {"command": "/usr/bin/mcp", "args": ["--flag"],
	              "env": {"SOME_TOKEN": "${ROSTER_TEST_FORGEJO}"}}
	  }
	}`)

	servers, err := LoadMCPRoster(path)
	if err != nil {
		t.Fatalf("LoadMCPRoster: %v", err)
	}
	if len(servers) != 3 {
		t.Fatalf("servers = %#v", servers)
	}
	// Sorted, because map order is random and the roster decides tool order.
	if servers[0].Name != "eco" || servers[1].Name != "forgejo" || servers[2].Name != "local" {
		t.Fatalf("order = %#v", servers)
	}
	if servers[0].ResolvedTransport() != MCPTransportStreamable {
		t.Fatalf("eco transport = %q", servers[0].ResolvedTransport())
	}
	// url is accepted alongside baseUrl, and ${VAR} resolves from the process.
	if servers[1].URL != "http://forgejo-mcp:8080/mcp" {
		t.Fatalf("forgejo url = %q", servers[1].URL)
	}
	if servers[2].ResolvedTransport() != MCPTransportStdio {
		t.Fatalf("local transport = %q", servers[2].ResolvedTransport())
	}
	if servers[2].Env["SOME_TOKEN"] != "http://forgejo-mcp:8080/mcp" {
		t.Fatalf("local env = %#v", servers[2].Env)
	}
}

// The deployment roster converts to YAML one lane at a time, so both forms
// have to yield the same servers while the conversion is in flight.
func TestLoadMCPRosterReadsYAMLAndJSONIdentically(t *testing.T) {
	t.Setenv("ROSTER_TEST_FORGEJO", "http://forgejo-mcp:8080/mcp")
	jsonPath := writeRoster(t, `{
	  "mcpServers": {
	    "forgejo": {"url": "${ROSTER_TEST_FORGEJO}"},
	    "steam": {"url": "http://steam-mcp:9112/mcp"}
	  }
	}`)
	yamlPath := writeRoster(t, `mcpServers:
  forgejo:
    url: ${ROSTER_TEST_FORGEJO}
  steam:
    url: http://steam-mcp:9112/mcp
`)

	fromJSON, err := LoadMCPRoster(jsonPath)
	if err != nil {
		t.Fatalf("LoadMCPRoster(json): %v", err)
	}
	fromYAML, err := LoadMCPRoster(yamlPath)
	if err != nil {
		t.Fatalf("LoadMCPRoster(yaml): %v", err)
	}
	if len(fromYAML) != 2 {
		t.Fatalf("yaml servers = %#v", fromYAML)
	}
	for i := range fromJSON {
		if fromJSON[i].Name != fromYAML[i].Name ||
			fromJSON[i].URL != fromYAML[i].URL ||
			fromJSON[i].ResolvedTransport() != fromYAML[i].ResolvedTransport() {
			t.Fatalf("entry %d differs: json %#v yaml %#v", i, fromJSON[i], fromYAML[i])
		}
	}
}

func TestLoadMCPRosterRejectsAnUnresolvedVariable(t *testing.T) {
	t.Setenv("ROSTER_TEST_ABSENT", "")
	path := writeRoster(t, `{"mcpServers": {"eco": {"baseUrl": "${ROSTER_TEST_ABSENT}"}}}`)

	// An unset variable expands to empty, which must name the server rather
	// than connect to nothing.
	if _, err := LoadMCPRoster(path); err == nil {
		t.Fatal("an unresolved endpoint was accepted")
	}
}

func TestLoadMCPRosterHonoursAnExplicitTransport(t *testing.T) {
	path := writeRoster(t, `{"mcpServers": {
	  "legacy": {"baseUrl": "https://legacy.example/sse", "transport": "sse"}
	}}`)

	servers, err := LoadMCPRoster(path)
	if err != nil {
		t.Fatalf("LoadMCPRoster: %v", err)
	}
	// The shared shape carries no discriminator, so this is the one key Echo
	// adds: streamable and sse are both HTTP endpoints.
	if servers[0].ResolvedTransport() != MCPTransportSSE {
		t.Fatalf("transport = %q", servers[0].ResolvedTransport())
	}
}

func TestLoadMCPRosterRejectsAnInvalidServerName(t *testing.T) {
	path := writeRoster(t, `{"mcpServers": {"Not Valid": {"baseUrl": "https://x.example/mcp"}}}`)

	if _, err := LoadMCPRoster(path); err == nil {
		t.Fatal("an invalid server name was accepted")
	}
}

func TestLoadMCPRosterExpandsHeaderValues(t *testing.T) {
	t.Setenv("ROSTER_TEST_API_KEY", "a-vendor-key-long-enough-to-guard")
	path := writeRoster(t, `{"mcpServers": {"exa": {
	  "url": "https://mcp.example/mcp",
	  "headers": {"x-api-key": "${ROSTER_TEST_API_KEY}"}
	}}}`)

	servers, err := LoadMCPRoster(path)
	if err != nil {
		t.Fatalf("LoadMCPRoster: %v", err)
	}
	// The point of the field: the credential reaches the entry without being
	// written into it, the way an endpoint variable already does.
	if got := servers[0].Headers["x-api-key"]; got != "a-vendor-key-long-enough-to-guard" {
		t.Fatalf("header = %q", got)
	}
}

func TestLoadMCPRosterRejectsAnUnresolvedHeader(t *testing.T) {
	t.Setenv("ROSTER_TEST_ABSENT_KEY", "")
	path := writeRoster(t, `{"mcpServers": {"exa": {
	  "url": "https://mcp.example/mcp",
	  "headers": {"x-api-key": "${ROSTER_TEST_ABSENT_KEY}"}
	}}}`)

	// An unset key would otherwise reach the vendor as an anonymous call and
	// fail with the vendor's error rather than naming the server.
	if _, err := LoadMCPRoster(path); err == nil {
		t.Fatal("an unresolved header was accepted")
	}
}

func TestLoadMCPRosterRejectsHeadersOnStdio(t *testing.T) {
	path := writeRoster(t, `{"mcpServers": {"local": {
	  "command": "./server",
	  "headers": {"x-api-key": "value"}
	}}}`)

	// A stdio child takes env, not headers, the same way an HTTP entry takes
	// headers and not env.
	if _, err := LoadMCPRoster(path); err == nil {
		t.Fatal("headers on a stdio entry were accepted")
	}
}
