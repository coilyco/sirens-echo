package community

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultDefinitionPath = "agent/sirens-echo.yaml"
	defaultHTTPListenAddr = "127.0.0.1:8080"
	// The fallback is a live service, so it is only safe for the definition that
	// service actually runs. See resolveInstanceName and sirens-echo#542.
	defaultInstanceName = "sirens-echo"
	// The identity that fallback belongs to, so the guard compares the thing
	// itself rather than a filename that can be spelled anything. See #706.
	defaultInstanceIdentity = "Sirens Echo"
	defaultBundleDir        = "/app/agent/bundles"

	ResponseStyleNeutral = "neutral"
	ResponseStyleSocial  = "social"
)

// Admission defaults are sized for a guild the operator does not moderate.
// See docs/sirens-echo-admission.md.

var defaultRateLimitPolicy = RateLimitPolicy{
	PerUser:     RateLimit{Burst: 3, Every: 30 * time.Second},
	PerContext:  RateLimit{Burst: 10, Every: 10 * time.Second},
	Global:      RateLimit{Burst: 20, Every: 5 * time.Second},
	MaxPending:  8,
	NotifyEvery: 5 * time.Minute,
}

// DefaultAgentProxyURL is a neutral fallback. Deployment owns the real
// endpoint and sets AGENT_PROXY_URL.
const DefaultAgentProxyURL = "http://agent-proxy:8080"

// DefaultOTLPEndpoint is the existing in-cluster SigNoZ collector.
const DefaultOTLPEndpoint = "http://signoz-otel-collector.observability.svc.cluster.local:4318"

var (
	mcpServerNamePattern   = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	environmentNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	// RFC 7230 token characters, because the vendor picks the header name and
	// x-api-key is as common as a capitalised one.
	headerNamePattern    = regexp.MustCompile(`^[A-Za-z0-9!#$%&'*+.^_` + "`" + `|~-]+$`)
	discordSnowflake     = regexp.MustCompile(`^[0-9]{15,20}$`)
	discordHandlePattern = regexp.MustCompile(`^[a-z0-9._]{2,32}$`)
	composedRolePattern  = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	// channelLabelPattern matches the grounding validator's channel form, so a
	// label cannot introduce a reference the model is rejected for repeating.
	channelLabelPattern = regexp.MustCompile(`^#[A-Za-z_][A-Za-z0-9_-]*$`)
	// rateLimitPattern is "<burst>/<refill interval for one token>".
	rateLimitPattern = regexp.MustCompile(`^([0-9]+)/(.+)$`)
)

// MCP client transports. Deployment picks the servers and their transports.
// Echo validates the shape of an entry, never which server is acceptable.
const (
	MCPTransportStreamable = "streamable"
	MCPTransportSSE        = "sse"
	MCPTransportStdio      = "stdio"
)

// MCPServerDefinition is one resolved roster entry. Deployment owns the roster,
// so this is built by the loader rather than written in a source definition.
type MCPServerDefinition struct {
	Name      string
	Transport string
	URL       string
	Command   string
	Args      []string
	Env       map[string]string
	Headers   map[string]string
}

// ResolvedTransport defaults to streamable, so an entry written before
// transports were selectable keeps working unchanged.
func (s MCPServerDefinition) ResolvedTransport() string {
	if trimmed := strings.TrimSpace(s.Transport); trimmed != "" {
		return trimmed
	}
	return MCPTransportStreamable
}

// Definition is the source-controlled attribution, route, and policy selection.
type Definition struct {
	Schema             string   `json:"schema" yaml:"schema"`
	Identity           string   `json:"identity" yaml:"identity"`
	SelfAliases        []string `json:"self_aliases,omitempty" yaml:"self_aliases,omitempty"`
	AuditRole          string   `json:"audit_role" yaml:"audit_role"`
	ResponseStyle      string   `json:"response_style" yaml:"response_style"`
	Channel            string   `json:"channel" yaml:"channel"`
	MaxContextMessages int      `json:"max_context_messages" yaml:"max_context_messages"`
	LocalSkillRoots    []string `json:"local_skill_roots" yaml:"local_skill_roots"`
	IssueTracker       string   `json:"issue_tracker,omitempty" yaml:"issue_tracker,omitempty"`
	// Composed requires a materialized agent-compose bundle, so a profile that
	// asks for an identity fails startup rather than answering without one.
	Composed bool `json:"composed,omitempty" yaml:"composed,omitempty"`
	// ModelBudget is empty for a definition that takes the packaged ceilings.
	ModelBudget ModelBudget `json:"model_budget,omitempty" yaml:"model_budget,omitempty"`
}

