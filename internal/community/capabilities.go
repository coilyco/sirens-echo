package community

import (
	"context"
	"log/slog"
	"strings"
)

// A capability the deployment did not switch on is inert rather than broken,
// which is why it needs saying out loud. See docs/sirens-echo-capability-report.md.

// capability names one optional surface and whether this process holds it.
type capability struct {
	Name       string
	Configured bool
}

// capabilityReport lists every optional surface in a fixed order. A name and a
// boolean only, so no configured value can reach a log.
func capabilityReport(cfg Config) []capability {
	return []capability{
		{"discord_ingress", cfg.DiscordEnabled},
		{"mcp_roster", strings.TrimSpace(cfg.MCPRosterPath) != ""},
		{"access_policy", strings.TrimSpace(cfg.AccessPolicyPath) != ""},
		{"content_classes", strings.TrimSpace(cfg.ContentClassesPath) != ""},
		{"job_store_durable", strings.TrimSpace(cfg.JobStoreDir) != ""},
		{"scratchpad", strings.TrimSpace(cfg.ScratchDir) != ""},
		{"issue_tracker", strings.TrimSpace(cfg.Definition.IssueTracker) != ""},
	}
}

// reportCapabilities says what this process can and cannot do, once, at boot.
// Establishing that cost a read of two repositories before this existed.
func (a *Agent) reportCapabilities(ctx context.Context) {
	report := capabilityReport(a.cfg)
	attributes := make([]slog.Attr, 0, len(report))
	for _, entry := range report {
		attributes = append(attributes, slog.Bool(entry.Name, entry.Configured))
	}
	a.telemetry.Info(ctx, "startup.capabilities", attributes...)
}
