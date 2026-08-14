package community

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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

// ToolResult is one completed MCP tool call rendered for the model. Bounding
// text cannot corrupt a structure the way cutting a marshalled envelope did.
type ToolResult struct {
	Text    string
	IsError bool
}

// GroundingDocument is one resource a server marked for the assistant, already
// read and bounded. See docs/sirens-echo-mcp-roster.md.
type GroundingDocument struct {
	Server string
	URI    string
	Title  string
	Text   string
}

// ToolSession is the per-turn MCP capability available to Agent Proxy.
type ToolSession interface {
	Tools() []ToolDefinition
	Grounding() []GroundingDocument
	Unavailable() []string
	Call(ctx context.Context, name string, arguments map[string]any) (ToolResult, error)
	Close() error
}

// ToolProvider opens the configured MCP roster for one model turn.
type ToolProvider interface {
	Open(ctx context.Context) (ToolSession, error)
}

const (
	// The one tool that belongs to the harness rather than a server. It is named
	// like any other so a collision is caught by the same rule.
	refreshToolServer = "harness"
	refreshToolTool   = "refresh_tools"
)

// refreshToolDescription is read by the model, so it states the bound rather
// than leaving the model to discover it. See docs/sirens-echo-harness-tools.md.
const refreshToolDescription = "Re-read which tools every configured server " +
	"offers. Use it when a tool you expected is missing or a tool list looks " +
	"stale. The new list applies to your next turn and not this one, so do not " +
	"claim a tool is available until you can see it."

// MCPProvider supervises the configured MCP roster through the official Go SDK.
// An empty roster is a valid no-tool capability boundary.
type MCPProvider struct {
	// Sandbox labels issues this service files. Zero applies nothing.
	Sandbox    sandboxLabelPolicy
	Servers    []MCPServerDefinition
	HTTPClient *http.Client
	// RefreshInterval bounds staleness where notifications cannot arrive. Zero
	// uses defaultRosterRefresh.
	RefreshInterval time.Duration
	// CallTimeout bounds one tool call. Zero uses defaultCallTimeout.
	CallTimeout time.Duration
	// Telemetry traces discovery. Nil records nothing, which is what a
	// hand-built provider in a test gets unless it asks otherwise.
	Telemetry *Telemetry

	mu      sync.Mutex
	started bool
	root    context.Context
	cancel  context.CancelFunc
	entries []*supervisedServer
}

// supervisedServer holds one server's connection across turns, so a turn pays
// no handshake and a listing survives until something invalidates it.
type supervisedServer struct {
	definition MCPServerDefinition
	notifies   bool
	session    *mcp.ClientSession
	tools      []*mcp.Tool
	resources  []*mcp.Resource
	prompts    []*mcp.Prompt
	refreshed  time.Time
	retryAfter time.Time
	backoff    time.Duration
	stale      atomic.Bool
}

// PromptMessage is one message from a server prompt, rendered to text. Prompts
// are user-controlled, so these enter a turn only when a caller names one.
type PromptMessage struct {
	Role string
	Text string
}

// transportNotifies reports whether a transport can deliver server-initiated
// messages. Streamable cannot while its standalone SSE stream stays disabled.
func transportNotifies(server MCPServerDefinition) bool {
	switch server.ResolvedTransport() {
	case MCPTransportStdio, MCPTransportSSE:
		return true
	}
	return false
}

type registeredMCPTool struct {
	serverName string
	toolName   string
	session    *mcp.ClientSession
}

type mcpToolSession struct {
	tools       []ToolDefinition
	grounding   []GroundingDocument
	registered  map[string]registeredMCPTool
	sessions    []*mcp.ClientSession
	unavailable []string
	callTimeout time.Duration
	// sandbox labels an issue this service files. See sirens-echo#208.
	sandbox sandboxLabelPolicy
	// refresh is the one tool that is not an MCP server's. Nil leaves it
	// unoffered. See docs/sirens-echo-mcp-roster.md.
	refresh func() int
}