// ModelBudget is what one turn on this definition may spend on model calls. The
// two profiles do not share a substrate, so they cannot share one ceiling.
type ModelBudget struct {
	ToolRounds           int `json:"tool_rounds,omitempty" yaml:"tool_rounds,omitempty"`
	BaseCompletionTokens int `json:"base_completion_tokens,omitempty" yaml:"base_completion_tokens,omitempty"`
	MaxCompletionTokens  int `json:"max_completion_tokens,omitempty" yaml:"max_completion_tokens,omitempty"`
	BudgetRaises         int `json:"budget_raises,omitempty" yaml:"budget_raises,omitempty"`
	ToolResultBytes      int `json:"tool_result_bytes,omitempty" yaml:"tool_result_bytes,omitempty"`

	// Keyed by the model-facing tool name. One ceiling across every tool is
	// more suspect than its value, so a tool may name its own. See #635.
	ToolResultBytesByTool map[string]int `json:"tool_result_bytes_by_tool,omitempty" yaml:"tool_result_bytes_by_tool,omitempty"`
}

// ToolResultBytesFor resolves the bound for one tool, falling back to the
// budget-wide ceiling so an unnamed tool keeps exactly today's behaviour.
func (b ModelBudget) ToolResultBytesFor(tool string) int {
	if bound, ok := b.ToolResultBytesByTool[tool]; ok {
		return bound
	}
	return b.ToolResultBytes
}

// resolved fills each unset field from the packaged default, so a definition
// names only what it changes and an unset budget is today's behaviour exactly.
func (b ModelBudget) resolved() ModelBudget {
	if b.ToolRounds == 0 {
		b.ToolRounds = maxToolRounds
	}
	if b.BaseCompletionTokens == 0 {
		b.BaseCompletionTokens = baseCompletionTokens
	}
	if b.MaxCompletionTokens == 0 {
		b.MaxCompletionTokens = maxCompletionTokens
	}
	if b.BudgetRaises == 0 {
		b.BudgetRaises = budgetRaisesAllowed
	}
	if b.ToolResultBytes == 0 {
		b.ToolResultBytes = maxToolResultBytes
	}
	return b
}

// validate refuses a budget that would spend without bound or contradict
// itself. Every field is a ceiling, so none of them may be negative.
func (b ModelBudget) validate() error {
	for _, field := range []struct {
		name  string
		value int
	}{
		{"tool_rounds", b.ToolRounds},
		{"base_completion_tokens", b.BaseCompletionTokens},
		{"max_completion_tokens", b.MaxCompletionTokens},
		{"budget_raises", b.BudgetRaises},
		{"tool_result_bytes", b.ToolResultBytes},
	} {
		if field.value < 0 {
			return fmt.Errorf("model_budget %s must not be negative, got %d",
				field.name, field.value)
		}
	}
	// An absent key inherits. A present one is deliberate, so zero here means
	// deliver nothing rather than unset, and that is never what anyone meant.
	for tool, bound := range b.ToolResultBytesByTool {
		if bound <= 0 {
			return fmt.Errorf(
				"model_budget tool_result_bytes_by_tool[%q] must be positive, got %d: "+
					"remove the entry to inherit tool_result_bytes", tool, bound)
		}
	}
	resolved := b.resolved()
	if resolved.MaxCompletionTokens < resolved.BaseCompletionTokens {
		return fmt.Errorf(
			"model_budget max_completion_tokens %d is below base_completion_tokens %d",
			resolved.MaxCompletionTokens, resolved.BaseCompletionTokens,
		)
	}
	// A ceiling the rungs stop short of never applies. See sirens-echo#522.
	if reached := resolved.ladderTop(); reached < resolved.MaxCompletionTokens {
		return fmt.Errorf(
			"model_budget max_completion_tokens %d is unreachable: %d raises from %d "+
				"tops out at %d",
			resolved.MaxCompletionTokens, resolved.BudgetRaises,
			resolved.BaseCompletionTokens, reached,
		)
	}
	return nil
}

