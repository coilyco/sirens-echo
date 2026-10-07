package community

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A direct tool call fills its arguments from a vocabulary the server publishes,
// never from Jev: values are open-set (sirens-echo#8249).
const toolArgsMetaKey = "coilyco/args"

// toolArgSpec is one argument's declared vocabulary, which entry field to pass,
// and forms that are domain words for the tool rather than values ("store").
type toolArgSpec struct {
	Vocabulary string
	Field      string
	Ignore     map[string]bool
}

// VocabEntry is one vocabulary item: the forms a member may write, and the values.
type VocabEntry struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
}

type vocabCache struct {
	entries []VocabEntry
	fetched time.Time
}

// toolArgSpecs reads a tool's `_meta` argument vocabularies, skipping malformed ones.
func toolArgSpecs(tool *mcp.Tool) map[string]toolArgSpec {
	specs := map[string]toolArgSpec{}
	if tool == nil {
		return specs
	}
	raw, _ := tool.Meta[toolArgsMetaKey].(map[string]any)
	for name, value := range raw {
		fields, ok := value.(map[string]any)
		if !ok {
			continue
		}
		vocabulary, _ := fields["vocabulary"].(string)
		field, _ := fields["field"].(string)
		if vocabulary == "" || (field != "id" && field != "name") {
			continue
		}
		spec := toolArgSpec{Vocabulary: vocabulary, Field: field, Ignore: map[string]bool{}}
		if ignore, ok := fields["ignore"].([]any); ok {
			for _, form := range ignore {
				if text, ok := form.(string); ok {
					spec.Ignore[strings.Join(vocabWords(text), " ")] = true
				}
			}
		}
		specs[name] = spec
	}
	return specs
}

// Vocabulary reads one server's vocabulary resource, cached as long as its tool listing.
func (p *MCPProvider) Vocabulary(ctx context.Context, server, uri string) ([]VocabEntry, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry := p.entryLocked(server)
	if entry == nil || entry.session == nil {
		return nil, fmt.Errorf("MCP server %q is not connected", server)
	}
	if cached, ok := entry.vocab[uri]; ok && time.Since(cached.fetched) < p.refreshInterval() {
		return cached.entries, nil
	}
	readCtx, cancel := context.WithTimeout(ctx, mcpListTimeout)
	defer cancel()
	result, err := entry.session.ReadResource(readCtx, &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		return nil, fmt.Errorf("read vocabulary %s from %s: %w", uri, server, err)
	}
	var body struct {
		Entries []VocabEntry `json:"entries"`
	}
	if err := json.Unmarshal([]byte(resourceText(result)), &body); err != nil {
		return nil, fmt.Errorf("parse vocabulary %s from %s: %w", uri, server, err)
	}
	if entry.vocab == nil {
		entry.vocab = make(map[string]vocabCache)
	}
	entry.vocab[uri] = vocabCache{entries: body.Entries, fetched: time.Now()}
	return body.Entries, nil
}

// matchVocab finds the entry whose longest form appears in message as whole
// words, skipping ignored forms. A tie at that length is ambiguous and matches nothing.
func matchVocab(message string, entries []VocabEntry, ignore map[string]bool) (VocabEntry, bool) {
	entry, _, found := matchVocabWords(vocabWords(message), entries, ignore)
	return entry, found == vocabMatched
}

type vocabFound int

const (
	vocabNone vocabFound = iota
	vocabTied
	vocabMatched
)

// matchVocabWords is matchVocab over split words, also returning the matched
// form, and telling a tie apart from no match at all.
func matchVocabWords(words []string, entries []VocabEntry, ignore map[string]bool) (VocabEntry, []string, vocabFound) {
	var best VocabEntry
	var bestForm []string
	tied := false
	for _, entry := range entries {
		var longest []string
		for _, form := range append([]string{entry.Name, entry.ID}, entry.Aliases...) {
			formWords := vocabWords(form)
			if ignore[strings.Join(formWords, " ")] {
				continue
			}
			if len(formWords) > len(longest) && containsWords(words, formWords) {
				longest = formWords
			}
		}
		switch {
		case len(longest) == 0:
		case len(longest) > len(bestForm):
			best, bestForm, tied = entry, longest, false
		case len(longest) == len(bestForm) && entry.ID != best.ID:
			tied = true
		}
	}
	switch {
	case len(bestForm) == 0:
		return VocabEntry{}, nil, vocabNone
	case tied:
		return VocabEntry{}, nil, vocabTied
	}
	return best, bestForm, vocabMatched
}

// withoutForm drops form's first whole-word occurrence, so the words one
// argument used cannot fill the next one ("iron at au3", eco-app#8425).
func withoutForm(words, form []string) []string {
	for start := 0; start+len(form) <= len(words); start++ {
		if containsWords(words[start:start+len(form)], form) {
			return append(append([]string{}, words[:start]...), words[start+len(form):]...)
		}
	}
	return words
}