// Open returns this turn's view over the supervised roster. It connects only
// what is not already connected and lists only what is stale or expired.
func (p *MCPProvider) Open(ctx context.Context) (ToolSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.startLocked()
	opened := &mcpToolSession{
		registered:  make(map[string]registeredMCPTool),
		callTimeout: p.callTimeout(),
		sandbox:     p.Sandbox,
	}
	now := time.Now()
	reached, listed := 0, 0
	for _, entry := range p.entries {
		// A server that cannot answer contributes no tools and the turn goes on
		// with the rest. One transient outage must not cost every turn.
		touched, discovered, err := p.readyLocked(ctx, entry, now)
		if touched {
			reached++
		}
		if discovered {
			listed++
		}
		if err != nil {
			opened.unavailable = append(opened.unavailable, entry.definition.Name)
			continue
		}
		// A collision stays fatal. That is a roster mistake, and degrading past
		// it would silently drop whichever tool lost the race.
		if err := opened.register(entry.definition.Name, entry.session, entry.tools); err != nil {
			return nil, err
		}
		opened.grounding = append(opened.grounding, p.readGrounding(p.turnTraced(ctx), entry)...)
	}
	// Stated rather than inferred from the span's duration. See
	// docs/sirens-echo-tool-discovery-telemetry.md.
	trace.SpanFromContext(ctx).SetAttributes(
		attribute.Int("mcp.tools.configured", len(p.entries)),
		attribute.Int("mcp.tools.reached", reached),
		attribute.Int("mcp.tools.listed", listed),
		attribute.Bool("mcp.tools.cached", len(p.entries) > 0 && reached == 0),
	)
	if len(p.entries) > 0 && len(opened.unavailable) == len(p.entries) {
		return nil, fmt.Errorf("no configured MCP server is reachable")
	}
	// An empty roster stays a no-tool boundary. Offering a refresh for nothing
	// would be a capability claim with no capability behind it.
	if len(p.entries) > 0 {
		if err := opened.registerRefresh(p.Refresh); err != nil {
			return nil, err
		}
	}
	return opened, nil
}

// refreshToolProxyName is the registered name, derived rather than written so
// it cannot drift from the one registerRefresh offered.
func refreshToolProxyName() string {
	name, _ := proxyToolName(refreshToolServer, refreshToolTool)
	return name
}

// callRefresh answers with what it did and what it did not do. A tool that
// implies the list already changed is the over-claiming defect in issue 211.
func (s *mcpToolSession) callRefresh() ToolResult {
	marked := s.refresh()
	return ToolResult{Text: fmt.Sprintf(
		"Marked %d configured server(s) for re-reading. This turn still sees the "+
			"tool list it started with.", marked,
	)}
}

// registerRefresh offers the roster's own refresh as a tool. A collision is
// fatal for the same reason a roster collision is: one of them would vanish.
func (s *mcpToolSession) registerRefresh(refresh func() int) error {
	name, err := proxyToolName(refreshToolServer, refreshToolTool)
	if err != nil {
		return err
	}
	if _, exists := s.registered[name]; exists {
		return fmt.Errorf("MCP tool name collision %q with the harness refresh", name)
	}
	s.refresh = refresh
	s.tools = append(s.tools, ToolDefinition{
		Name:        name,
		Server:      refreshToolServer,
		Original:    refreshToolTool,
		Description: refreshToolDescription,
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	})
	return nil
}

func (p *MCPProvider) startLocked() {
	if p.started {
		return
	}
	p.started = true
	// Detached from any turn, because these connections outlive the turn that
	// first needed them.
	p.root, p.cancel = context.WithCancel(context.Background())
	for _, server := range p.Servers {
		p.entries = append(p.entries, &supervisedServer{
			definition: server,
			notifies:   transportNotifies(server),
		})
	}
}

