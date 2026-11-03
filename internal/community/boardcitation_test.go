package community

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// prompt-check guards the rendered snapshot against its sources. Nothing
// guarded the board's line citations against the snapshot. See sirens-echo#310.

// citationLine reads one entry: clause = first-last = text inside that range.
var citationLine = regexp.MustCompile(`^#\s+([a-z][a-z-]*) = (\d+)-(\d+) = (.+)$`)

type clauseCitation struct {
	clause string
	first  int
	last   int
	anchor string
}

// A citation nobody follows rots silently, and a grader following a stale one
// scores against a clause the prompt does not state there.
func TestBoardClauseCitationsStillPointAtTheirClause(t *testing.T) {
	t.Parallel()
	citations := boardCitations(t)
	if len(citations) == 0 {
		t.Fatal("board-citations.txt declares no clause citations, so this asserts nothing")
	}
	lines := renderedDeepPrompt(t)
	for _, citation := range citations {
		if citation.first < 1 || citation.last > len(lines) || citation.first > citation.last {
			t.Errorf("clause %s cites lines %d-%d, outside a prompt of %d lines",
				citation.clause, citation.first, citation.last, len(lines))
			continue
		}
		cited := strings.Join(lines[citation.first-1:citation.last], " ")
		if strings.Contains(cited, citation.anchor) {
			continue
		}
		t.Errorf("clause %s cites lines %d-%d, which no longer contain %q. %s",
			citation.clause, citation.first, citation.last, citation.anchor,
			whereItMovedTo(lines, citation.anchor))
	}
}

// Every clause a case declares needs a citation, or the board grades against a
// clause nobody located in the prompt the subject actually read.
func TestEveryBoardClauseIsCited(t *testing.T) {
	t.Parallel()
	cited := make(map[string]struct{})
	for _, citation := range boardCitations(t) {
		cited[citation.clause] = struct{}{}
	}
	pack, err := LoadBoardPack(filepath.Join("..", "..", "agents", "deep", "packs", "board.yaml"))
	if err != nil {
		t.Fatalf("load the board pack: %v", err)
	}
	missing := make([]string, 0)
	for _, boardChallenge := range pack.Challenges {
		if _, ok := cited[boardChallenge.Clause]; !ok {
			missing = append(missing, boardChallenge.Clause)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("clauses with no citation: %s. Add one to board-citations.txt naming "+
			"the lines of agents/deep/rendered/prompt.txt that state the clause",
			strings.Join(unique(missing), ", "))
	}
}

// whereItMovedTo turns a failure into the fix, since the answer is a line
// number the reader would otherwise go and find by hand.
func whereItMovedTo(lines []string, anchor string) string {
	for index, line := range lines {
		if strings.Contains(line, anchor) {
			return fmt.Sprintf("It is at line %d now, so update the citation", index+1)
		}
	}
	return "It is nowhere in the prompt now, so the clause or the anchor changed"
}

func boardCitations(t *testing.T) []clauseCitation {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "agents", "deep", "packs", "board-citations.txt"))
	if err != nil {
		t.Fatalf("read the board citations: %v", err)
	}
	citations := make([]clauseCitation, 0)
	for _, line := range strings.Split(string(body), "\n") {
		match := citationLine.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		first, firstErr := strconv.Atoi(match[2])
		last, lastErr := strconv.Atoi(match[3])
		if firstErr != nil || lastErr != nil {
			t.Fatalf("citation %q carries an unreadable line range", line)
		}
		citations = append(citations, clauseCitation{
			clause: match[1], first: first, last: last,
			anchor: strings.TrimSpace(match[4]),
		})
	}
	return citations
}

func renderedDeepPrompt(t *testing.T) []string {
	t.Helper()
	body, err := os.ReadFile(
		filepath.Join("..", "..", "agents", "deep", "rendered", "prompt.txt"))
	if err != nil {
		t.Fatalf("read the rendered prompt: %v", err)
	}
	return strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
}

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
