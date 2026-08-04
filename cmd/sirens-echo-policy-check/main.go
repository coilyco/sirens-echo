package main

import (
	"fmt"
	"log"

	"forgejo.coilysiren.me/coilyco-gaming/sirens-echo/internal/community"
)

func main() {
	definition, err := community.LoadDefinition("agent/sirens-echo.json")
	if err != nil {
		log.Fatalf("definition: %v", err)
	}
	localPolicy, err := community.LoadSkillpack(definition.LocalSkillRoots)
	if err != nil {
		log.Fatalf("local policy: %v", err)
	}
	prompt := community.BuildSystemPrompt(definition, localPolicy)
	if err := community.ValidateNeutralSystemPrompt(prompt); err != nil {
		log.Fatalf("neutral policy: %v", err)
	}
	fmt.Printf(
		"verified neutral model policy with %d bytes and %d local roots\n",
		len(prompt),
		len(definition.LocalSkillRoots),
	)
}
