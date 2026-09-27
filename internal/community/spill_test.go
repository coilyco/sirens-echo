package community

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func spillSession(t *testing.T, requester string) ToolSession {
	t.Helper()
	provider := &ScratchProvider{Root: t.TempDir()}
	session, err := provider.Open(scratchContext(requester))
	if err != nil {
		t.Fatalf("open scratchpad: %v", err)
	}
	return session
}

// A trimmed result keeps its remainder where a scratchpad is mounted.
func TestSpillToolResultSavesTheRemainder(t *testing.T) {
	t.Parallel()
	session := spillSession(t, "member-1")
	full := strings.Repeat("t", maxToolResultBytes+1)

	path := spillToolResult(context.Background(), session, "get_trades", 0, full)
	if path != "tool-output/get_trades-1.txt" {
		t.Fatalf("spill path = %q", path)
	}

	read, err := session.Call(context.Background(), "scratch_read", map[string]any{"path": path})
	if err != nil {
		t.Fatalf("scratch_read: %v", err)
	}
	if !strings.Contains(read.Text, strings.Repeat("t", 64)) {
		t.Fatal("saved result does not carry the original content")
	}
}

// Two calls to one tool must not overwrite each other.
func TestSpillToolResultNumbersEachSave(t *testing.T) {
	t.Parallel()
	session := spillSession(t, "member-1")
	first := spillToolResult(context.Background(), session, "get_trades", 0, "alpha")
	second := spillToolResult(context.Background(), session, "get_trades", 1, "beta")
	if first == second {
		t.Fatalf("both saves took the same path %q", first)
	}
	if second != "tool-output/get_trades-2.txt" {
		t.Fatalf("second path = %q", second)
	}
}

// A deployment mounting no scratchpad keeps today's plain truncation.
func TestSpillToolResultWithoutScratchpadIsInert(t *testing.T) {
	t.Parallel()
	provider := &ScratchProvider{}
	session, err := provider.Open(context.Background())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if path := spillToolResult(context.Background(), session, "get_trades", 0, "alpha"); path != "" {
		t.Fatalf("saved without a scratchpad: %q", path)
	}
	if path := spillToolResult(context.Background(), nil, "get_trades", 0, "alpha"); path != "" {
		t.Fatalf("saved with no session: %q", path)
	}
}

// A result over the scratchpad file limit is refused, and a refusal must fall
// back rather than fail the turn.
func TestSpillToolResultOverFileLimitFallsBack(t *testing.T) {
	t.Parallel()
	session := spillSession(t, "member-1")
	oversized := strings.Repeat("t", maxScratchFileBytes+1)
	if path := spillToolResult(context.Background(), session, "get_trades", 0, oversized); path != "" {
		t.Fatalf("saved an oversized result: %q", path)
	}
}

// A server-supplied tool name must not reach the filesystem as a path.
func TestSpillPathFlattensToolNames(t *testing.T) {
	t.Parallel()
	for _, tool := range []string{"../../etc/passwd", "a/b", "..", ""} {
		got := spillPath(tool, 0)
		if strings.Contains(got, "..") || strings.Count(got, "/") != 1 {
			t.Fatalf("spillPath(%q) = %q", tool, got)
		}
	}
}

// A traversal attempt is refused by the scratchpad itself, not only by naming.
func TestSpillToolResultRefusesTraversalName(t *testing.T) {
	t.Parallel()
	session := spillSession(t, "member-1")
	path := spillToolResult(context.Background(), session, "../../escape", 0, "alpha")
	if strings.Contains(path, "..") {
		t.Fatalf("traversal survived: %q", path)
	}
}

