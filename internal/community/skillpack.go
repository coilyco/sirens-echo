package community

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxSkillpackBytes = 256 * 1024

// LoadSkillpack loads SKILL.md and one-level reference Markdown from each
// configured repo-local skill root in deterministic order.
func LoadSkillpack(roots []string) (string, error) {
	files := make([]string, 0)
	for _, root := range roots {
		if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			slashed := filepath.ToSlash(relative)
			if slashed == "SKILL.md" ||
				(strings.HasPrefix(slashed, "references/") && strings.HasSuffix(slashed, ".md")) {
				files = append(files, path)
			}
			return nil
		}); err != nil {
			return "", fmt.Errorf("walk skill root %s: %w", root, err)
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return "", fmt.Errorf("skillpack contains no SKILL.md or reference Markdown")
	}

	var output strings.Builder
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read skill file %s: %w", path, err)
		}
		body := stripFrontmatter(string(raw))
		if body == "" {
			return "", fmt.Errorf("skill file %s has no body", path)
		}
		fmt.Fprintf(&output, "\n## Source: %s\n\n%s\n", filepath.ToSlash(path), body)
		if output.Len() > maxSkillpackBytes {
			return "", fmt.Errorf("skillpack exceeds %d bytes", maxSkillpackBytes)
		}
	}
	return strings.TrimSpace(output.String()), nil
}

func stripFrontmatter(raw string) string {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return strings.TrimSpace(normalized)
	}
	remainder := normalized[4:]
	end := strings.Index(remainder, "\n---\n")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(remainder[end+5:])
}
