package community

import (
	"strings"
	"testing"
)

// The fallback for an unset instance name is a live service, so a profile that
// forgets it reports as Echo rather than as misconfigured. See sirens-echo#542.

// echoDefinition and deepDefinition carry only what the guard reads, so a row
// naming a path proves the path is not what decides it.
var (
	echoDefinition = Definition{Identity: "Sirens Echo", AuditRole: "community"}
	deepDefinition = Definition{Identity: "Sirens Deep of Coilyco", AuditRole: "general"}
)

// This is the shape Echo's own deployment ships: values.yaml sets neither
// SIRENS_ECHO_DEFINITION nor SIRENS_ECHO_INSTANCE. Breaking it renames Echo.
func TestEchoStillGetsItsNameFromNeitherVariable(t *testing.T) {
	t.Parallel()
	name, err := resolveInstanceName(echoDefinition, defaultDefinitionPath, "")
	if err != nil {
		t.Fatalf("Echo's own shipped configuration was rejected: %v", err)
	}
	if name != defaultInstanceName {
		t.Errorf("Echo resolved to %q, want %q", name, defaultInstanceName)
	}
}

// Echo's definition is Echo's definition wherever it is read from, and whatever
// the file is called. The filename comparison this replaced failed the last row.
func TestEchosDefinitionIsRecognisedByAnyPathAndName(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"../../agent/sirens-echo.yaml",
		"/app/agent/sirens-echo.yaml",
		"sirens-echo.yaml",
		"/tmp/fixture-1234/definition.yaml",
	} {
		name, err := resolveInstanceName(echoDefinition, path, "")
		if err != nil {
			t.Errorf("%s was refused: %v", path, err)
			continue
		}
		if name != defaultInstanceName {
			t.Errorf("%s resolved to %q, want %q", path, name, defaultInstanceName)
		}
	}
}

// The defect sirens-echo#706 reports: Deep's content in a file named
// sirens-echo.yaml was accepted, because the guard read the name and not the file.
func TestAForeignDefinitionCannotWearEchosFilename(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"/tmp/probe/sirens-echo.yaml",
		"agent/sirens-echo.yaml",
		defaultDefinitionPath,
	} {
		name, err := resolveInstanceName(deepDefinition, path, "")
		if err == nil {
			t.Errorf("Deep's definition at %s resolved to %q instead of refusing", path, name)
			continue
		}
		// The identity is what the reader needs, because the filename is the
		// thing that misled them into filing this.
		if !strings.Contains(err.Error(), deepDefinition.Identity) {
			t.Errorf("%s: the reason does not name the identity: %v", path, err)
		}
		if !strings.Contains(err.Error(), "SIRENS_ECHO_INSTANCE") {
			t.Errorf("%s: the reason does not name the variable: %v", path, err)
		}
	}
}

// Deep in production sets both, which must keep working unchanged.
func TestAConfiguredNameIsUsedForAnyDefinition(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		definition Definition
		path       string
		configured string
	}{
		{deepDefinition, "/app/agent/sirens-deep.yaml", "sirens-deep"},
		{echoDefinition, defaultDefinitionPath, "sirens-echo-canary"},
	} {
		name, err := resolveInstanceName(row.definition, row.path, row.configured)
		if err != nil {
			t.Errorf("%s with %q: %v", row.path, row.configured, err)
			continue
		}
		if name != row.configured {
			t.Errorf("%s resolved to %q, want %q", row.path, name, row.configured)
		}
	}
}

// An empty identity is not Echo's. A definition that declares none must not
// inherit the name by having nothing to compare.
func TestAnUnnamedDefinitionIsNotEcho(t *testing.T) {
	t.Parallel()
	for name, identity := range map[string]string{
		"empty":      "",
		"whitespace": "   ",
		"near miss":  "Sirens Echo of Coilyco",
		"case":       "sirens echo",
	} {
		_, err := resolveInstanceName(
			Definition{Identity: identity}, defaultDefinitionPath, "")
		if err == nil {
			t.Errorf("%s identity %q was accepted as Echo", name, identity)
		}
	}
}

// A variable set to spaces is unset. Treating it as a name would ship a service
// called "   " rather than fail, which is the same class of silence.
func TestAWhitespaceNameIsNotAName(t *testing.T) {
	t.Parallel()
	name, err := resolveInstanceName(echoDefinition, defaultDefinitionPath, "   ")
	if err != nil {
		t.Fatalf("Echo with a blank override was rejected: %v", err)
	}
	if name != defaultInstanceName {
		t.Errorf("a blank override produced %q", name)
	}
	if _, err := resolveInstanceName(deepDefinition, "agent/sirens-deep.yaml", "  \t "); err == nil {
		t.Error("a blank override satisfied the requirement for a foreign definition")
	}
}

// Surrounding space in a real name is trimmed, because service.name is compared
// literally in every query that reads it.
func TestAConfiguredNameIsTrimmed(t *testing.T) {
	t.Parallel()
	name, err := resolveInstanceName(deepDefinition, "agent/sirens-deep.yaml", "  sirens-deep\n")
	if err != nil {
		t.Fatalf("resolveInstanceName: %v", err)
	}
	if name != "sirens-deep" {
		t.Errorf("name = %q, want %q", name, "sirens-deep")
	}
}

// Echo's shipped definition really does declare the identity the guard compares
// against, or every row above tests a constant against itself.
func TestTheShippedDefinitionCarriesEchosIdentity(t *testing.T) {
	t.Parallel()
	definition, err := LoadDefinition("../../" + defaultDefinitionPath)
	if err != nil {
		t.Fatalf("LoadDefinition: %v", err)
	}
	if definition.Identity != echoIdentity {
		t.Errorf("the shipped definition declares %q, the guard expects %q",
			definition.Identity, echoIdentity)
	}
}