// ladderTop is where the raises stop climbing, by the same doubling
// nextCompletionBudget performs. Stops early, so a large count cannot overflow.
func (b ModelBudget) ladderTop() int {
	reached := b.BaseCompletionTokens
	for raise := 0; raise < b.BudgetRaises && reached < b.MaxCompletionTokens; raise++ {
		reached *= completionBudgetStep
	}
	return reached
}

// Principal identifies the one speaker the prompt trusts. The values are
// deployment-owned. See docs/sirens-echo-prompt.md.
type Principal struct {
	Handle string
	UserID string
}

// Configured reports whether deployment supplied both signals. One alone
// identifies nobody, so the prompt renders neither.
func (p Principal) Configured() bool { return p.Handle != "" && p.UserID != "" }

// PlaceholderPrincipal renders the tracked snapshot and the build-time policy
// check. It is not a real account, matching docs/access-policy.reference.yaml.
var PlaceholderPrincipal = Principal{Handle: "example_handle", UserID: "1024000000000000001"}

// Config combines the source-controlled definition with deployment secrets.
type Config struct {
	Definition     Definition
	DefinitionPath string
	InstanceName   string
	// LogWriter receives structured logs. Empty means stdout, which is what the
	// service wants. A runner emitting a dataset on stdout selects stderr.
	LogWriter io.Writer
	// Principal is empty until deployment names the trusted account.
	Principal Principal
	// BundlePath is the materialized bundle for the deployment's role, empty
	// when the definition composes nothing.
	BundlePath     string
	DiscordEnabled bool
	DiscordToken   string
	// DiscordChannelIDs are the channels that may summon this deployment, plus
	// their threads. Channel IDs are globally unique, so the list spans guilds.
	DiscordChannelIDs []string
	// DiscordGuildIDs optionally restricts which guilds may summon at all.
	// Empty means every guild the bot joined, still bounded by the channels.
	DiscordGuildIDs []string
	// DiscordDMEnabled admits direct messages. Off by default, because a
	// direct message has no guild moderation behind it.
	DiscordDMEnabled bool
	AgentProxyURL    string
	AgentProxyModel  string
	OTLPEndpoint     string
	HTTPListenAddr   string
	// MCPRosterPath names the deployment-owned mcpServers file. Empty is a
	// valid no-tool boundary.
	MCPRosterPath string
	// ContentClassesPath names the taxonomy the content gate enforces. Empty
	// runs no gate at all, so the deployment is the switch.
	ContentClassesPath string
	// HTTPTrustToken authenticates a caller on the tailnet. Empty trusts
	// nobody. See docs/sirens-echo-http-identity.md.
	HTTPTrustToken string
	// FetchHosts is the allowlist the fetch tool may reach. Empty offers no
	// tool. See docs/sirens-echo-fetch.md.
	FetchHosts []string
	// SandboxLabelID labels every issue this service files. Zero applies
	// nothing. See docs/sirens-echo-sandbox-label.md.
	SandboxLabelID int
	// AccessPolicyPath names the deployment's tracked allowlist file. Empty
	// synthesizes the equivalent from the Discord environment variables.
	AccessPolicyPath string
	// DiscordCommandsEnabled registers and serves application commands. Off by
	// default because registering is a write to Discord's API.
	DiscordCommandsEnabled bool
	// JobWorkspaceRoot enables executing jobs. Empty means no execution at all,
	// which is the default posture.
	JobWorkspaceRoot string
	// JobRepository and JobVerb fix what an executing job does, both from
	// closed sets in this repository.
	JobRepository string
	JobVerb       string
	// JobStoreDir is the durable job store's directory. Empty keeps jobs in
	// memory, which loses them on restart. See docs/sirens-echo-jobs.md.
	JobStoreDir string
	// RepoInventoryURL and RepoInventoryOrg name the forge and organization the
	// inventory lists. Either empty offers no tool.
	RepoInventoryURL string
	// The read is unauthenticated, so it sees public repositories only.
	RepoInventoryOrg string
	// ScratchDir mounts the per-requester scratchpad. Empty offers no scratchpad
	// tools at all. See docs/sirens-echo-scratchpad.md.
	ScratchDir string
	// PhrasesPath names the canonical phrase registry. Empty renders nothing,
	// which is today's behaviour. See docs/sirens-echo-phrases.md.
	PhrasesPath    string
	RequestTimeout time.Duration
	QueueTimeout   time.Duration
	// ShutdownGrace is how long a restart waits for the turns already running.
	// It has to fit the pod's kill window. See docs/sirens-echo-shutdown.md.
	ShutdownGrace time.Duration
	RateLimit     RateLimitPolicy
}