// Provenance is a property rather than a convention: the model cannot write
// where the runtime writes, so a file there was not planted. See issue 273.
func TestTheModelCannotWriteIntoRuntimeOutput(t *testing.T) {
	t.Parallel()
	session := spillSession(t, "member-1")
	for _, attempt := range []string{
		"tool-output/get_trades-1.txt",
		"tool-output/planted.txt",
		"./tool-output/planted.txt",
		"TOOL-OUTPUT/planted.txt",
		"tool-output/nested/planted.txt",
		// Traversal is the spelling the guard's own comment would permit if
		// the check were moved ahead of path cleaning, as that comment claims.
		"a/../tool-output/planted.txt",
		"x/y/../../tool-output/planted.txt",
		`tool-output\planted.txt`,
		`.\tool-output\planted.txt`,
		"/tool-output/planted.txt",
		"  tool-output/planted.txt  ",
		"tool-output",
	} {
		attempt := attempt
		t.Run(attempt, func(t *testing.T) {
			t.Parallel()
			result, err := session.Call(context.Background(), "scratch_write", map[string]any{
				"path": attempt, "content": "planted",
			})
			if err != nil {
				t.Fatalf("scratch_write: %v", err)
			}
			if !result.IsError {
				t.Fatalf("the model wrote into runtime output at %q", attempt)
			}
		})
	}
}

// The model keeps its own scratchpad, so the reservation must not be a general
// write refusal.
func TestTheModelStillWritesItsOwnFiles(t *testing.T) {
	t.Parallel()
	session := spillSession(t, "member-1")
	for _, allowed := range []string{
		"notes.txt", "work/plan.md", "tool-outputs.txt",
		// Lookalikes. The reservation is the first segment, not the substring.
		"my-tool-output/plan.md", "tooloutput/plan.md", "a/tool-output/plan.md",
	} {
		allowed := allowed
		t.Run(allowed, func(t *testing.T) {
			t.Parallel()
			result, err := session.Call(context.Background(), "scratch_write", map[string]any{
				"path": allowed, "content": "mine",
			})
			if err != nil {
				t.Fatalf("scratch_write: %v", err)
			}
			if result.IsError {
				t.Fatalf("an ordinary write was refused at %q: %s", allowed, result.Text)
			}
		})
	}
}

// A saved result stays readable by the model, which is the whole point of
// saving it rather than truncating it away.
func TestRuntimeOutputStaysReadable(t *testing.T) {
	t.Parallel()
	session := spillSession(t, "member-1")
	path := spillToolResult(context.Background(), session, "get_trades", 0, "alpha beta")
	if path == "" {
		t.Fatal("nothing was saved")
	}
	read, err := session.Call(context.Background(), "scratch_read", map[string]any{"path": path})
	if err != nil || read.IsError {
		t.Fatalf("saved result is unreadable: %v %s", err, read.Text)
	}
}

// Reading a large result back spends the budget the trim protected, so search
// is the half that scales and the notice has to name both.
func TestTheSpillNoticeNamesBothWaysBackToTheResult(t *testing.T) {
	t.Parallel()
	notice := fmt.Sprintf(spillNotice, 262144, "tool-output/eco__get_stores-0")
	for _, tool := range []string{"scratch_read", "scratch_search"} {
		if !strings.Contains(notice, tool) {
			t.Errorf("the spill notice does not name %s: %q", tool, notice)
		}
	}
}

// A result a scratch file cannot hold is refused by the write and falls back to
// plain truncation, so the remainder is lost rather than saved.
func TestASpilledResultFitsAScratchFile(t *testing.T) {
	t.Parallel()
	if maxToolResultBytes >= maxScratchFileBytes {
		t.Errorf("a tool result is bounded at %d and a scratch file at %d, so spilling "+
			"one over the bound is refused and the remainder is dropped",
			maxToolResultBytes, maxScratchFileBytes)
	}
}

// A model that drops the directory from the notice's path still reads the
// spill, where it previously got "no such file" and looped (#8325, #8071).
func TestABareSpillNameStillReads(t *testing.T) {
	t.Parallel()
	session := spillSession(t, "member-1")
	if path := spillToolResult(context.Background(), session, "get_stores", 0, "store rows"); path == "" {
		t.Fatal("nothing was saved")
	}
	read, err := session.Call(context.Background(), "scratch_read", map[string]any{"path": "get_stores-1.txt"})
	if err != nil || read.IsError || !strings.Contains(read.Text, "store rows") {
		t.Fatalf("bare spill name = %v %q", err, read.Text)
	}
	missing, _ := session.Call(context.Background(), "scratch_read", map[string]any{"path": "absent.txt"})
	if !missing.IsError || missing.Text != "no such file: absent.txt" {
		t.Fatalf("a real miss = %q", missing.Text)
	}
}
