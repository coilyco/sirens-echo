// Offline validation of an agent definition against the skill tree this image
// carries, for a caller that cannot import Go. See docs/sirens-echo-compose.md.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/coilyco/sirens-echo/internal/community"
)

// usage names the argument rather than describing the tool, because the caller
// is a CI step that already knows what it invoked.
const usage = "usage: sirens-echo-definition-check <definition.yaml|-> [...]"

// stdinPath is the argument deploy uses, because its definitions live inside a
// ConfigMap and only the extracted key is a definition.
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
		summary, err := check(path)
		if err != nil {
			// The path is repeated because a CI log shows one line, and which
			// file failed is the first thing the reader needs.
			fmt.Fprintf(stderr, "%s: %v\n", path, err)
			failed = true
			continue
		}
		fmt.Fprintf(stdout, "%s: ok\n%s", path, summary)
	}
	if failed {
		return 1
	}
	return 0
}

// check runs the loader and the skillpack walk the runtime runs, so what
// passes here is what the pod accepts. A second parser would be a worse gate.
func check(path string) (string, error) {
	if path == stdinPath {
		return checkStdin()
	}
	definition, err := community.LoadDefinition(path)
	if err != nil {
		return "", err
	}
	// The exact call that crashlooped the dowel lane for 90 minutes, run
	// against the tree this image carries. See sirens-echo#973.
	pack, err := community.LoadSkillpack(definition.LocalSkillRoots)
	if err != nil {
		return "", err
	}
	references, err := community.LoadSkillReferences(definition.LocalSkillRoots)
	if err != nil {
		return "", err
	}
	return summarize(definition, pack, references), nil
}

// summarize reports what the definition resolved to, not only that it did. A
// root can exist and still be the wrong one.
func summarize(
	definition community.Definition,
	pack string,
	references []community.SkillReference,
) string {
	var out strings.Builder
	fmt.Fprintf(&out, "  identity: %s\n", definition.Identity)
	fmt.Fprintf(&out, "  local skill roots (%d):\n", len(definition.LocalSkillRoots))
	for _, root := range definition.LocalSkillRoots {
		fmt.Fprintf(&out, "    %s\n", root)
	}
	fmt.Fprintf(&out, "  inline policy: %d bytes\n", len(pack))
	fmt.Fprintf(&out, "  readable skills: %d\n", len(references))
	return out.String()
}

// checkStdin spools the piped definition to a file, so the runtime's loader
// stays the only parser rather than gaining a second entry point.
func checkStdin() (string, error) {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	spooled, err := os.CreateTemp("", "agent-definition-*.yaml")
	if err != nil {
		return "", fmt.Errorf("spool stdin: %w", err)
	}
	defer os.Remove(spooled.Name())
	if _, err := spooled.Write(raw); err != nil {
		spooled.Close()
		return "", fmt.Errorf("spool stdin: %w", err)
	}
	if err := spooled.Close(); err != nil {
		return "", fmt.Errorf("spool stdin: %w", err)
	}
	return check(spooled.Name())
}
