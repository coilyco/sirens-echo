package community

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// threads.md says the job binding is unwired and commands.md describes it as
// shipped. Nothing kept either in step with the code. See sirens-echo#620.

// threadBindingWriters are the only functions that set Origin.ThreadID. A
// production caller for any of them means the binding is wired.
var threadBindingWriters = []string{"BindJobToThread"}

// productionCallers counts references from outside the named function, in
// non-test sources. A doc comment is not a caller and neither is its own body.
func productionCallers(t *testing.T, name string) int {
	t.Helper()
	declaration := regexp.MustCompile(`^func (?:\([^)]*\) )?` + name + `\(`)
	reference := regexp.MustCompile(`\b` + name + `\b`)
	callers := 0
	for _, body := range nonTestGoSources(t) {
		inOwnBody := false
		for _, line := range strings.Split(body, "\n") {
			trimmed := strings.TrimSpace(line)
			// A recursive call and a wrapper calling its only sibling both read
			// as usage from a bare count. See sirens-echo#618.
			if inOwnBody {
				inOwnBody = line != "}"
				continue
			}
			if declaration.MatchString(trimmed) {
				inOwnBody = true
				continue
			}
			if trimmed == "" || strings.HasPrefix(trimmed, "//") {
				continue
			}
			if reference.MatchString(trimmed) {
				callers++
			}
		}
	}
	return callers
}

// The doc's own claim, so the honest half cannot go stale silently. When the
// binding is wired this fails and names the second doc that also needs it.
func TestTheUnwiredBindingIsDocumentedAsUnwired(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile(filepath.Join("..", "..", "docs", "sirens-echo-threads.md"))
	if err != nil {
		t.Fatalf("read the threads doc: %v", err)
	}
	documentsUnwired := strings.Contains(string(body), "still\nunwired") ||
		strings.Contains(string(body), "still unwired")

	for _, writer := range threadBindingWriters {
		wired := productionCallers(t, writer) > 0
		if wired && documentsUnwired {
			t.Errorf("%s now has a production caller and docs/sirens-echo-threads.md "+
				"still says it is unwired. Update that doc, and update the Thread "+
				"binding section of docs/sirens-echo-commands.md, which already "+
				"describes this as shipped. See sirens-echo#620", writer)
		}
		if !wired && !documentsUnwired {
			t.Errorf("%s has no production caller and docs/sirens-echo-threads.md "+
				"no longer says so, so the only honest description of this feature "+
				"is gone. See sirens-echo#620", writer)
		}
	}
}
