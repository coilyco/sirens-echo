package main

import (
	"fmt"
	"log"
	"os"

	"forgejo.coilysiren.me/coilyco-gaming/sirens-echo/internal/community"
)

func main() {
	bundlePath := os.Getenv("SIRENS_ECHO_AOS_BUNDLE")
	if len(os.Args) == 2 {
		bundlePath = os.Args[1]
	}
	if len(os.Args) > 2 || bundlePath == "" {
		log.Fatal("usage: sirens-echo-bundle-check <bundle-directory>")
	}
	definition, err := community.LoadDefinition("agent/sirens-echo.json")
	if err != nil {
		log.Fatalf("definition: %v", err)
	}
	bundle, err := community.LoadAOSBundle(bundlePath, definition.Role)
	if err != nil {
		log.Fatalf("bundle: %v", err)
	}
	fmt.Printf(
		"verified AOS %s bundle with %d personalities and %d sources\n",
		bundle.Manifest.Role,
		len(bundle.Manifest.Personalities),
		len(bundle.Manifest.Sources),
	)
}
