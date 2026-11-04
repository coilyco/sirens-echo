package community

import (
	"context"
	"log/slog"
	"slices"

	"github.com/coilyco/sirens-echo/internal/community/systemone"
)

const (
	// namesInMessageKey is the Jev state key holding the names a server's own
	// vocabularies found in the message: {server: {argument: [names]}}.
	namesInMessageKey = "names_in_message"
	evidenceSentence  = " names_in_message lists names the server's own vocabularies found in the message."
)

// toolEvidence finds the vocabulary names a message contains, per server and
// argument, for arguments published with `"evidence": true` (COI-3279).
func (a *Agent) toolEvidence(ctx context.Context, message string) map[string]map[string][]string {
	if a.tools == nil {
		return nil
	}
	words := vocabWords(message)
	found := map[string]map[string][]string{}
	for _, listing := range a.tools.CachedTools() {
		for _, tool := range listing.Tools {
			for arg, spec := range toolArgSpecs(tool) {
				if !spec.Evidence {
					continue
				}
				entries, err := a.tools.Vocabulary(ctx, listing.Server, spec.Vocabulary)
				if err != nil {
					a.telemetry.Info(ctx, "tool.evidence.vocab_failed",
						slog.String("server", listing.Server), slog.String("error", err.Error()))
					continue
				}
				entry, _, status := matchVocabWords(words, entries, spec.Ignore)
				if status != vocabMatched {
					continue
				}
				name := entry.Name
				if name == "" {
					name = entry.ID
				}
				if found[listing.Server] == nil {
					found[listing.Server] = map[string][]string{}
				}
				if !slices.Contains(found[listing.Server][arg], name) {
					found[listing.Server][arg] = append(found[listing.Server][arg], name)
				}
			}
		}
	}
	return found
}

// noteEvidence tells each server's tool pick that names_in_message exists, only
// when that server has a hit, so every other pick reads exactly as before.
func noteEvidence(questions []systemone.Question, meta map[string]jevQuestionMeta, evidence map[string]map[string][]string) {
	for i, q := range questions {
		m := meta[q.Key]
		if m.family == RouteFamilyTool && len(evidence[m.target]) > 0 {
			questions[i].Prompt += evidenceSentence
		}
	}
}