// discoveryStage names where a round trip was when it failed, so a rejection is
// attributable without a request body. See sirens-echo#139.
const (
	discoveryStageConnect   = "connect"
	discoveryStageTools     = "tools"
	discoveryStageResources = "resources"
	discoveryStagePrompts   = "prompts"
)

// startDiscoverySpan names the server a round trip belongs to. Injected rather
// than global, so two tests recording spans cannot overwrite each other.
func (p *MCPProvider) startDiscoverySpan(
	ctx context.Context, server string,
) (context.Context, trace.Span) {
	return telemetryOrNoop(p.Telemetry).StartSpan(
		ctx,
		"mcp.server.discovery",
		attribute.String("mcp.server.name", server),
	)
}

// readyLocked connects and lists only what this turn needs. Reaching the
// network and completing a listing are different: a failed connect is neither.
func (p *MCPProvider) readyLocked(
	turnCtx context.Context,
	entry *supervisedServer,
	now time.Time,
) (reached, listed bool, err error) {
	base := p.turnTraced(turnCtx)
	connecting := entry.session == nil
	if connecting && now.Before(entry.retryAfter) {
		// Backing off spends no round trip, so it is neither.
		return false, false, fmt.Errorf(
			"MCP server %s is backing off", entry.definition.Name)
	}
	// needsTools consumes a list_changed notification, so it runs exactly once
	// on every path through this function.
	if !connecting && !entry.needsTools(p.refreshInterval(), now) {
		return false, false, nil
	}
	// From here a round trip happens, and it gets a span naming the server.
	// See docs/sirens-echo-tool-discovery-telemetry.md.
	discoveryCtx, span := p.startDiscoverySpan(base, entry.definition.Name)
	defer span.End()
	if connecting {
		span.SetAttributes(attribute.String("mcp.discovery.stage", discoveryStageConnect))
		if err := p.connectLocked(discoveryCtx, entry); err != nil {
			entry.penalise(now)
			return true, false, err
		}
		entry.backoff = 0
		if !entry.needsTools(p.refreshInterval(), now) {
			return true, false, nil
		}
	}
	listCtx, cancel := context.WithTimeout(discoveryCtx, mcpListTimeout)
	defer cancel()
	span.SetAttributes(attribute.String("mcp.discovery.stage", discoveryStageTools))
	discovered, err := discoverTools(listCtx, entry.session)
	if err != nil {
		entry.dropSession(now)
		return true, false, err
	}
	// A server may publish tools without resources, so an unsupported listing
	// is an empty one rather than a failure.
	span.SetAttributes(attribute.String("mcp.discovery.stage", discoveryStageResources))
	resources, err := discoverResources(listCtx, entry.session)
	if err != nil {
		entry.dropSession(now)
		return true, false, err
	}
	span.SetAttributes(attribute.String("mcp.discovery.stage", discoveryStagePrompts))
	prompts, err := discoverPrompts(listCtx, entry.session)
	if err != nil {
		entry.dropSession(now)
		return true, false, err
	}
	entry.tools = discovered
	entry.resources = resources
	entry.prompts = prompts
	entry.refreshed = now
	return true, true, nil
}

func (s *supervisedServer) dropSession(now time.Time) {
	if s.session != nil {
		_ = s.session.Close()
	}
	s.session = nil
	s.tools = nil
	s.resources = nil
	s.prompts = nil
	s.penalise(now)
}

// turnTraced cancels with the pool but carries the calling turn's span context,
// so a pooled connection outlives the turn while its traffic stays correlated.
func (p *MCPProvider) turnTraced(turnCtx context.Context) context.Context {
	return trace.ContextWithSpanContext(p.root, trace.SpanContextFromContext(turnCtx))
}

