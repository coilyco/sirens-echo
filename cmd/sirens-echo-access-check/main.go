// Offline validation of an access policy, for a caller that cannot import Go.
// Reads files and stdin and nothing else. See docs/sirens-echo-access.md.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/coilyco/sirens-echo/internal/community"
)

// usage names the argument rather than describing the tool, because the caller
// is a CI step that already knows what it invoked.
const usage = "usage: sirens-echo-access-check <access-policy.yaml|-> [...]"

// stdinPath is the argument deploy uses, because its policies live inside a
// ConfigMap and only the extracted key is a policy.
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
		policy, err := check(path)
		if err != nil {
			// The path is repeated because a CI log shows one line, and which
			// file failed is the first thing the reader needs.
			fmt.Fprintf(stderr, "%s: %v\n", path, err)
			failed = true
			continue
		}
		fmt.Fprintf(stdout, "%s: ok\n", path)
		// What it admits, not only that it loaded. A policy can be valid and
		// still open a guild the reviewer did not mean to open.
		fmt.Fprint(stdout, community.RenderAccessSummary(policy))
	}
	if failed {
		return 1
	}
	return 0
}

// check runs the same loader the runtime does, so this cannot drift from what
// the pod will accept. A second implementation would be a worse gate than none.
func check(path string) (*community.AccessPolicy, error) {
	if path == stdinPath {
		return checkStdin()
	}
	policy, err := community.LoadAccessPolicy(path)
	if err != nil {
		return nil, err
	}
	if policy == nil {
		return nil, fmt.Errorf("loaded no policy")
	}
	return policy, nil
}

// checkStdin spools the piped policy to a file, so the runtime's loader stays
// the only parser rather than gaining a second entry point.
func checkStdin() (*community.AccessPolicy, error) {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	spooled, err := os.CreateTemp("", "access-policy-*.yaml")
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
