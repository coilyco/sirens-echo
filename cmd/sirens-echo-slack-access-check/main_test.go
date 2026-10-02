package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const bounded = `schema: coilyco-harness.slack-access.v1
workspaces:
  - id: T0COILYCO
    channels: ["C0GENERAL"]
    users: ["U0KAI0001"]
`

func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "slack-access.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestABoundedPolicyPassesAndSaysWhatItAdmits(t *testing.T) {
	t.Parallel()
	var out, errs bytes.Buffer
	if code := run([]string{write(t, bounded)}, strings.NewReader(""), &out, &errs); code != 0 {
		t.Fatalf("code = %d, stderr = %s", code, errs.String())
	}
	if !strings.Contains(out.String(), "workspace T0COILYCO") || !strings.Contains(out.String(), "ok") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestAPolicyTheRuntimeWouldRefuseExitsOne(t *testing.T) {
	t.Parallel()
	var out, errs bytes.Buffer
	open := strings.Replace(bounded, `users: ["U0KAI0001"]`, "users: all", 1)
	if code := run([]string{write(t, open)}, strings.NewReader(""), &out, &errs); code != 1 {
		t.Fatalf("code = %d, want 1 for an unbounded workspace", code)
	}
	if !strings.Contains(errs.String(), "per_user") {
		t.Fatalf("the reason must name the missing bound, got %q", errs.String())
	}
}

func TestNoArgumentIsAUsageErrorAndStdinIsAPolicy(t *testing.T) {
	t.Parallel()
	var out, errs bytes.Buffer
	if code := run(nil, strings.NewReader(""), &out, &errs); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	out.Reset()
	errs.Reset()
	if code := run([]string{"-"}, strings.NewReader(bounded), &out, &errs); code != 0 {
		t.Fatalf("stdin policy: code = %d, stderr = %s", code, errs.String())
	}
}
