package main

import (
	"fmt"
	"log"

	"forgejo.coilysiren.me/coilyco-gaming/sirens-echo/internal/community"
)

func main() {
	for _, path := range []string{
		"agent/sirens-echo.yaml",
		"agent/sirens-deep.yaml",
	} {
		verify(path)
	}
}

func verify(path string) {
	definition, err := community.LoadDefinition(path)
	if err != nil {
		log.Fatalf("definition %s: %v", path, err)
	}
	localPolicy, err := community.LoadSkillpack(definition.LocalSkillRoots)
	if err != nil {
		log.Fatalf("local policy %s: %v", path, err)
	}
	prompt := community.BuildSystemPrompt(definition, localPolicy)
	if err := community.ValidateSystemPrompt(definition, prompt); err != nil {
		log.Fatalf("response policy %s: %v", path, err)
	}
	fmt.Printf(
		"verified %s response policy with %d bytes and %d local roots\n",
		definition.ResponseStyle,
		len(prompt),
		len(definition.LocalSkillRoots),
	)
}
