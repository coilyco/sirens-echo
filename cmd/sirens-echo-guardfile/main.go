package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/coilyco/sirens-echo/internal/community"
)

// Generates the agent's own-authority skill from the deployed guardfile. See
// docs/sirens-echo-config.md.
const skillTarget = ".agents/skills/coilyco-general/references/guardfile.md"

func main() {
	source := flag.String("guardfile", "", "path to the deployed .mcp.kdl guardfile")
	check := flag.Bool("check", false, "fail on a stale skill instead of rewriting it")
	flag.Parse()

	if *source == "" {
		log.Fatal("--guardfile is required: it lives in coilyco-bridge/deploy")
	}
	guard, err := community.ParseGuardfile(*source)
	if err != nil {
		log.Fatal(err)
	}
	rendered := community.RenderGuardfileSkill(guard)
	existing, readErr := os.ReadFile(skillTarget)
	if readErr == nil && string(existing) == rendered {
		fmt.Printf("current %s (%d grants, digest %s)\n",
			skillTarget, len(guard.Grants), guard.Digest)
		return
	}
	if *check {
		log.Fatalf(
			"%s is stale against %s\nrun `just guardfile-skill` and commit the result",
			skillTarget, *source,
		)
	}
	if err := os.WriteFile(skillTarget, []byte(rendered), 0o644); err != nil {
		log.Fatalf("write %s: %v", skillTarget, err)
	}
	fmt.Printf("wrote %s (%d grants, digest %s)\n",
		skillTarget, len(guard.Grants), guard.Digest)
}
