package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The gate deploy cannot build itself. See sirens-echo#6805.

func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "roster.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

// The shape every shipped roster has: names only, endpoints interpolated.
const interpolated = `mcpServers:
  discord:
    url: "${SIRENS_ECHO_DISCORD_MCP_URL}"
  open-meteo:
    url: "${SIRENS_ECHO_OPEN_METEO_MCP_URL}"
`

// The outage: coilyco-bridge/deploy@50a4b61d added two underscored names and
// both lanes crashlooped for 21 hours. Two, so one pass must report both.
const underscored = `mcpServers:
  open_meteo:
    url: "${SIRENS_ECHO_OPEN_METEO_MCP_URL}"
  open_meteo_marine:
    url: "${SIRENS_ECHO_OPEN_METEO_MARINE_MCP_URL}"
  discord:
    url: "${SIRENS_ECHO_DISCORD_MCP_URL}"
`

func run1(t *testing.T, path string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run([]string{path}, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// The false positive that would have this switched off inside a week. Every URL
// in the shipped rosters is a ${VAR} filled from extraEnv, and CI has none.
func TestAnInterpolatedRosterPassesWithNoEnvironment(t *testing.T) {
	code, stdout, stderr := run1(t, write(t, interpolated))
	if code != 0 {
		t.Fatalf("a valid roster failed: code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "2 server(s)") {
		t.Fatalf("expected both servers reported, got %q", stdout)
	}
}

// Saying so is the difference between a green line that means "checked" and one
// that means "checked what it could".
func TestUnresolvedVariablesAreNamedRatherThanImplied(t *testing.T) {
	_, stdout, _ := run1(t, write(t, interpolated))
	if !strings.Contains(stdout, "2 endpoint value(s) unresolved here") {
		t.Fatalf("expected the unresolved count, got %q", stdout)
	}
	for _, want := range []string{
		"SIRENS_ECHO_DISCORD_MCP_URL", "SIRENS_ECHO_OPEN_METEO_MCP_URL",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %s named, got %q", want, stdout)
		}
	}
}

func TestTheOutageRosterFails(t *testing.T) {
	code, _, stderr := run1(t, write(t, underscored))
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr, `"open_meteo"`) {
		t.Fatalf("expected the offending name, got %q", stderr)
	}
}

// One round trip, not one per bad entry. The outage had two.
func TestEveryInvalidEntryIsReportedInOnePass(t *testing.T) {
	_, _, stderr := run1(t, write(t, underscored))
	for _, want := range []string{`"open_meteo"`, `"open_meteo_marine"`} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("expected %s reported, got %q", want, stderr)
		}
	}
}

// "Reporting the offending name and the pattern it failed would have made this
// a 30-second fix" is the record's own sentence.
func TestTheFailureNamesThePatternItFailed(t *testing.T) {
	_, _, stderr := run1(t, write(t, underscored))
	if !strings.Contains(stderr, "^[a-z][a-z0-9-]*$") {
		t.Fatalf("expected the pattern in the message, got %q", stderr)
	}
}

// The rosters live inside ConfigMaps, so this is the likely first mistake.
func TestAConfigMapRatherThanARosterIsRejected(t *testing.T) {
	code, _, stderr := run1(t, write(t, `apiVersion: v1
kind: ConfigMap
metadata:
  name: sirens-echo-mcp-roster
data:
  roster.yaml: |
    mcpServers:
      discord:
        url: "https://example.invalid/mcp"
`))
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr, "ConfigMap that carries it") {
		t.Fatalf("expected the ConfigMap hint, got %q", stderr)
	}
}

func TestNoArgumentsIsAUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("expected exit 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Fatalf("expected usage, got %q", stderr.String())
	}
}

// deploy pipes the extracted ConfigMap key rather than a file.
func TestStdinIsAccepted(t *testing.T) {
	path := write(t, interpolated)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	piped := filepath.Join(t.TempDir(), "piped")
	if err := os.WriteFile(piped, body, 0o600); err != nil {
		t.Fatalf("write piped: %v", err)
	}
	handle, err := os.Open(piped)
	if err != nil {
		t.Fatalf("open piped: %v", err)
	}
	defer handle.Close()
	saved := os.Stdin
	os.Stdin = handle
	defer func() { os.Stdin = saved }()

	var stdout, stderr bytes.Buffer
	if code := run([]string{stdinPath}, &stdout, &stderr); code != 0 {
		t.Fatalf("stdin roster failed: code=%d stderr=%s", code, stderr.String())
	}
}

// A real value must still win, or the check would pass a roster the pod cannot
// use. The placeholder is a fallback, not a replacement.
func TestASuppliedValueIsUsedOverThePlaceholder(t *testing.T) {
	t.Setenv("SIRENS_ECHO_DISCORD_MCP_URL", "not a url")
	code, _, stderr := run1(t, write(t, `mcpServers:
  discord:
    url: "${SIRENS_ECHO_DISCORD_MCP_URL}"
`))
	if code != 1 {
		t.Fatalf("expected the supplied bad value to fail, got %d", code)
	}
	if !strings.Contains(stderr, "invalid baseUrl") {
		t.Fatalf("expected the url rejected, got %q", stderr)
	}
}
