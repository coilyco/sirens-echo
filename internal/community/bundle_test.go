package community

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAOSBundleLoadsCompiledRoleContext(t *testing.T) {
	t.Parallel()
	root := writeBundleFixture(t, "community", "delivery/community.md")
	bundle, err := LoadAOSBundle(root, "community")
	if err != nil {
		t.Fatalf("LoadAOSBundle: %v", err)
	}
	if !strings.Contains(bundle.Context, "selected community skill") {
		t.Fatalf("compiled context = %q", bundle.Context)
	}
	if len(bundle.Manifest.Personalities) == 0 {
		t.Fatal("bundle personality meld was not loaded")
	}
}

func TestLoadAOSBundleRejectsRoleMismatch(t *testing.T) {
	t.Parallel()
	root := writeBundleFixture(t, "community", "delivery/community.md")
	if _, err := LoadAOSBundle(root, "engineer"); err == nil {
		t.Fatal("role-mismatched bundle passed")
	}
}

func writeBundleFixture(t *testing.T, role, compiledPath string) string {
	t.Helper()
	root := t.TempDir()
	contextPath := filepath.Join(root, filepath.FromSlash(compiledPath))
	if err := os.MkdirAll(filepath.Dir(contextPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		contextPath,
		[]byte("# Role instructions\n\nselected community skill\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	manifest := BundleManifest{
		Format:        "agent-compose.bundle",
		Role:          role,
		ModelClass:    "frontier",
		Personalities: []string{"kind"},
		Sources:       []string{"person:test", "aos-public"},
		Delivery: BundleDelivery{
			Mode:            "compiled",
			Instructions:    "content/instructions.md",
			CompiledContext: compiledPath,
		},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}
