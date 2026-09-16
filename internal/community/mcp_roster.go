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
	// belongs. See docs/sirens-echo-mcp.md.
	Headers map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
}

// MCPRosterReport is what a roster check found. Servers is populated whether or
// not Issues is: a reviewer reading one bad name still wants the rest.
type MCPRosterReport struct {
	Servers []MCPServerDefinition
	Issues  []error
	// The ${VAR} entries no value was supplied for. docs/sirens-echo-mcp.md.
	Unresolved []string
}

// LoadMCPRoster reads the deployment-owned MCP inventory, stopping at the first
// invalid entry. CheckMCPRoster is the same rules reported in full, for CI.
func LoadMCPRoster(path string) ([]MCPServerDefinition, error) {
	report, err := readMCPRoster(path, os.Getenv)
	if err != nil {
		return nil, err
	}
	if len(report.Issues) > 0 {
		return nil, report.Issues[0]
	}
	return report.Servers, nil
}

// CheckMCPRoster reports every invalid entry rather than the first. The
// placeholder below is load-bearing: docs/sirens-echo-mcp.md.
func CheckMCPRoster(path string) (*MCPRosterReport, error) {
	var unresolved []string
	seen := map[string]bool{}
	lookup := func(key string) string {
		if value := os.Getenv(key); value != "" {
			return value
		}
		if !seen[key] {
			seen[key] = true
			unresolved = append(unresolved, key)
		}
		// Valid as a URL and a command, so value-independent rules still run.
		// `.invalid` is reserved by RFC 2606 and resolves nowhere.
		return "https://unset.invalid/" + key
	}
	report, err := readMCPRoster(path, lookup)
	if err != nil {
		return nil, err
	}
	sort.Strings(unresolved)
	report.Unresolved = unresolved
	return report, nil
}

// readMCPRoster is the one implementation of the rules, so a rule added here
// reaches the runtime and the check together. docs/sirens-echo-mcp.md.
func readMCPRoster(path string, lookup func(string) string) (*MCPRosterReport, error) {
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
	report := &MCPRosterReport{Servers: make([]MCPServerDefinition, 0, len(names))}
	for _, name := range names {
		server := file.MCPServers[name].definition(name, lookup)
		if !mcpServerNamePattern.MatchString(server.Name) {
			report.Issues = append(report.Issues, fmt.Errorf(
				"invalid MCP server name %q: names match %s, so an underscore "+
					"or a capital is rejected", server.Name, mcpServerNamePattern,
			))
			continue
		}
		if err := validateMCPServer(server); err != nil {
			report.Issues = append(report.Issues, err)
			continue
		}
		report.Servers = append(report.Servers, server)
	}
	return report, nil
}

func (e rosterEntry) definition(name string, lookup func(string) string) MCPServerDefinition {
	endpoint := e.BaseURL
	if endpoint == "" {
		endpoint = e.URL
	}
	server := MCPServerDefinition{
		Name:      name,
		Transport: e.Transport,
		URL:       expandRoster(endpoint, lookup),
		Command:   expandRoster(e.Command, lookup),
	}
	for _, arg := range e.Args {
		server.Args = append(server.Args, expandRoster(arg, lookup))
	}
	if len(e.Env) > 0 {
		server.Env = make(map[string]string, len(e.Env))
		for key, value := range e.Env {
			server.Env[key] = expandRoster(value, lookup)
		}
	}
	if len(e.Headers) > 0 {
		server.Headers = make(map[string]string, len(e.Headers))
		for key, value := range e.Headers {
			server.Headers[key] = expandRoster(value, lookup)
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

// expandRoster resolves ${VAR} through the caller's lookup: os.Getenv for the
// runtime, a placeholder for a check. docs/sirens-echo-mcp.md.
func expandRoster(value string, lookup func(string) string) string {
	return rosterVariable.ReplaceAllStringFunc(value, func(match string) string {
		return lookup(rosterVariable.FindStringSubmatch(match)[1])
	})
}