func vocabWords(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// containsWords reports whether form occurs contiguously in words, a message word
// with a trailing s or es counting as the same word.
func containsWords(words, form []string) bool {
	return indexWords(words, form) >= 0
}

// indexWords is where form first occurs in words, or -1.
func indexWords(words, form []string) int {
	for start := 0; start+len(form) <= len(words); start++ {
		matched := true
		for i, want := range form {
			if !sameWord(words[start+i], want) {
				matched = false
				break
			}
		}
		if matched {
			return start
		}
	}
	return -1
}

func sameWord(got, want string) bool {
	return got == want || got == want+"s" || got == want+"es"
}

// vocabHit is one item a message names: the entry, the form that matched, and
// where in the message's words that form starts.
type vocabHit struct {
	entry VocabEntry
	form  []string
	at    int
}

// matchVocabAll finds every item a message names, each word spent once, in
// message order. Two items claiming the same words is one ambiguous item: false.
func matchVocabAll(words []string, entries []VocabEntry, ignore map[string]bool) ([]vocabHit, bool) {
	spent := append([]string{}, words...)
	var hits []vocabHit
	for {
		round, ok := longestVocabHits(spent, entries, ignore)
		if !ok {
			return nil, false
		}
		if len(round) == 0 {
			break
		}
		for _, hit := range round {
			for i := range hit.form {
				// An empty word equals no form word, so a spent word cannot match twice.
				spent[hit.at+i] = ""
			}
		}
		hits = append(hits, round...)
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].at < hits[j].at })
	return hits, true
}

// longestVocabHits returns every entry whose longest form is the longest form
// present, or false when two of them overlap.
func longestVocabHits(words []string, entries []VocabEntry, ignore map[string]bool) ([]vocabHit, bool) {
	var round []vocabHit
	longest := 0
	for _, entry := range entries {
		var best vocabHit
		for _, form := range append([]string{entry.Name, entry.ID}, entry.Aliases...) {
			formWords := vocabWords(form)
			if ignore[strings.Join(formWords, " ")] || len(formWords) <= len(best.form) {
				continue
			}
			if at := indexWords(words, formWords); at >= 0 {
				best = vocabHit{entry: entry, form: formWords, at: at}
			}
		}
		switch {
		case len(best.form) == 0 || len(best.form) < longest:
		case len(best.form) > longest:
			longest, round = len(best.form), []vocabHit{best}
		default:
			round = append(round, best)
		}
	}
	for i, a := range round {
		for _, b := range round[i+1:] {
			if a.at < b.at+len(b.form) && b.at < a.at+len(a.form) {
				return nil, false
			}
		}
	}
	return round, true
}

// resolveToolArgs fills every name in whenArgs from its vocabulary, or fails. A
// tied argument yields one set per item (COI-2112), at most one such argument.
func (a *Agent) resolveToolArgs(
	ctx context.Context,
	server string,
	specs map[string]toolArgSpec,
	whenArgs []string,
	message string,
) ([]map[string]any, bool) {
	args := map[string]any{}
	words := vocabWords(message)
	several, values := "", []any(nil)
	for _, name := range whenArgs {
		spec, ok := specs[name]
		if !ok {
			return nil, false
		}
		entries, err := a.tools.Vocabulary(ctx, server, spec.Vocabulary)
		if err != nil {
			return nil, false
		}
		match, form, found := matchVocabWords(words, entries, spec.Ignore)
		if found == vocabTied && several == "" {
			hits, ok := matchVocabAll(words, entries, spec.Ignore)
			if !ok || len(hits) < 2 || len(hits) > maxDirectItems {
				return nil, false
			}
			several = name
			for _, hit := range hits {
				value := vocabValue(hit.entry, spec)
				if value == "" {
					return nil, false
				}
				values = append(values, value)
				words = withoutForm(words, hit.form)
			}
			continue
		}
		if found != vocabMatched {
			return nil, false
		}
		words = withoutForm(words, form)
		value := vocabValue(match, spec)
		if value == "" {
			return nil, false
		}
		args[name] = value
	}
	if several == "" {
		return []map[string]any{args}, true
	}
	sets := make([]map[string]any, 0, len(values))
	for _, value := range values {
		set := map[string]any{several: value}
		for name, fixed := range args {
			set[name] = fixed
		}
		sets = append(sets, set)
	}
	return sets, true
}

func vocabValue(entry VocabEntry, spec toolArgSpec) string {
	value := entry.Name
	if spec.Field == "id" {
		value = entry.ID
	}
	return strings.TrimSpace(value)
}

// argsUnmatched: every named vocabulary was read and matched nothing. An unread
// vocabulary is not a miss, so any error keeps the turn on its model path.
func (a *Agent) argsUnmatched(
	ctx context.Context,
	server string,
	specs map[string]toolArgSpec,
	names []string,
	message string,
) bool {
	for _, name := range names {
		spec, ok := specs[name]
		if !ok {
			return false
		}
		entries, err := a.tools.Vocabulary(ctx, server, spec.Vocabulary)
		if err != nil || len(entries) == 0 {
			return false
		}
		// A tie is two items named, not none, so it is not a miss either.
		if _, _, found := matchVocabWords(vocabWords(message), entries, spec.Ignore); found != vocabNone {
			return false
		}
	}
	return len(names) > 0
}