// resolveInstanceName refuses to hand a non-Echo definition Echo's service name.
// Defaulting there merges another profile's spans into Echo's. See #542.
func resolveInstanceName(identity, configured string) (string, error) {
	if name := strings.TrimSpace(configured); name != "" {
		return name, nil
	}
	// The definition's own identity, never its path or filename: both are
	// spellings a foreign definition can also be given. See #706.
	if identity != defaultInstanceIdentity {
		return "", fmt.Errorf(
			"SIRENS_ECHO_INSTANCE is required when the definition is %q rather "+
				"than %q: defaulting to %q would report this profile as Echo",
			identity, defaultInstanceIdentity, defaultInstanceName,
		)
	}
	return defaultInstanceName, nil
}

// LoadConfig loads the Sirens Echo deployment from environment and its
// source-controlled definition. Secrets never have source defaults.
func LoadConfig() (Config, error) {
	// Applied before anything reads a tuning number, so a derived value is
	// never computed from a default the deployment replaced.
	applyTuningOverrides(os.Getenv)
	definitionPath := valueOrDefault(os.Getenv("SIRENS_ECHO_DEFINITION"), defaultDefinitionPath)
	definition, err := LoadDefinition(definitionPath)
	if err != nil {
		return Config{}, err
	}
	instanceName, err := resolveInstanceName(definition.Identity, os.Getenv("SIRENS_ECHO_INSTANCE"))
	if err != nil {
		return Config{}, err
	}
	discordEnabled, err := boolOrDefault(os.Getenv("SIRENS_ECHO_DISCORD_ENABLED"), true)
	if err != nil {
		return Config{}, fmt.Errorf("SIRENS_ECHO_DISCORD_ENABLED: %w", err)
	}
	dmEnabled, err := boolOrDefault(os.Getenv("SIRENS_ECHO_DISCORD_DM_ENABLED"), false)
	if err != nil {
		return Config{}, fmt.Errorf("SIRENS_ECHO_DISCORD_DM_ENABLED: %w", err)
	}
	commandsEnabled, err := boolOrDefault(os.Getenv("SIRENS_ECHO_DISCORD_COMMANDS"), false)
	if err != nil {
		return Config{}, fmt.Errorf("SIRENS_ECHO_DISCORD_COMMANDS: %w", err)
	}
	requestTimeout, err := durationOrDefault(
		os.Getenv("SIRENS_ECHO_REQUEST_TIMEOUT"),
		defaultRequestTimeout,
	)
	if err != nil {
		return Config{}, fmt.Errorf("SIRENS_ECHO_REQUEST_TIMEOUT: %w", err)
	}
	queueTimeout, err := durationOrDefault(
		os.Getenv("SIRENS_ECHO_QUEUE_TIMEOUT"),
		defaultQueueTimeout,
	)
	if err != nil {
		return Config{}, fmt.Errorf("SIRENS_ECHO_QUEUE_TIMEOUT: %w", err)
	}
	shutdownGrace, err := durationOrDefault(
		os.Getenv("SIRENS_ECHO_SHUTDOWN_GRACE"),
		defaultShutdownGrace,
	)
	if err != nil {
		return Config{}, fmt.Errorf("SIRENS_ECHO_SHUTDOWN_GRACE: %w", err)
	}
	rateLimit, err := loadRateLimitPolicy()
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Definition:     definition,
		DefinitionPath: definitionPath,
		InstanceName:   instanceName,
		Principal: Principal{
			Handle: strings.TrimSpace(os.Getenv("SIRENS_ECHO_PRINCIPAL_HANDLE")),
			UserID: strings.TrimSpace(os.Getenv("SIRENS_ECHO_PRINCIPAL_USER_ID")),
		},
		DiscordEnabled:         discordEnabled,
		DiscordToken:           strings.TrimSpace(os.Getenv("DISCORD_TOKEN")),
		DiscordChannelIDs:      splitList(os.Getenv("DISCORD_CHANNEL_ID")),
		DiscordGuildIDs:        splitList(os.Getenv("DISCORD_GUILD_IDS")),
		DiscordDMEnabled:       dmEnabled,
		DiscordCommandsEnabled: commandsEnabled,
		AgentProxyURL:          valueOrDefault(os.Getenv("AGENT_PROXY_URL"), DefaultAgentProxyURL),
		AgentProxyModel:        strings.TrimSpace(os.Getenv("AGENT_PROXY_MODEL")),
		OTLPEndpoint:           valueOrDefault(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"), DefaultOTLPEndpoint),
		HTTPListenAddr:         valueOrDefault(os.Getenv("SIRENS_ECHO_HTTP_ADDR"), defaultHTTPListenAddr),
		MCPRosterPath:          strings.TrimSpace(os.Getenv("SIRENS_ECHO_MCP_ROSTER")),
		AccessPolicyPath:       strings.TrimSpace(os.Getenv("SIRENS_ECHO_ACCESS_POLICY")),
		ContentClassesPath:     strings.TrimSpace(os.Getenv("SIRENS_ECHO_CONTENT_CLASSES")),
		HTTPTrustToken:         strings.TrimSpace(os.Getenv("SIRENS_ECHO_HTTP_TOKEN")),
		FetchHosts:             fetchHosts(os.Getenv("SIRENS_ECHO_FETCH_HOSTS")),
		SandboxLabelID:         positiveInt(os.Getenv("SIRENS_ECHO_SANDBOX_LABEL")),
		JobStoreDir:            strings.TrimSpace(os.Getenv("SIRENS_ECHO_JOB_STORE")),
		RepoInventoryURL:       strings.TrimSpace(os.Getenv("SIRENS_ECHO_REPO_INVENTORY_URL")),
		RepoInventoryOrg:       strings.TrimSpace(os.Getenv("SIRENS_ECHO_REPO_INVENTORY_ORG")),
		ScratchDir:             strings.TrimSpace(os.Getenv("SIRENS_ECHO_SCRATCH")),
		PhrasesPath:            strings.TrimSpace(os.Getenv("SIRENS_ECHO_PHRASES")),
		RequestTimeout:         requestTimeout,
		QueueTimeout:           queueTimeout,
		ShutdownGrace:          shutdownGrace,
		RateLimit:              rateLimit,
	}
	if !mcpServerNamePattern.MatchString(cfg.InstanceName) {
		return Config{}, fmt.Errorf("SIRENS_ECHO_INSTANCE must be a lowercase service name")
	}
	if err := validatePrincipal(cfg.Principal); err != nil {
		return Config{}, err
	}
	if cfg.Definition.Composed {
		path, err := resolveBundlePath()
		if err != nil {
			return Config{}, err
		}
		cfg.BundlePath = path
	}
	for _, id := range append(append([]string{}, cfg.DiscordChannelIDs...), cfg.DiscordGuildIDs...) {
		if !discordSnowflake.MatchString(id) {
			return Config{}, fmt.Errorf("Discord IDs must be numeric snowflakes, got %q", id)
		}
	}
	missing := make([]string, 0, 5)
	if cfg.DiscordEnabled {
		if cfg.DiscordToken == "" {
			missing = append(missing, "DISCORD_TOKEN")
		}
		// The access policy file supplies scope on its own. Otherwise a
		// channel list is required unless direct messages are the only ingress.
		if cfg.AccessPolicyPath == "" &&
			len(cfg.DiscordChannelIDs) == 0 && !cfg.DiscordDMEnabled {
			missing = append(missing, "DISCORD_CHANNEL_ID")
		}
	}
	if cfg.AgentProxyModel == "" {
		missing = append(missing, "AGENT_PROXY_MODEL")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required env: %v", missing)
	}
	return cfg, nil
}

