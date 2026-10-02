package community

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const slackAccessSchema = "coilyco-harness.slack-access.v1"

var (
	slackWorkspaceID = regexp.MustCompile(`^T[A-Z0-9]{2,}$`)
	slackChannelID   = regexp.MustCompile(`^[CG][A-Z0-9]{2,}$`)
	slackUserID      = regexp.MustCompile(`^[UW][A-Z0-9]{2,}$`)
)

// SlackWorkspaceAccess is one workspace's entry. Note is a review aid for a
// diff of opaque ids and is never matched on.
type SlackWorkspaceAccess struct {
	ID       string          `yaml:"id"`
	Note     string          `yaml:"note"`
	Channels Allowlist       `yaml:"channels"`
	Users    Allowlist       `yaml:"users"`
	Rate     *GuildRateLimit `yaml:"rate_limit"`

	overrides *RateLimitOverride
}

// SlackAccessPolicy is the deployment-owned Slack allowlist, a separate schema
// so the Discord policy is untouched. See docs/sirens-echo-transports.md.
type SlackAccessPolicy struct {
	Schema         string                 `yaml:"schema"`
	Deny           DenyList               `yaml:"deny"`
	DirectMessages DirectMessageAccess    `yaml:"direct_messages"`
	Workspaces     []SlackWorkspaceAccess `yaml:"workspaces"`

	byWorkspace map[string]*SlackWorkspaceAccess
}

// LoadSlackAccessPolicy reads and validates the Slack allowlist file.
func LoadSlackAccessPolicy(path string) (*SlackAccessPolicy, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read slack access policy: %w", err)
	}
	var policy SlackAccessPolicy
	decoder := yaml.NewDecoder(strings.NewReader(string(raw)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&policy); err != nil {
		return nil, fmt.Errorf("parse slack access policy: %w", err)
	}
	if err := policy.validate(); err != nil {
		return nil, err
	}
	return &policy, nil
}

// validate fails closed. A malformed or empty policy must stop the process
// rather than widen the surface it was written to narrow.
func (p *SlackAccessPolicy) validate() error {
	if p.Schema != slackAccessSchema {
		return fmt.Errorf("unsupported slack access policy schema %q", p.Schema)
	}
	if len(p.Workspaces) == 0 {
		return fmt.Errorf("slack access policy grants nothing: list a workspace")
	}
	for _, id := range append(append([]string{}, p.Deny.Users...), p.DirectMessages.Allow...) {
		if !slackUserID.MatchString(id) {
			return fmt.Errorf("slack access policy user ids look like U0123ABCD, got %q", id)
		}
	}
	p.byWorkspace = make(map[string]*SlackWorkspaceAccess, len(p.Workspaces))
	for index := range p.Workspaces {
		workspace := &p.Workspaces[index]
		if err := workspace.validate(); err != nil {
			return err
		}
		if _, exists := p.byWorkspace[workspace.ID]; exists {
			return fmt.Errorf("slack access policy lists workspace %q twice", workspace.ID)
		}
		p.byWorkspace[workspace.ID] = workspace
	}
	return nil
}

func (w *SlackWorkspaceAccess) validate() error {
	if !slackWorkspaceID.MatchString(w.ID) {
		return fmt.Errorf("slack workspace ids look like T0123ABCD, got %q", w.ID)
	}
	if !w.Channels.All {
		for _, id := range w.Channels.IDs {
			if !slackChannelID.MatchString(id) {
				return fmt.Errorf("slack channel ids look like C0123ABCD, got %q", id)
			}
		}
	}
	if !w.Users.All {
		for _, id := range w.Users.IDs {
			if !slackUserID.MatchString(id) {
				return fmt.Errorf("slack user ids look like U0123ABCD, got %q", id)
			}
		}
	}
	if w.Channels.empty() {
		return fmt.Errorf("workspace %q allows no channel: use `all` or list channel ids", w.ID)
	}
	if w.Users.empty() {
		return fmt.Errorf("workspace %q allows no member: use `all` or list users", w.ID)
	}
	if err := w.resolveOverrides(); err != nil {
		return err
	}
	if w.Users.All && (w.overrides == nil || w.overrides.PerUser == nil || !w.overrides.PerUser.enabled()) {
		return fmt.Errorf(
			"workspace %q opens to every member: set rate_limit.per_user to a real bound such as 1/60s",
			w.ID,
		)
	}
	return nil
}

