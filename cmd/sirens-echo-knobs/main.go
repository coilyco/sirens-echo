// Command sirens-echo-knobs writes the generated reference of every number a
// deployment may set. See docs/sirens-echo-tuning.md.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/coilyco/sirens-echo/internal/community"
)

const referencePath = "agent/rendered/knobs.txt"

func main() {
	check := flag.Bool("check", false, "fail when the tracked reference is stale")
	flag.Parse()
	rendered := community.RenderKnobReference()
	if *check {
		tracked, err := os.ReadFile(referencePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", referencePath, err)
			os.Exit(1)
		}
		if string(tracked) != rendered {
			fmt.Fprintf(os.Stderr,
				"%s is stale. Run `just knobs`.\n", referencePath)
			os.Exit(1)
		}
		return
	}
	if err := os.WriteFile(referencePath, []byte(rendered), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", referencePath, err)
		os.Exit(1)
	}
}
