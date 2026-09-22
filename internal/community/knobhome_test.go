package community

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The consolidation had nothing holding it, and three constants drifted back
// out before anyone noticed. See sirens-echo#829.

// Detection is by shape rather than by name, and the two failures that bought
// that are in docs/sirens-echo-tuning.md.

// elsewhereByDesign are numbers that read as knobs and are not. Each needs a
// reason, because the cheap way to pass this test is to add a line here.
var elsewhereByDesign = map[string]string{
	"minNormalizedIDDigits":  "an algorithm's collision floor, not a knob: lowering it changes what counts as an identifier",
	"minEncodedGuardBytes":   "the same floor for the base64 reading: it decides what is a match, not how much of one to allow",
	"opaqueSecretRunes":      "the same floor again, for the opaque reading: it decides what is a credential rather than a word",
	"unboundedReply":         "a sentinel for a transport that declares no ceiling, so it is the absence of a bound rather than one",
	"scratchPermissions":     "a file mode. Text-only is enforced by denying the execute bit, so a deployment must not be able to grant it",
	"scratchFilePermissions": "a file mode, for the same reason: the partition is readable only by this process",
	"workspacePermissions":   "a file mode, for the same reason as the scratchpad's",
	"jevContentThreshold":    "the spec's proposed content-match cutoff: part of route.jev's algorithm (which classes count as matched), not a deployment timeout or count",
	"jevDrawerThreshold":     "the spec's proposed drawer-match cutoff, same reason as the content threshold",
	"pollMinAnswers":         "Discord's own floor for what a poll is: fewer than two options is not a poll, so this decides the schema's shape rather than a deployment's tuning",
	"pollMinDuration":        "Discord's own floor: a poll cannot run for less than one hour, which is a structural fact rather than a deployment's tuning",
	"jevShapeCutoff":         "the spec's proposed shape-pick cutoff, same reason: it decides what counts as a confident pick, not how long to wait or how many to allow",
}

// numericValue reports whether an expression is a literal number or literals
// combined into one, so `10 * time.Second` counts and a call does not.
func numericValue(expr ast.Expr) bool {
	switch value := expr.(type) {
	case *ast.BasicLit:
		return value.Kind == token.INT || value.Kind == token.FLOAT
	case *ast.BinaryExpr:
		return numericValue(value.X) || numericValue(value.Y)
	case *ast.UnaryExpr:
		return numericValue(value.X)
	case *ast.ParenExpr:
		return numericValue(value.X)
	}
	return false
}

// packageNumbers returns every package-level const or var declared with a
// numeric value. A number inside a function is a local and never a knob.
func packageNumbers(t *testing.T, path string) map[string]token.Position {
	t.Helper()
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	found := make(map[string]token.Position)
	for _, decl := range parsed.Decls {
		general, ok := decl.(*ast.GenDecl)
		if !ok || (general.Tok != token.CONST && general.Tok != token.VAR) {
			continue
		}
		for _, spec := range general.Specs {
			values, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for index, name := range values.Names {
				if index >= len(values.Values) || !numericValue(values.Values[index]) {
					continue
				}
				found[name.Name] = fileSet.Position(name.Pos())
			}
		}
	}
	return found
}

// A knob outside config.go is invisible to anyone asking what this service can
// be tuned to do, which is the whole reason one file holds them.
func TestEveryTuningNumberLivesInConfigGo(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	strays := make([]string, 0)
	claimed := make(map[string]bool, len(elsewhereByDesign))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") || name == "config.go" {
			continue
		}
		for number, at := range packageNumbers(t, filepath.Join(".", name)) {
			if _, ok := elsewhereByDesign[number]; ok {
				claimed[number] = true
				continue
			}
			strays = append(strays, name+":"+itoa(at.Line)+" "+number)
		}
	}
	sort.Strings(strays)
	if len(strays) > 0 {
		t.Errorf("a tuning number lives outside config.go, so nothing lists it as a "+
			"knob. Move it there, or add it to elsewhereByDesign with a reason:\n%s",
			strings.Join(strays, "\n"))
	}
	for name, reason := range elsewhereByDesign {
		if !claimed[name] {
			t.Errorf("elsewhereByDesign names %s, which no longer exists. It was "+
				"exempt because: %s", name, reason)
		}
	}
}

// Every number goes through the one helper, so a number declared in config.go
// and left out of the table is settable by nobody.
func TestEveryNumberInConfigGoIsInTheTable(t *testing.T) {
	t.Parallel()
	// Every knob here is declared with a type and set by the table, so a
	// package-level `name = 6` is one that escaped the helper.
	for name, at := range packageNumbers(t, "config.go") {
		t.Errorf("config.go:%d %s is assigned a literal rather than declared "+
			"through overridable(), so no environment name reaches it", at.Line, name)
	}
}

// Two knobs on one name means the second silently wins and the first is set by
// nothing, which reads exactly like a working override.
func TestNoTwoKnobsShareAnEnvironmentName(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool)
	for _, entry := range knobs() {
		if seen[entry.env] {
			t.Errorf("%s is declared twice, so one of the two is set by nothing", entry.env)
		}
		seen[entry.env] = true
	}
}

// Every name is this service's own, so a knob cannot collide with an unrelated
// variable a deployment already sets.
func TestEveryKnobNameIsPrefixed(t *testing.T) {
	t.Parallel()
	for _, entry := range knobs() {
		if !strings.HasPrefix(entry.env, "SIRENS_ECHO_") {
			t.Errorf("%s is not prefixed, so it can collide with something else "+
				"in the pod's environment", entry.env)
		}
	}
}

// A declared default must be the value the variable actually starts on, or the
// line reads as documentation of something that is not true.
func TestEveryKnobStartsOnItsDeclaredDefault(t *testing.T) {
	applyKnobs(func(string) string { return "" })
	for _, entry := range knobs() {
		if got := entry.value(); got != entry.fallback {
			t.Errorf("%s declares %s and its variable holds %s",
				entry.env, entry.fallback, got)
		}
	}
}

// Generated from the table, so it cannot fall behind what the code offers. It
// is rewritten rather than asserted, so moving a number stays one edit.
func TestTheKnobReferenceFollowsTheTable(t *testing.T) {
	path := filepath.Join("..", "..", "agent", "rendered", "knobs.txt")
	tracked, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the reference: %v", err)
	}
	rendered := RenderKnobReference()
	if string(tracked) == rendered {
		return
	}
	if err := os.WriteFile(path, []byte(rendered), 0o644); err != nil {
		t.Fatalf("rewrite the reference: %v", err)
	}
	t.Logf("%s followed a moved number and was rewritten. Commit it.", path)
}

// A deployment reads the prose page first, so it has to reach the list from
// there rather than from the source.
func TestTheOverrideDocPointsAtTheReference(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile(filepath.Join("..", "..", "docs", "sirens-echo-tuning.md"))
	if err != nil {
		t.Fatalf("read the overrides doc: %v", err)
	}
	if !strings.Contains(string(body), "agent/rendered/knobs.txt") {
		t.Error("the overrides doc never links the generated reference, so a " +
			"deployment cannot find the list of names")
	}
}

// itoa keeps the line number readable without pulling strconv in for one call.
func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}
