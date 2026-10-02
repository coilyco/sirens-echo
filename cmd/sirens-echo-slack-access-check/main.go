// Offline validation of a Slack access policy, for a caller that cannot import
// Go. Reads files and stdin and nothing else. See docs/sirens-echo-transports.md.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/coilyco/sirens-echo/internal/community"
)

const usage = "usage: sirens-echo-slack-access-check <slack-access-policy.yaml|-> [...]"

// stdinPath is the argument deploy uses, because its policies live inside a
// ConfigMap and only the extracted key is a policy.
const stdinPath = "-"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run returns the code rather than calling os.Exit, because the codes are what
// deploy's CI keys on and an untestable main cannot prove them.
func run(paths []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(paths) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	failed := false
	for _, path := range paths {
		policy, err := check(path, stdin)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", path, err)
			failed = true
			continue
		}
		fmt.Fprintf(stdout, "%s: ok\n", path)
		_, _ = fmt.Fprint(stdout, community.RenderSlackAccessSummary(policy))
	}
	if failed {
		return 1
	}
	return 0
}

// check runs the loader the runtime does, so it cannot drift from what the pod
// will accept.
func check(path string, stdin io.Reader) (*community.SlackAccessPolicy, error) {
	if path != stdinPath {
		return community.LoadSlackAccessPolicy(path)
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	spooled, err := os.CreateTemp("", "slack-access-policy-*.yaml")
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
	return community.LoadSlackAccessPolicy(spooled.Name())
}
