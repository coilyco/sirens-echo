package community

import (
	"fmt"
	"os"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"
)

// rosterVariable matches the ${VAR} interpolation the shared mcpServers format
// already uses, so a secret reaches an entry without being written into it.
var rosterVariable = regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*)\}`)

// rosterFile is the deployment-owned inventory, in the mcpServers shape shared
// with mcporter, Claude Code, and Codex. Unknown keys are ignored.
type rosterFile struct {
	MCPServers map[string]rosterEntry `json:"mcpServers" yaml:"mcpServers"`
}

type rosterEntry struct {
	Transport string            `json:"transport,omitempty" yaml:"transport,omitempty"`
	BaseURL   string            `json:"baseUrl,omitempty" yaml:"baseUrl,omitempty"`
	URL       string            `json:"url,omitempty" yaml:"url,omitempty"`
	Command   string            `json:"command,omitempty" yaml:"command,omitempty"`
	Args      []string          `json:"args,omitempty" yaml:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	// Headers reaches an authenticated hosted MCP, and is where a credential
	// belongs. See docs/sirens-echo-mcp-roster-auth.md.
	Headers map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
}

// LoadMCPRoster reads the deployment-owned MCP inventory. YAML, and JSON is a
// subset of it, so a roster still written as JSON parses unchanged.
func LoadMCPRoster(path string) ([]MCPServerDefinition, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read MCP roster: %w", err)
	}
	var file rosterFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse MCP roster: %w", err)
	}
	// A supplied path resolving to nothing is a mistake every time. The genuine
	// no-tool case supplies no path at all. See sirens-echo#684.
	if len(file.MCPServers) == 0 {
		return nil, fmt.Errorf(
			"MCP roster %s names no servers: check the file is the roster itself "+
				"rather than the ConfigMap that carries it", path,
		)
	}
	names := make([]string, 0, len(file.MCPServers))
	for name := range file.MCPServers {
		names = append(names, name)
	}
	// Map order is random, and the roster decides tool order in the prompt.
	sort.Strings(names)
	servers := make([]MCPServerDefinition, 0, len(names))
	for _, name := range names {
		server := file.MCPServers[name].definition(name)
		if !mcpServerNamePattern.MatchString(server.Name) {
			return nil, fmt.Errorf("invalid MCP server name %q", server.Name)
		}
		if err := validateMCPServer(server); err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}
	return servers, nil
}

func (e rosterEntry) definition(name string) MCPServerDefinition {
	endpoint := e.BaseURL
	if endpoint == "" {
		endpoint = e.URL
	}
	server := MCPServerDefinition{
		Name:      name,
		Transport: e.Transport,
		URL:       expandRoster(endpoint),
		Command:   expandRoster(e.Command),
	}
	for _, arg := range e.Args {
		server.Args = append(server.Args, expandRoster(arg))
	}
	if len(e.Env) > 0 {
		server.Env = make(map[string]string, len(e.Env))
		for key, value := range e.Env {
			server.Env[key] = expandRoster(value)
		}
	}
	if len(e.Headers) > 0 {
		server.Headers = make(map[string]string, len(e.Headers))
		for key, value := range e.Headers {
			server.Headers[key] = expandRoster(value)
		}
	}
	if server.Transport == "" {
		// The shared format carries no discriminator, so a command means stdio
		// and an endpoint means HTTP. `transport` only disambiguates sse.
		server.Transport = MCPTransportStreamable
		if server.Command != "" {
			server.Transport = MCPTransportStdio
		}
	}
	return server
}

// expandRoster resolves ${VAR} from Echo's environment. An unset variable
// expands to empty, which validation then reports against the named server.
func expandRoster(value string) string {
	return rosterVariable.ReplaceAllStringFunc(value, func(match string) string {
		return os.Getenv(rosterVariable.FindStringSubmatch(match)[1])
	})
}
