package community

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// A root nothing names is inert, and absence is the one signal review misses.
// See sirens-echo#1006.

// unloadedByDesign are roots no definition here names, each with its reason. A
// root reached only from deploy's definition counts, being invisible to this.
var unloadedByDesign = map[string]string{
	"sirens-dowel": "the owl.glass lane's root, named by deploy's sirens-dowel definition rather than by anything here",
	"ops-social-discord": "guidance for guarded read-only investigations, deliberately not a runtime skill root. " +
		"See AGENTS.md",
	"repo-sirens-echo": "the generated repository-pointer skill, which is discovery for an agent reading the " +
		"catalog rather than a runtime root",
	"moxn": "public contribution surface for Moxn's own people, inert until after the 2026-08-19 stream. " +
		"The lane reads references/moxn-knowledge-base.md meanwhile. See sirens-echo#999",
	"temporal": "public contribution surface for Temporal's own people, inert on the same schedule. " +
		"The lane reads references/temporal.md meanwhile. See sirens-echo#999",
}

// A root nobody loads is either a mistake or a decision, and this makes the
// difference visible rather than leaving it to be rediscovered.
func TestEverySkillRootIsLoadedOrNamedUnloaded(t *testing.T) {
	t.Parallel()
	roots := skillRootsOnDisk(t)
	named := skillRootsNamedByDefinitions(t)

	stray := make([]string, 0)
	for _, root := range roots {
		if named[root] {
			continue
		}
		if _, allowed := unloadedByDesign[root]; allowed {
			continue
		}
		// A parked game focus is inert on purpose, so it is a class rather than an
		// entry hand-edited per swap. gamefocus_test.go owns what one must carry.
		if strings.HasPrefix(root, gameFocusPrefix) {
			continue
		}
		stray = append(stray, root)
	}
	sort.Strings(stray)
	if len(stray) > 0 {
		t.Errorf(".agents/skills/%s is named by no definition's local_skill_roots, so nothing "+
			"loads it. Name it in a definition, or record it in unloadedByDesign with the reason",
			strings.Join(stray, ", .agents/skills/"))
	}
}

// The exemption list is hand-kept, so an entry left behind after its root was
// wired up would quietly excuse the next one to go missing.
func TestNoStaleUnloadedByDesignEntries(t *testing.T) {
	t.Parallel()
	onDisk := make(map[string]bool)
	for _, root := range skillRootsOnDisk(t) {
		onDisk[root] = true
	}
	named := skillRootsNamedByDefinitions(t)

	stale := make([]string, 0)
	for root := range unloadedByDesign {
		if !onDisk[root] {
			stale = append(stale, root+" (no such root)")
			continue
		}
		if named[root] {
			stale = append(stale, root+" (a definition names it now)")
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("unloadedByDesign carries %s. An entry that stopped being true excuses "+
			"the next root that goes missing", strings.Join(stale, ", "))
	}
}

func skillRootsOnDisk(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join("..", "..", ".agents", "skills"))
	if err != nil {
		t.Fatalf("read .agents/skills: %v", err)
	}
	roots := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			roots = append(roots, entry.Name())
		}
	}
	if len(roots) == 0 {
		t.Fatal("no skill roots found, so this test would pass on an empty tree")
	}
	return roots
}

func skillRootsNamedByDefinitions(t *testing.T) map[string]bool {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "agents", "*", "definition.yaml"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("glob definitions: %v, found %d", err, len(paths))
	}
	named := make(map[string]bool)
	for _, path := range paths {
		definition, err := LoadDefinition(path)
		if err != nil {
			t.Fatalf("LoadDefinition %s: %v", path, err)
		}
		for _, root := range definition.LocalSkillRoots {
			named[filepath.Base(root)] = true
		}
	}
	return named
}
