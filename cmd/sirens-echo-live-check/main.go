// Commit-time refusal of a recorded live run, for pre-commit. Reads files and nothing
// else. See docs/sirens-echo-testing.md, Live eval output.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/coilyco/sirens-echo/internal/community"
)

const usage = "usage: sirens-echo-live-check <file> [...]"

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

// run returns the code rather than exiting, because pre-commit keys on it.
func run(paths []string, stderr io.Writer) int {
	if len(paths) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	failed := false
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			// A path pre-commit listed and cannot be read is a failure, not a pass.
			fmt.Fprintf(stderr, "%s: %v\n", path, err)
			failed = true
			continue
		}
		if community.HasLiveRunMarker(body) {
			// Every file, not the first: a commit with two needs both named.
			fmt.Fprintf(stderr, "%s: output of a run against a live roster, which carries real tool "+
				"output. Keep it untracked and paste it onto the private tracker record.\n", path)
			failed = true
		}
	}
	if failed {
		return 1
	}
	return 0
}