func (p *MCPProvider) connectLocked(base context.Context, entry *supervisedServer) error {
	// One client per server, so its notification handler needs no way to map a
	// request back to which server sent it.
	client := mcp.NewClient(
		&mcp.Implementation{Name: "sirens-echo", Version: "1"},
		&mcp.ClientOptions{
			ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) {
				entry.stale.Store(true)
			},
			ResourceListChangedHandler: func(context.Context, *mcp.ResourceListChangedRequest) {
				entry.stale.Store(true)
			},
			PromptListChangedHandler: func(context.Context, *mcp.PromptListChangedRequest) {
				entry.stale.Store(true)
			},
		},
	)
	transport, err := clientTransport(base, entry.definition, p.HTTPClient)
	if err != nil {
		return err
	}
	connectCtx, cancel := context.WithTimeout(base, mcpConnectTimeout)
	defer cancel()
	session, err := client.Connect(connectCtx, transport, nil)
	if err != nil {
		return err
	}
	entry.session = session
	entry.tools = nil
	entry.stale.Store(false)
	return nil
}

// Refresh marks every rostered server stale, so the next turn re-lists instead
// of waiting out the interval. It dials nothing itself. See sirens-echo#163.
func (p *MCPProvider) Refresh() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, entry := range p.entries {
		entry.stale.Store(true)
	}
	return len(p.entries)
}

func (p *MCPProvider) refreshInterval() time.Duration {
	if p.RefreshInterval > 0 {
		return p.RefreshInterval
	}
	return defaultRosterRefresh
}

func (p *MCPProvider) callTimeout() time.Duration {
	if p.CallTimeout > 0 {
		return p.CallTimeout
	}
	return defaultCallTimeout
}

// needsTools is true on a first listing, on a list_changed notification, and on
// expiry for a transport that cannot send one.
func (s *supervisedServer) needsTools(interval time.Duration, now time.Time) bool {
	if s.tools == nil || s.stale.Swap(false) {
		return true
	}
	return !s.notifies && now.Sub(s.refreshed) >= interval
}

func (s *supervisedServer) penalise(now time.Time) {
	if s.backoff == 0 {
		s.backoff = mcpBackoffMin
	} else if s.backoff < mcpBackoffMax {
		s.backoff *= 2
	}
	if s.backoff > mcpBackoffMax {
		s.backoff = mcpBackoffMax
	}
	s.retryAfter = now.Add(s.backoff)
}

// Close shuts down every supervised connection. A stdio child exits with the
// root context rather than outliving the process that started it.
func (p *MCPProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	errs := make([]error, 0, len(p.entries))
	for _, entry := range p.entries {
		if entry.session == nil {
			continue
		}
		if err := entry.session.Close(); err != nil {
			errs = append(errs, err)
		}
		entry.session = nil
	}
	if p.cancel != nil {
		p.cancel()
	}
	p.entries = nil
	p.started = false
	return errors.Join(errs...)
}

// clientTransport builds the transport one roster entry asks for. Validation
// has already rejected an entry whose fields do not match its transport.
func clientTransport(
	ctx context.Context,
	server MCPServerDefinition,
	httpClient *http.Client,
) (mcp.Transport, error) {
	switch server.ResolvedTransport() {
	case MCPTransportStreamable:
		return &mcp.StreamableClientTransport{
			Endpoint:             server.URL,
			HTTPClient:           headerClient(httpClient, server.Headers),
			DisableStandaloneSSE: true,
		}, nil
	case MCPTransportSSE:
		return &mcp.SSEClientTransport{
			Endpoint:   server.URL,
			HTTPClient: headerClient(httpClient, server.Headers),
		}, nil
	case MCPTransportStdio:
		// Bound to the caller's context, so the child dies with the session
		// rather than outliving it.
		command := exec.CommandContext(ctx, server.Command, server.Args...)
		command.Env = environSlice(server.Env)
		return &mcp.CommandTransport{Command: command}, nil
	}
	return nil, fmt.Errorf(
		"MCP server %s has unsupported transport %q",
		server.Name,
		server.ResolvedTransport(),
	)
}