func (w *SlackWorkspaceAccess) resolveOverrides() error {
	if w.Rate == nil {
		return nil
	}
	override := &RateLimitOverride{}
	for _, tier := range []struct {
		raw    string
		target **RateLimit
	}{
		{w.Rate.PerUser, &override.PerUser},
		{w.Rate.PerContext, &override.PerContext},
	} {
		limit, ok, err := parseRateLimit(tier.raw)
		if err != nil {
			return fmt.Errorf("workspace %q rate_limit: %w", w.ID, err)
		}
		if ok {
			value := limit
			*tier.target = &value
		}
	}
	w.overrides = override
	return nil
}

// Overrides returns the per-workspace admission overrides, or nil for the
// deployment defaults.
func (w *SlackWorkspaceAccess) Overrides() *RateLimitOverride {
	if w == nil {
		return nil
	}
	return w.overrides
}

// Evaluate applies the gate from the event payload alone. A direct message
// still needs its workspace listed, so a foreign workspace cannot reach it.
func (p *SlackAccessPolicy) Evaluate(
	workspaceID, channelID, userID string, direct bool,
) (accessReason, *SlackWorkspaceAccess) {
	for _, blocked := range p.Deny.Users {
		if blocked == userID {
			return accessDeniedBlocked, nil
		}
	}
	workspace := p.byWorkspace[workspaceID]
	if workspace == nil {
		return accessDeniedGuild, nil
	}
	if direct {
		for _, allowed := range p.DirectMessages.Allow {
			if allowed == userID {
				return accessAllowed, workspace
			}
		}
		return accessDeniedDM, workspace
	}
	if !workspace.Users.Permits(userID) {
		return accessDeniedMember, workspace
	}
	if !workspace.Channels.Permits(channelID) {
		return accessDeniedChannel, workspace
	}
	return accessAllowed, workspace
}

// missingSlackConfig names what a Slack deployment lacks. Absent configuration
// never widens the surface, so each of these stops the process.
func missingSlackConfig(cfg Config) []string {
	var missing []string
	if !strings.HasPrefix(cfg.SlackBotToken, "xoxb-") {
		missing = append(missing, "SLACK_BOT_TOKEN (an xoxb- bot token)")
	}
	if !strings.HasPrefix(cfg.SlackAppToken, "xapp-") {
		missing = append(missing, "SLACK_APP_TOKEN (an xapp- app-level token)")
	}
	if cfg.SlackAccessPolicyPath == "" {
		missing = append(missing, "SIRENS_ECHO_SLACK_ACCESS_POLICY")
	}
	return missing
}

// RenderSlackAccessSummary states what a loaded Slack policy admits. It reads
// the resolved entries, so it cannot disagree with the runtime.
func RenderSlackAccessSummary(policy *SlackAccessPolicy) string {
	if policy == nil {
		return "no slack access policy\n"
	}
	var out strings.Builder
	fmt.Fprintf(&out, "schema           %s\n", policy.Schema)
	fmt.Fprintf(&out, "denied accounts  %s\n", countOrNone(len(policy.Deny.Users)))
	fmt.Fprintf(&out, "direct messages  %s\n", countOrNone(len(policy.DirectMessages.Allow)))
	fmt.Fprintf(&out, "workspaces       %s\n", countOrNone(len(policy.Workspaces)))
	for index := range policy.Workspaces {
		workspace := &policy.Workspaces[index]
		fmt.Fprintf(&out, "\nworkspace %s%s\n", workspace.ID, notedAs(workspace.Note))
		fmt.Fprintf(&out, "  channels     %s\n", describeAllowlist(workspace.Channels))
		fmt.Fprintf(&out, "  members      %s\n", describeAllowlist(workspace.Users))
		fmt.Fprintf(&out, "  per user     %s\n", describeTier(workspace.Overrides(), tierPerUser))
		fmt.Fprintf(&out, "  per context  %s\n", describeTier(workspace.Overrides(), tierPerContext))
		if workspace.Users.All {
			out.WriteString("  every member of this workspace is admitted\n")
		}
	}
	return out.String()
}