// validatePrincipal rejects a half-configured principal, which would otherwise
// render a sentence naming one signal and an empty string for the other.
func validatePrincipal(principal Principal) error {
	if principal.Handle == "" && principal.UserID == "" {
		return nil
	}
	if !principal.Configured() {
		return fmt.Errorf(
			"SIRENS_ECHO_PRINCIPAL_HANDLE and SIRENS_ECHO_PRINCIPAL_USER_ID must be set together",
		)
	}
	if !discordHandlePattern.MatchString(principal.Handle) {
		return fmt.Errorf(
			"SIRENS_ECHO_PRINCIPAL_HANDLE must be a Discord username, got %q",
			principal.Handle,
		)
	}
	if !discordSnowflake.MatchString(principal.UserID) {
		return fmt.Errorf(
			"SIRENS_ECHO_PRINCIPAL_USER_ID must be a numeric snowflake, got %q",
			principal.UserID,
		)
	}
	return nil
}

// resolveBundlePath selects the baked bundle for the deployment's role, with no
// default now that two lanes compose. See docs/sirens-echo-config.md.
func resolveBundlePath() (string, error) {
	role := strings.TrimSpace(os.Getenv("SIRENS_ECHO_ROLE"))
	if role == "" {
		return "", fmt.Errorf("a composing definition requires SIRENS_ECHO_ROLE to name a role slug")
	}
	if !composedRolePattern.MatchString(role) {
		return "", fmt.Errorf("SIRENS_ECHO_ROLE must be a lowercase role slug, got %q", role)
	}
	dir := valueOrDefault(strings.TrimSpace(os.Getenv("SIRENS_ECHO_BUNDLE_DIR")), defaultBundleDir)
	path := filepath.Join(dir, role)
	if _, err := os.Stat(filepath.Join(path, "manifest.json")); err != nil {
		return "", fmt.Errorf("no composed bundle for role %q under %s: %w", role, dir, err)
	}
	return path, nil
}