// headerClient adds an entry's declared headers to its requests. One client is
// shared by the roster, so a declaring entry gets a copy rather than mutating.
func headerClient(base *http.Client, headers map[string]string) *http.Client {
	if len(headers) == 0 {
		return base
	}
	if base == nil {
		base = http.DefaultClient
	}
	inner := base.Transport
	if inner == nil {
		inner = http.DefaultTransport
	}
	copied := *base
	copied.Transport = &headerRoundTripper{inner: inner, headers: headers}
	return &copied
}

// headerRoundTripper clones before writing, because a RoundTripper must not
// modify the request it is given and the SDK reuses one across a retry.
type headerRoundTripper struct {
	inner   http.RoundTripper
	headers map[string]string
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	for name, value := range h.headers {
		cloned.Header.Set(name, value)
	}
	return h.inner.RoundTrip(cloned)
}

// environSlice renders the roster's declared environment for a stdio child.
// Echo's own variables are never inherited, so nothing rides along by accident.
func environSlice(env map[string]string) []string {
	pairs := make([]string, 0, len(env))
	for key, value := range env {
		pairs = append(pairs, key+"="+value)
	}
	sort.Strings(pairs)
	return pairs
}

func discoverTools(ctx context.Context, session *mcp.ClientSession) ([]*mcp.Tool, error) {
	discovered := make([]*mcp.Tool, 0)
	cursor := ""
	for {
		result, err := session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		discovered = append(discovered, result.Tools...)
		if result.NextCursor == "" {
			return discovered, nil
		}
		cursor = result.NextCursor
	}
}

