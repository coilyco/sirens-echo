// Offline validation of an MCP roster, for a caller that cannot import Go.
// Reads files and stdin and nothing else. See docs/sirens-echo-mcp.md.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"forgejo.coilysiren.me/coilyco-gaming/sirens-echo/internal/community"
)

// usage names the argument rather than describing the tool, because the caller
// is a CI step that already knows what it invoked.
const usage = "usage: sirens-echo-roster-check <roster.yaml|-> [...]"

// stdinPath is the argument deploy uses, because its rosters live inside a
// ConfigMap and only the extracted key is a roster.
const stdinPath = "-"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run returns the code rather than calling os.Exit, because the codes are what
// deploy's CI keys on and an untestable main cannot prove them.
func run(paths []string, stdout, stderr io.Writer) int {
	if len(paths) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	failed := false
	for _, path := range paths {
		report, err := check(path)
		if err != nil {
			// The path is repeated because a CI log shows one line, and which
			// file failed is the first thing the reader needs.
			fmt.Fprintf(stderr, "%s: %v\n", path, err)
			failed = true
			continue
		}
		// Every bad entry, not the first. The outage that asked for this tool
		// had two, and a check that stops at one costs a second round trip.
		for _, issue := range report.Issues {
			fmt.Fprintf(stderr, "%s: %v\n", path, issue)
		}
		if len(report.Issues) > 0 {
			failed = true
			continue
		}
		fmt.Fprintf(stdout, "%s: ok, %d server(s): %s\n",
			path, len(report.Servers), strings.Join(names(report.Servers), " "))
		// What it could not verify, so a green line is not read as more than it
		// is. Every shipped roster interpolates its endpoints from extraEnv.
		if len(report.Unresolved) > 0 {
			fmt.Fprintf(stdout, "%s: %d endpoint value(s) unresolved here: %s\n",
				path, len(report.Unresolved), strings.Join(report.Unresolved, " "))
		}
	}
	if failed {
		return 1
	}
	return 0
}

func names(servers []community.MCPServerDefinition) []string {
	out := make([]string, 0, len(servers))
	for _, server := range servers {
		out = append(out, server.Name)
	}
	return out
}

// check runs the same loader the runtime does, so this cannot drift from what
// the pod will accept. A second implementation would be a worse gate than none.
func check(path string) (*community.MCPRosterReport, error) {
	if path == stdinPath {
		return checkStdin()
	}
	return community.CheckMCPRoster(path)
}

// checkStdin spools the piped roster to a file, so the runtime's loader stays
// the only parser rather than gaining a second entry point.
func checkStdin() (*community.MCPRosterReport, error) {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	spooled, err := os.CreateTemp("", "mcp-roster-*.yaml")
	if err != nil {
		return nil, fmt.Errorf("spool stdin: %w", err)
	}
	defer os.Remove(spooled.Name())
	if _, err := spooled.Write(raw); err != nil {
		spooled.Close()
		return nil, fmt.Errorf("spool stdin: %w", err)
	}
	if err := spooled.Close(); err != nil {
		return nil, fmt.Errorf("spool stdin: %w", err)
	}
	return check(spooled.Name())
}
