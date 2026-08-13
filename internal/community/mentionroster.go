package community

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Naming someone should reach them. Only people already in the conversation
// can be reached. See docs/sirens-echo-mentions.md.

// mentionLead is what may precede a name. A person is named in prose, so
// whitespace or an opening bracket. See docs/sirens-echo-mentions.md.
var mentionLead = `(?i)(^|[\s(\["'*_~])`

// mentionNameRunes is the shortest name worth resolving. A one or two
// character display name matches too much ordinary prose to be safe.
const mentionNameRunes = 3

// mentionRoster maps a display name to the account it belongs to, built from
// the turn's own transcript.
type mentionRoster map[string]string

// add records a name, ignoring one too short to match safely or one already
// known. First writer wins, so the oldest message in the turn decides.
func (r mentionRoster) add(name, userID string) {
	name = strings.TrimSpace(name)
	if len([]rune(name)) < mentionNameRunes || strings.TrimSpace(userID) == "" {
		return
	}
	if _, known := r[name]; !known {
		r[name] = userID
	}
}

// names returns the roster's names longest first, so a display name that
// contains another is matched before its substring.
func (r mentionRoster) names() []string {
	names := make([]string, 0, len(r))
	for name := range r {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if len(names[i]) != len(names[j]) {
			return len(names[i]) > len(names[j])
		}
		return names[i] < names[j]
	})
	return names
}

// codeSpanPattern matches a fenced block or an inline span. The longest
// delimiter is listed first, so a fence is not read as an empty inline span.
const codeSpanPattern = "```(?s:.*?)```" + "|``[^`]*``" + "|`[^`\n]*`"

// opaqueSpan is a run no name is read inside: a link, a code span, or Discord
// markup whose inner text is an id or an emoji name. See docs/sirens-echo-mentions.md.
var opaqueSpan = regexp.MustCompile(
	urlSpan.String() + `|<[@#:][^>]{0,64}>|<a?:[^>]{0,64}>|` + codeSpanPattern,
)

// mentionSpan is a run of the reply. An opaque run is carried through
// byte-identical, because its components are not people.
type mentionSpan struct {
	text string
	link bool
}

// mentionSpans splits a reply on the same urlSpan the prose validators mask
// with, so where a link starts has one definition. See sirens-echo#465.
func mentionSpans(reply string) []mentionSpan {
	spans := make([]mentionSpan, 0, 3)
	end := 0
	for _, bounds := range opaqueSpan.FindAllStringIndex(reply, -1) {
		if bounds[0] > end {
			spans = append(spans, mentionSpan{text: reply[end:bounds[0]]})
		}
		spans = append(spans, mentionSpan{text: reply[bounds[0]:bounds[1]], link: true})
		end = bounds[1]
	}
	if end < len(reply) {
		spans = append(spans, mentionSpan{text: reply[end:]})
	}
	return spans
}

// resolveMentions rewrites each named person as a Discord mention and reports
// the accounts it resolved. Nothing else in the reply is touched.
func (r mentionRoster) resolveMentions(reply string) (string, []string) {
	if len(r) == 0 || strings.TrimSpace(reply) == "" {
		return reply, nil
	}
	resolved := make([]string, 0, len(r))
	seen := make(map[string]bool, len(r))
	spans := mentionSpans(reply)
	for _, name := range r.names() {
		userID := r[name]
		if seen[userID] {
			continue
		}
		pattern, err := regexp.Compile(mentionLead + regexp.QuoteMeta(name) + `\b`)
		if err != nil {
			continue
		}
		// Once per person, and prose only. A reply naming someone four times
		// should reach them once, not ping them four times.
		if !resolveWithin(spans, pattern, userID) {
			continue
		}
		seen[userID] = true
		resolved = append(resolved, userID)
	}
	var rewritten strings.Builder
	for _, span := range spans {
		rewritten.WriteString(span.text)
	}
	return rewritten.String(), resolved
}

// resolveWithin replaces the first prose occurrence in place and reports
// whether it found one. A match only inside a link reaches nobody.
func resolveWithin(spans []mentionSpan, pattern *regexp.Regexp, userID string) bool {
	for index := range spans {
		if spans[index].link {
			continue
		}
		text := spans[index].text
		// Every occurrence, not the first. The first can be a hostname label
		// while a real mention follows it. See sirens-echo#481.
		for _, where := range pattern.FindAllStringSubmatchIndex(text, -1) {
			if inDottedIdentifier(text, where[3], where[1]) {
				continue
			}
			// Cut on the match rather than on the roster's spelling of the
			// name, which the case-insensitive pattern need not match.
			spans[index].text = text[:where[3]] + "<@" + userID + ">" + text[where[1]:]
			return true
		}
	}
	return false
}

// inDottedIdentifier reports whether a name sits inside a dotted identifier
// such as a hostname, where it is a label and not a person.
func inDottedIdentifier(text string, start, end int) bool {
	if start > 0 && text[start-1] == '.' {
		return true
	}
	// Scan the rest of the label first: the dot that proves a host can come
	// after a hyphenated remainder. See sirens-echo#515.
	rest := text[end:]
	for {
		next, width := utf8.DecodeRuneInString(rest)
		if width == 0 {
			return false
		}
		if next == '.' {
			// A dot before a letter or digit continues an identifier. A dot
			// before a space or the end is the punctuation of a sentence.
			after, size := utf8.DecodeRuneInString(rest[width:])
			return size > 0 && (unicode.IsLetter(after) || unicode.IsDigit(after))
		}
		if next != '-' && !unicode.IsLetter(next) && !unicode.IsDigit(next) {
			return false
		}
		rest = rest[width:]
	}
}
