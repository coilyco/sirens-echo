package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coilyco/sirens-echo/internal/community"
)

func write(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestACommitOfALiveRunIsRefusedWhereverTheMarkerSits(t *testing.T) {
	opening := write(t, "a.yaml", community.LiveRunMarker+" roster=r.yaml\nreplies: []\n")
	pasted := write(t, "b.yaml", "replies: []\n"+community.LiveRunMarker+"\n")
	var stderr bytes.Buffer

	if code := run([]string{opening, pasted}, &stderr); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	for _, path := range []string{opening, pasted} {
		if !strings.Contains(stderr.String(), path) {
			t.Errorf("%s is not named in %q", path, stderr.String())
		}
	}
}

func TestAFixtureRecordPassesTheCheck(t *testing.T) {
	record := write(t, "fixture.yaml", "# fixture: invented names only\nreplies: []\n")
	var stderr bytes.Buffer

	if code := run([]string{record}, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("code = %d, stderr = %q, want a clean pass", code, stderr.String())
	}
}

func TestAnUnreadableFileAndNoArgumentsAreFailuresNotPasses(t *testing.T) {
	var stderr bytes.Buffer
	if code := run([]string{filepath.Join(t.TempDir(), "gone.yaml")}, &stderr); code != 1 {
		t.Errorf("unreadable: code = %d, want 1", code)
	}
	if code := run(nil, &stderr); code != 2 {
		t.Errorf("no arguments: code = %d, want 2", code)
	}
}