// LoadDefinition reads and validates the repository-owned agent definition.
func LoadDefinition(path string) (Definition, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, fmt.Errorf("read agent definition: %w", err)
	}
	var definition Definition
	if err := yaml.Unmarshal(raw, &definition); err != nil {
		return Definition{}, fmt.Errorf("parse agent definition: %w", err)
	}
	if definition.Schema != "coilyco-harness.agent.v1" {
		return Definition{}, fmt.Errorf("unsupported agent definition schema %q", definition.Schema)
	}
	if definition.Identity == "" || definition.AuditRole == "" {
		return Definition{}, fmt.Errorf("agent definition requires identity and audit_role")
	}
	// Channel is the prompt's boundary label, not the routing key. Deployment
	// owns routing through DISCORD_CHANNEL_ID.
	if definition.Channel != "" && !channelLabelPattern.MatchString(definition.Channel) {
		return Definition{}, fmt.Errorf(
			"agent definition channel must be empty or a #channel-name, got %q",
			definition.Channel,
		)
	}
	if definition.ResponseStyle != ResponseStyleNeutral &&
		definition.ResponseStyle != ResponseStyleSocial {
		return Definition{}, fmt.Errorf("unsupported response_style %q", definition.ResponseStyle)
	}
	if definition.MaxContextMessages < 1 || definition.MaxContextMessages > 50 {
		return Definition{}, fmt.Errorf("max_context_messages must be between 1 and 50")
	}
	if len(definition.LocalSkillRoots) == 0 {
		return Definition{}, fmt.Errorf("agent definition requires at least one local skill root")
	}
	// The roster is deployment-owned, so whether this names a live server is
	// checked where both are known rather than here.
	if definition.IssueTracker != "" &&
		!mcpServerNamePattern.MatchString(definition.IssueTracker) {
		return Definition{}, fmt.Errorf("invalid issue_tracker %q", definition.IssueTracker)
	}
	if err := definition.ModelBudget.validate(); err != nil {
		return Definition{}, err
	}
	return definition, nil
}

