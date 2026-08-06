package community

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxProxyToolNameBytes = 64

var invalidProxyToolName = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// ToolDefinition is one MCP tool translated to Agent Proxy's function schema.
type ToolDefinition struct {
	Name        string
	Server      string
	Original    string
	Description string
	InputSchema any
}

// ToolSession is the per-turn MCP capability available to Agent Proxy.
type ToolSession interface {
	Tools() []ToolDefinition
	Call(ctx context.Context, name string, arguments map[string]any) (string, error)
	Close() error
}

// ToolProvider opens the configured MCP roster for one model turn.
type ToolProvider interface {
	Open(ctx context.Context) (ToolSession, error)
}

// MCPProvider connects a definition's source-controlled MCP roster through the
// official Go SDK. An empty roster is a valid no-tool capability boundary.
type MCPProvider struct {
	Servers    []MCPServerDefinition
	HTTPClient *http.Client
}

type registeredMCPTool struct {
	serverName string
	toolName   string
	session    *mcp.ClientSession
}

type mcpToolSession struct {
	tools      []ToolDefinition
	registered map[string]registeredMCPTool
	sessions   []*mcp.ClientSession
}

// Open initializes every configured MCP server and discovers its complete tool
// list before Agent Proxy chooses a tool.
func (p MCPProvider) Open(ctx context.Context) (ToolSession, error) {
	client := mcp.NewClient(
		&mcp.Implementation{Name: "sirens-echo", Version: "1"},
		nil,
	)
	opened := &mcpToolSession{
		registered: make(map[string]registeredMCPTool),
	}
	for _, server := range p.Servers {
		session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
			Endpoint:             server.URL,
			HTTPClient:           p.HTTPClient,
			DisableStandaloneSSE: true,
		}, nil)
		if err != nil {
			_ = opened.Close()
			return nil, fmt.Errorf("connect MCP server %s: %w", server.Name, err)
		}
		opened.sessions = append(opened.sessions, session)
		cursor := ""
		for {
			result, err := session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
			if err != nil {
				_ = opened.Close()
				return nil, fmt.Errorf("list MCP tools from %s: %w", server.Name, err)
			}
			for _, tool := range result.Tools {
				name, err := proxyToolName(server.Name, tool.Name)
				if err != nil {
					_ = opened.Close()
					return nil, err
				}
				if _, exists := opened.registered[name]; exists {
					_ = opened.Close()
					return nil, fmt.Errorf("MCP tool name collision %q", name)
				}
				opened.registered[name] = registeredMCPTool{
					serverName: server.Name,
					toolName:   tool.Name,
					session:    session,
				}
				opened.tools = append(opened.tools, ToolDefinition{
					Name:        name,
					Server:      server.Name,
					Original:    tool.Name,
					Description: tool.Description,
					InputSchema: tool.InputSchema,
				})
			}
			if result.NextCursor == "" {
				break
			}
			cursor = result.NextCursor
		}
	}
	return opened, nil
}

func (s *mcpToolSession) Tools() []ToolDefinition {
	return append([]ToolDefinition(nil), s.tools...)
}

func (s *mcpToolSession) Call(
	ctx context.Context,
	name string,
	arguments map[string]any,
) (string, error) {
	tool, exists := s.registered[name]
	if !exists {
		return "", fmt.Errorf("model requested unavailable MCP tool %q", name)
	}
	result, err := tool.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      tool.toolName,
		Arguments: arguments,
	})
	if err != nil {
		return "", fmt.Errorf("call MCP tool %s/%s: %w", tool.serverName, tool.toolName, err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal MCP tool result %s/%s: %w", tool.serverName, tool.toolName, err)
	}
	return string(raw), nil
}

func (s *mcpToolSession) Close() error {
	errs := make([]error, 0, len(s.sessions))
	for _, session := range s.sessions {
		if err := session.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func proxyToolName(server, tool string) (string, error) {
	name := invalidProxyToolName.ReplaceAllString(server+"__"+tool, "_")
	name = strings.Trim(name, "_")
	if name == "" {
		return "", fmt.Errorf("MCP tool %q/%q has no usable proxy name", server, tool)
	}
	if len(name) > maxProxyToolNameBytes {
		return "", fmt.Errorf(
			"MCP tool proxy name %q exceeds %d bytes",
			name,
			maxProxyToolNameBytes,
		)
	}
	return name, nil
}
