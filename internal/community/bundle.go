package community

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxBundleContextBytes = 1024 * 1024

// BundleDelivery is the narrow agent-compose entry-point contract consumed by
// the Discord Community harness.
type BundleDelivery struct {
	Mode            string `json:"mode"`
	Instructions    string `json:"instructions"`
	SkillsRoot      string `json:"skills_root,omitempty"`
	CompiledContext string `json:"compiled_context,omitempty"`
}

// BundleManifest is the public agent-compose manifest schema.
type BundleManifest struct {
	Format        string         `json:"format"`
	Role          string         `json:"role"`
	ModelClass    string         `json:"model_class"`
	Personalities []string       `json:"personalities"`
	Color         string         `json:"color"`
	Sources       []string       `json:"sources"`
	Delivery      BundleDelivery `json:"delivery"`
}

// AOSBundle is the verified Community context consumed by Sirens Echo.
type AOSBundle struct {
	Manifest BundleManifest
	Context  string
}

// LoadAOSBundle validates the agent-compose manifest and loads its compiled
// context after the image build has run agent-compose verify.
func LoadAOSBundle(root, expectedRole string) (AOSBundle, error) {
	raw, err := readBundleFile(root, "manifest.json")
	if err != nil {
		return AOSBundle{}, fmt.Errorf("read AOS bundle manifest: %w", err)
	}
	var manifest BundleManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return AOSBundle{}, fmt.Errorf("parse AOS bundle manifest: %w", err)
	}
	if manifest.Format != "agent-compose.bundle" {
		return AOSBundle{}, fmt.Errorf("unsupported AOS bundle format %q", manifest.Format)
	}
	if manifest.Role != expectedRole {
		return AOSBundle{}, fmt.Errorf(
			"AOS bundle role %q does not match configured role %q",
			manifest.Role,
			expectedRole,
		)
	}
	if manifest.Delivery.Mode != "compiled" || manifest.Delivery.CompiledContext == "" {
		return AOSBundle{}, fmt.Errorf("AOS bundle must use compiled delivery")
	}
	if len(manifest.Personalities) == 0 {
		return AOSBundle{}, fmt.Errorf("AOS bundle contains no personality meld")
	}
	if !containsString(manifest.Sources, "aos-public") {
		return AOSBundle{}, fmt.Errorf("AOS bundle does not contain the aos-public provider")
	}
	contextRaw, err := readBundleFile(root, manifest.Delivery.CompiledContext)
	if err != nil {
		return AOSBundle{}, fmt.Errorf("read AOS compiled context: %w", err)
	}
	if len(contextRaw) == 0 {
		return AOSBundle{}, fmt.Errorf("AOS compiled context is empty")
	}
	if len(contextRaw) > maxBundleContextBytes {
		return AOSBundle{}, fmt.Errorf(
			"AOS compiled context exceeds %d bytes",
			maxBundleContextBytes,
		)
	}
	return AOSBundle{
		Manifest: manifest,
		Context:  strings.TrimSpace(string(contextRaw)),
	}, nil
}

func readBundleFile(root, relative string) ([]byte, error) {
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || filepath.IsAbs(clean) ||
		clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("unsafe bundle path %q", relative)
	}
	path := filepath.Join(root, clean)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("bundle path %q is not a regular file", relative)
	}
	return os.ReadFile(path)
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