// validateMCPServer checks that an entry carries the fields its transport needs
// and none belonging to another. It makes no judgement about which server runs.
func validateMCPServer(server MCPServerDefinition) error {
	hasURL := strings.TrimSpace(server.URL) != ""
	hasCommand := strings.TrimSpace(server.Command) != ""
	switch server.ResolvedTransport() {
	case MCPTransportStreamable, MCPTransportSSE:
		// An unset interpolation variable lands here as an empty endpoint.
		if !hasURL {
			return fmt.Errorf("MCP server %q requires a baseUrl", server.Name)
		}
		if !validHTTPURL(server.URL) {
			return fmt.Errorf("MCP server %q has invalid baseUrl", server.Name)
		}
		if hasCommand || len(server.Args) > 0 || len(server.Env) > 0 {
			return fmt.Errorf("MCP server %q takes no command, args, or env", server.Name)
		}
		for name, value := range server.Headers {
			if !headerNamePattern.MatchString(name) {
				return fmt.Errorf("MCP server %q has invalid header name %q", server.Name, name)
			}
			// An unset variable lands here empty, and sending it would fail as
			// the vendor's anonymous-call error rather than as a roster one.
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("MCP server %q has empty header %q", server.Name, name)
			}
		}
	case MCPTransportStdio:
		if !hasCommand {
			return fmt.Errorf("MCP server %q requires a command", server.Name)
		}
		if hasURL {
			return fmt.Errorf("MCP server %q takes no baseUrl", server.Name)
		}
		if len(server.Headers) > 0 {
			return fmt.Errorf("MCP server %q takes no headers", server.Name)
		}
		for name := range server.Env {
			if !environmentNamePattern.MatchString(name) {
				return fmt.Errorf("MCP server %q has invalid env name %q", server.Name, name)
			}
		}
	default:
		return fmt.Errorf("MCP server %q has unsupported transport %q", server.Name, server.Transport)
	}
	return nil
}

func validHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func valueOrDefault(value, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}

// splitList parses a comma-separated deployment list, dropping empty entries so
// a trailing comma is not a configuration error.
func splitList(value string) []string {
	items := make([]string, 0, 4)
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func durationOrDefault(value string, fallback time.Duration) (time.Duration, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, fmt.Errorf("must be a Go duration such as 90s")
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("must be greater than zero")
	}
	return parsed, nil
}

// loadRateLimitPolicy overlays deployment overrides onto the packaged
// defaults. See docs/sirens-echo-admission.md for the format.
func loadRateLimitPolicy() (RateLimitPolicy, error) {
	policy := defaultRateLimitPolicy
	tiers := []struct {
		name   string
		target *RateLimit
	}{
		{"SIRENS_ECHO_RATE_USER", &policy.PerUser},
		{"SIRENS_ECHO_RATE_CONTEXT", &policy.PerContext},
		{"SIRENS_ECHO_RATE_GLOBAL", &policy.Global},
	}
	for _, tier := range tiers {
		limit, ok, err := parseRateLimit(os.Getenv(tier.name))
		if err != nil {
			return RateLimitPolicy{}, fmt.Errorf("%s: %w", tier.name, err)
		}
		if ok {
			*tier.target = limit
		}
	}
	pending, err := intOrDefault(os.Getenv("SIRENS_ECHO_MAX_PENDING"), policy.MaxPending)
	if err != nil {
		return RateLimitPolicy{}, fmt.Errorf("SIRENS_ECHO_MAX_PENDING: %w", err)
	}
	policy.MaxPending = pending
	notify, err := durationOrDefault(
		os.Getenv("SIRENS_ECHO_RATE_NOTIFY_EVERY"),
		policy.NotifyEvery,
	)
	if err != nil {
		return RateLimitPolicy{}, fmt.Errorf("SIRENS_ECHO_RATE_NOTIFY_EVERY: %w", err)
	}
	policy.NotifyEvery = notify
	return policy, nil
}

func parseRateLimit(value string) (RateLimit, bool, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return RateLimit{}, false, nil
	}
	if trimmed == "off" {
		return RateLimit{}, true, nil
	}
	match := rateLimitPattern.FindStringSubmatch(trimmed)
	if match == nil {
		return RateLimit{}, false, fmt.Errorf(`must be "<burst>/<interval>" such as 3/30s, or "off"`)
	}
	burst, err := strconv.Atoi(match[1])
	if err != nil || burst < 1 {
		return RateLimit{}, false, fmt.Errorf("burst must be a positive integer")
	}
	every, err := time.ParseDuration(match[2])
	if err != nil || every <= 0 {
		return RateLimit{}, false, fmt.Errorf("interval must be a positive Go duration such as 30s")
	}
	return RateLimit{Burst: burst, Every: every}, true, nil
}

func intOrDefault(value string, fallback int) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("must be a non-negative integer")
	}
	return parsed, nil
}

func boolOrDefault(value string, fallback bool) (bool, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(trimmed)
	if err != nil {
		return false, fmt.Errorf("must be true or false")
	}
	return parsed, nil
}
