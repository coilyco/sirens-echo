package community

import (
	"context"
	"encoding/json"
	"fmt"
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
	for start := 0; start+len(form) <= len(words); start++ {
		matched := true
		for i, want := range form {
			if !sameWord(words[start+i], want) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func sameWord(got, want string) bool {
	return got == want || got == want+"s" || got == want+"es"
}

// resolveToolArgs fills every name in whenArgs from its vocabulary, or fails.
func (a *Agent) resolveToolArgs(
	ctx context.Context,
	server string,
	specs map[string]toolArgSpec,
	whenArgs []string,
	message string,
) (map[string]any, bool) {
	args := map[string]any{}
	words := vocabWords(message)
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
		if found != vocabMatched {
			return nil, false
		}
		words = withoutForm(words, form)
		value := match.Name
		if spec.Field == "id" {
			value = match.ID
		}
		if strings.TrimSpace(value) == "" {
			return nil, false
		}
		args[name] = value
	}
	return args, true
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
