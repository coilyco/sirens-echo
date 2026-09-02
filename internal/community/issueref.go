package community

import (
	"regexp"
	"strings"
)

// An issue reference outlives the conversation that wrote it, so the harness
// links what the model named in short form. See docs/sirens-echo-issues.md.

// issueKeyPattern matches the tracker's key, org/repo#number. It is the
// canonical form: the tracker is tailnet-only and no URL is invented.
var issueKeyPattern = regexp.MustCompile(`[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+#\d+`)

// shortIssueRef matches the ambiguous form. The leading boundary rejects a
// channel mention, a URL fragment, and a number inside a longer reference.
var shortIssueRef = regexp.MustCompile(`(?:^|[^A-Za-z0-9_/#-])#(\d+)\b`)

// createIssueSuffix names the tool whose result reaches the member whether or
// not the model mentioned filing anything.
const createIssueSuffix = "__" + trackerFileTool

// referenceHeading labels the appended block. It carries no first person and no
// emoji, because it is appended after the response-style check has run.
const referenceHeading = "Referenced issues:"

// AppendIssueReferences links the issues a turn observed or filed. Every key it
// appends came back from a tool call, so it states nothing unobserved.
func AppendIssueReferences(reply string, executed ...ExecutedTool) string {
	return appendIssueReferencesWithin(reply, discordReplyLimit, executed)
}

// appendIssueReferencesWithin renders the block inside a caller's ceiling, and
// a limit of zero is unbounded. See docs/sirens-echo-issues.md.
func appendIssueReferencesWithin(
	reply string, limit int, executed []ExecutedTool,
) string {
	observed := observedIssueKeys(executed)
	if len(observed) == 0 {
		return reply
	}
	appended := make([]string, 0, len(observed))
	seen := make(map[string]struct{}, len(observed))
	add := func(key string) {
		if key == "" || strings.Contains(reply, key) {
			return
		}
		if _, done := seen[key]; done {
			return
		}
		seen[key] = struct{}{}
		appended = append(appended, key)
	}
	// A short form is the reported defect, so it resolves first and in the
	// order a reader meets it.
	for _, match := range shortIssueRef.FindAllStringSubmatch(reply, -1) {
		add(observed[match[1]])
	}
	// An issue this turn filed belongs in the reply even when the model said
	// nothing about it.
	for _, key := range createdIssueKeys(executed) {
		add(key)
	}
	return withReferenceBlock(reply, appended, limit)
}

// withReferenceBlock renders the block within the send budget. A block that
// cannot fit whole is dropped rather than truncated into a broken key.
func withReferenceBlock(reply string, keys []string, limit int) string {
	if len(keys) == 0 {
		return reply
	}
	trimmed := strings.TrimRight(reply, " \t\n")
	for len(keys) > 0 {
		block := "\n\n" + referenceHeading + "\n" + strings.Join(keys, "\n")
		if limit <= 0 || len([]rune(trimmed))+len([]rune(block)) <= limit {
			return trimmed + block
		}
		keys = keys[:len(keys)-1]
	}
	return reply
}

// observedIssueKeys maps an issue number to the key a tool returned for it.
// Scanning every result is deliberate: a quoted sibling issue is a real source.
func observedIssueKeys(executed []ExecutedTool) map[string]string {
	observed := make(map[string]string)
	for _, tool := range executed {
		for _, ref := range issueRefsIn(tool.Result) {
			previous, seen := observed[ref.number]
			if !seen {
				observed[ref.number] = ref.key
				continue
			}
			// One number, two repositories. Naming either is a guess, and the
			// empty value suppresses it. See docs/sirens-echo-issues.md.
			if previous != ref.key {
				observed[ref.number] = ""
			}
		}
	}
	return observed
}

// createdIssueKeys names what this turn filed. The filing tool reports the key
// it read back, so an unconfirmed write contributes nothing here.
func createdIssueKeys(executed []ExecutedTool) []string {
	keys := make([]string, 0)
	for _, tool := range executed {
		if !strings.HasSuffix(tool.Name, createIssueSuffix) {
			continue
		}
		// The first key in a filing result is the issue just filed, so a key
		// quoted inside the body is not mistaken for it.
		for _, ref := range issueRefsIn(tool.Result) {
			keys = append(keys, ref.key)
			break
		}
	}
	return keys
}

type issueRef struct {
	number string
	key    string
}

func issueRefsIn(result string) []issueRef {
	matches := issueKeyPattern.FindAllString(result, -1)
	refs := make([]issueRef, 0, len(matches))
	for _, match := range matches {
		_, number, found := strings.Cut(match, "#")
		if !found {
			continue
		}
		refs = append(refs, issueRef{number: number, key: match})
	}
	return refs
}