func discoverResources(
	ctx context.Context,
	session *mcp.ClientSession,
) ([]*mcp.Resource, error) {
	// A server without the resources capability answers nothing here, which the
	// SDK surfaces as an error rather than an empty list.
	if session.InitializeResult().Capabilities.Resources == nil {
		return nil, nil
	}
	discovered := make([]*mcp.Resource, 0)
	cursor := ""
	for {
		result, err := session.ListResources(ctx, &mcp.ListResourcesParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		discovered = append(discovered, result.Resources...)
		if result.NextCursor == "" {
			return discovered, nil
		}
		cursor = result.NextCursor
	}
}

// groundingCandidate reports whether a server marked this resource for the
// assistant, so a large catalogue does not reach the prompt by default.
func groundingCandidate(resource *mcp.Resource) bool {
	if resource == nil || resource.Annotations == nil {
		return false
	}
	for _, role := range resource.Annotations.Audience {
		if role == "assistant" {
			return true
		}
	}
	return false
}

func resourcePriority(resource *mcp.Resource) float64 {
	if resource.Annotations == nil {
		return 0
	}
	return resource.Annotations.Priority
}

// readGrounding reads the marked resources in priority order, stopping at the
// document and byte bounds so a large catalogue cannot crowd out the turn.
func (p *MCPProvider) readGrounding(
	base context.Context,
	entry *supervisedServer,
) []GroundingDocument {
	candidates := make([]*mcp.Resource, 0, len(entry.resources))
	for _, resource := range entry.resources {
		if groundingCandidate(resource) {
			candidates = append(candidates, resource)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if resourcePriority(candidates[i]) != resourcePriority(candidates[j]) {
			return resourcePriority(candidates[i]) > resourcePriority(candidates[j])
		}
		return candidates[i].URI < candidates[j].URI
	})
	documents := make([]GroundingDocument, 0, len(candidates))
	budget := maxGroundingBytes
	for _, resource := range candidates {
		if len(documents) >= maxGroundingDocuments || budget <= 0 {
			break
		}
		readCtx, cancel := context.WithTimeout(base, mcpListTimeout)
		result, err := entry.session.ReadResource(
			readCtx,
			&mcp.ReadResourceParams{URI: resource.URI},
		)
		cancel()
		if err != nil {
			continue
		}
		text := resourceText(result)
		if text == "" {
			continue
		}
		if len(text) > budget {
			text = text[:runeBoundary(text, budget)] + "\n[truncated by the runtime]"
		}
		budget -= len(text)
		documents = append(documents, GroundingDocument{
			Server: entry.definition.Name,
			URI:    resource.URI,
			Title:  resourceTitle(resource),
			Text:   text,
		})
	}
	return documents
}

// resourceText keeps the readable parts. A blob has no meaning to the model.
func resourceText(result *mcp.ReadResourceResult) string {
	parts := make([]string, 0, len(result.Contents))
	for _, contents := range result.Contents {
		if contents != nil && contents.Text != "" {
			parts = append(parts, contents.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func resourceTitle(resource *mcp.Resource) string {
	if resource.Title != "" {
		return resource.Title
	}
	return resource.Name
}

func (s *mcpToolSession) register(
	serverName string,
	session *mcp.ClientSession,
	discovered []*mcp.Tool,
) error {
	for _, tool := range discovered {
		name, err := proxyToolName(serverName, tool.Name)
		if err != nil {
			return err
		}
		if _, exists := s.registered[name]; exists {
			return fmt.Errorf("MCP tool name collision %q", name)
		}
		s.registered[name] = registeredMCPTool{
			serverName: serverName,
			toolName:   tool.Name,
			session:    session,
		}
		s.tools = append(s.tools, ToolDefinition{
			Name:        name,
			Server:      serverName,
			Original:    tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}
	return nil
}

func (s *mcpToolSession) Tools() []ToolDefinition {
	return append([]ToolDefinition(nil), s.tools...)
}

func (s *mcpToolSession) Grounding() []GroundingDocument {
	return append([]GroundingDocument(nil), s.grounding...)
}

// Unavailable names the configured servers that did not answer this turn.
func (s *mcpToolSession) Unavailable() []string {
	return append([]string(nil), s.unavailable...)
}

func (s *mcpToolSession) Call(
	ctx context.Context,
	name string,
	arguments map[string]any,
) (ToolResult, error) {
	if s.refresh != nil && name == refreshToolProxyName() {
		return s.callRefresh(), nil
	}
	tool, exists := s.registered[name]
	if !exists {
		return ToolResult{}, fmt.Errorf("model requested unavailable MCP tool %q", name)
	}
	// Bounded below the turn budget, so a server that never answers fails as a
	// tool failure with time left to report it. See docs/sirens-echo-tools.md.
	bound := s.callTimeout
	if bound <= 0 {
		bound = defaultCallTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, bound)
	defer cancel()
	// The model never supplies this and cannot omit it. See sirens-echo#208.
	if s.sandbox.applies(tool) {
		arguments = s.sandbox.withSandboxLabel(arguments)
	}
	result, err := tool.session.CallTool(callCtx, &mcp.CallToolParams{
		Name:      tool.toolName,
		Arguments: arguments,
	})
	if err != nil {
		return ToolResult{}, fmt.Errorf(
			"call MCP tool %s/%s: %w", tool.serverName, tool.toolName, err,
		)
	}
	text, err := toolResultText(result)
	if err != nil {
		return ToolResult{}, fmt.Errorf(
			"render MCP tool result %s/%s: %w", tool.serverName, tool.toolName, err,
		)
	}
	return ToolResult{Text: text, IsError: result.IsError}, nil
}

// toolResultText renders a result as text. A non-text part keeps its JSON form
// rather than being dropped, which would read to the model as an empty answer.
func toolResultText(result *mcp.CallToolResult) (string, error) {
	parts := make([]string, 0, len(result.Content))
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
			continue
		}
		raw, err := content.MarshalJSON()
		if err != nil {
			return "", err
		}
		parts = append(parts, string(raw))
	}
	if len(parts) == 0 && result.StructuredContent != nil {
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil {
			return "", err
		}
		parts = append(parts, string(raw))
	}
	return strings.Join(parts, "\n"), nil
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
	// Checked before composing. Trimming the separator leaves the other half
	// standing, so an empty half passes the test below. See sirens-echo#587.
	if strings.TrimSpace(server) == "" || strings.TrimSpace(tool) == "" {
		return "", fmt.Errorf("MCP tool %q/%q is missing a server or tool name", server, tool)
	}
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

// runeBoundary returns the largest offset at or below limit that does not split
// a rune, so a bounded document stays valid UTF-8.
func runeBoundary(value string, limit int) int {
	if limit >= len(value) {
		return len(value)
	}
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return limit
}

func discoverPrompts(ctx context.Context, session *mcp.ClientSession) ([]*mcp.Prompt, error) {
	if session.InitializeResult().Capabilities.Prompts == nil {
		return nil, nil
	}
	discovered := make([]*mcp.Prompt, 0)
	cursor := ""
	for {
		result, err := session.ListPrompts(ctx, &mcp.ListPromptsParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		discovered = append(discovered, result.Prompts...)
		if result.NextCursor == "" {
			return discovered, nil
		}
		cursor = result.NextCursor
	}
}

// PromptRequestError marks a prompt failure the caller can fix, so its text is
// safe to return. Transport failures stay generic to keep endpoints out of a body.
type PromptRequestError struct{ Message string }

func (e PromptRequestError) Error() string { return e.Message }

// Prompt retrieves a named server prompt. It is reached only when a caller
// selected one, because prompts are user-controlled by design.
func (p *MCPProvider) Prompt(
	ctx context.Context,
	server, name string,
	arguments map[string]string,
) ([]PromptMessage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.startLocked()
	entry := p.entryLocked(server)
	if entry == nil {
		return nil, PromptRequestError{fmt.Sprintf("MCP server %q is not in the roster", server)}
	}
	if _, _, err := p.readyLocked(ctx, entry, time.Now()); err != nil {
		return nil, fmt.Errorf("MCP server %s is unavailable: %w", server, err)
	}
	declared := findPrompt(entry.prompts, name)
	if declared == nil {
		return nil, PromptRequestError{fmt.Sprintf("MCP server %s publishes no prompt %q", server, name)}
	}
	// Checked here so a missing argument names itself rather than surfacing as
	// whatever the server decides to return.
	for _, argument := range declared.Arguments {
		if argument.Required && strings.TrimSpace(arguments[argument.Name]) == "" {
			return nil, PromptRequestError{
				fmt.Sprintf("prompt %s/%s requires argument %q", server, name, argument.Name),
			}
		}
	}
	getCtx, cancel := context.WithTimeout(p.turnTraced(ctx), mcpListTimeout)
	defer cancel()
	result, err := entry.session.GetPrompt(getCtx, &mcp.GetPromptParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		return nil, fmt.Errorf("get prompt %s/%s: %w", server, name, err)
	}
	messages := make([]PromptMessage, 0, len(result.Messages))
	for _, message := range result.Messages {
		text := promptContentText(message.Content)
		if text == "" {
			continue
		}
		messages = append(messages, PromptMessage{Role: string(message.Role), Text: text})
	}
	return messages, nil
}

func (p *MCPProvider) entryLocked(server string) *supervisedServer {
	for _, entry := range p.entries {
		if entry.definition.Name == server {
			return entry
		}
	}
	return nil
}

func findPrompt(prompts []*mcp.Prompt, name string) *mcp.Prompt {
	for _, prompt := range prompts {
		if prompt != nil && prompt.Name == name {
			return prompt
		}
	}
	return nil
}

// promptContentText renders a prompt message. An embedded resource carries its
// text, which is the part a model can read.
func promptContentText(content mcp.Content) string {
	switch typed := content.(type) {
	case *mcp.TextContent:
		return typed.Text
	case *mcp.EmbeddedResource:
		if typed.Resource != nil {
			return typed.Resource.Text
		}
	}
	return ""
}
