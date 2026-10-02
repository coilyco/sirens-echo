// Command sirens-echo-evidence counts a behaviour across committed run
// records. See docs/sirens-echo-grounding.md.
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/coilyco/sirens-echo/internal/community"
)

// evidenceGlob is every committed dataset. Runs we already pay for answer how
// often, without a bespoke case or a larger N.
const evidenceGlob = "agents/*/evaluations/*.yaml"

// replyStrings walks a decoded record for text a model produced. Walking rather
// than modelling the schema, since three runners emit three shapes.
func replyStrings(node any, into *[]string) {
	switch value := node.(type) {
	case map[string]any:
		for key, child := range value {
			if text, ok := child.(string); ok && (key == "text" || key == "reply") {
				*into = append(*into, text)
				continue
			}
			replyStrings(child, into)
		}
	case []any:
		for _, child := range value {
			replyStrings(child, into)
		}
	}
}

type finding struct {
	dataset string
	replies int
	marked  int
	// structured is false for a free-text transcript, which has occurrences and
	// no reply denominator. A share over those two summed would be invented.
	structured bool
}

// datasetStart finds the record inside a file that also carries the run's log
// stream. See docs/sirens-echo-grounding.md.
func datasetStart(body []byte) int {
	marker := []byte("\nschema:")
	if at := bytes.Index(body, marker); at >= 0 {
		return at + 1
	}
	return 0
}

func scan(path string) (finding, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return finding{}, err
	}
	var decoded any
	if err := yaml.Unmarshal(body[datasetStart(body):], &decoded); err != nil {
		return finding{}, fmt.Errorf("%s: %w", path, err)
	}
	replies := make([]string, 0)
	replyStrings(decoded, &replies)

	result := finding{
		dataset:    filepath.Base(path),
		replies:    len(replies),
		structured: len(replies) > 0,
	}
	if !result.structured {
		// A free-text transcript carries no record boundaries, so it can report
		// that the behaviour is present and never how often per attempt.
		if community.ValidateNoToolCallMarkup(string(body)) != nil {
			result.marked = 1
		}
		return result, nil
	}
	for _, reply := range replies {
		if community.ValidateNoToolCallMarkup(reply) != nil {
			result.marked++
		}
	}
	return result, nil
}

func main() {
	paths, err := filepath.Glob(evidenceGlob)
	if err != nil || len(paths) == 0 {
		fmt.Fprintf(os.Stderr, "no evidence at %s: %v\n", evidenceGlob, err)
		os.Exit(1)
	}
	sort.Strings(paths)
	totalReplies, totalMarked := 0, 0
	for _, path := range paths {
		result, err := scan(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		if !result.structured {
			state := "clean"
			if result.marked > 0 {
				state = "markup present"
			}
			fmt.Printf("%-32s free text, no reply count, %s\n", result.dataset, state)
			continue
		}
		fmt.Printf("%-32s %4d replies %4d with tool-call markup\n",
			result.dataset, result.replies, result.marked)
		totalReplies += result.replies
		totalMarked += result.marked
	}
	// A denominator only exists for structured records, so the share is over
	// those. Summing the two kinds would invent a rate.
	if totalReplies == 0 {
		fmt.Fprintln(os.Stderr, "no structured dataset was parsed, so nothing was counted")
		os.Exit(1)
	}
	share := 0.0
	if totalReplies > 0 {
		share = 100 * float64(totalMarked) / float64(totalReplies)
	}
	fmt.Printf("%-32s %4d replies %4d with tool-call markup, %.2f%%\n",
		"TOTAL", totalReplies, totalMarked, share)
	// Zero is not proof of absence. The pattern set covers one measured model
	// family. See docs/sirens-echo-tool-markup.md.
	fmt.Println("Counts cover the measured delimiter syntaxes only.")
}
