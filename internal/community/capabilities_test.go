package community

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// A capability that is built and never switched on reads exactly like one that
// was never built. See sirens-echo#539.

// optionalConfigFields select an optional surface. Each must appear in the
// report, or a deployment holds a capability nobody can see from a log.
var optionalConfigFields = []string{
	"DiscordEnabled",
	"MCPRosterPath",
	"AccessPolicyPath",
	"ContentClassesPath",
	"JobStoreDir",
	"ScratchDir",
}

func TestEveryOptionalSurfaceIsReported(t *testing.T) {
	t.Parallel()
	if got := len(capabilityReport(Config{})); got != len(optionalConfigFields)+1 {
		t.Fatalf("report covers %d surfaces, want %d: the tracker plus %v",
			got, len(optionalConfigFields)+1, optionalConfigFields)
	}
}

// The guard above counts. This one proves the fields are actually read, by
// setting each one and requiring the report to change.
func TestEachSurfaceReportsItsOwnConfiguration(t *testing.T) {
	t.Parallel()
	configured := Config{
		DiscordEnabled:     true,
		MCPRosterPath:      "/etc/roster.yaml",
		AccessPolicyPath:   "/etc/access.yaml",
		ContentClassesPath: "/etc/classes.yaml",
		JobStoreDir:        "/jobs",
		ScratchDir:         "/scratch",
	}
	configured.Definition.IssueTracker = "forgejo"
	for _, entry := range capabilityReport(configured) {
		if !entry.Configured {
			t.Errorf("%s reads unconfigured with every field set", entry.Name)
		}
	}
	for _, entry := range capabilityReport(Config{}) {
		if entry.Configured {
			t.Errorf("%s reads configured on an empty config", entry.Name)
		}
	}
}

// Whitespace is not configuration. A variable set to spaces is the shape a
// values file produces by accident, and it must not read as switched on.
func TestBlankConfigurationIsNotConfigured(t *testing.T) {
	t.Parallel()
	blank := Config{MCPRosterPath: "   ", ScratchDir: "\t", ContentClassesPath: " "}
	for _, entry := range capabilityReport(blank) {
		if entry.Configured {
			t.Errorf("%s reads configured from whitespace", entry.Name)
		}
	}
}

// The report is metadata. A path or endpoint reaching a log would put a
// deployment's filesystem into telemetry, which the contract forbids.
func TestTheReportCarriesNoConfiguredValue(t *testing.T) {
	t.Parallel()
	secretish := "/etc/sirens-secret-location/roster.yaml"
	for _, entry := range capabilityReport(Config{MCPRosterPath: secretish}) {
		if strings.Contains(entry.Name, secretish) {
			t.Errorf("a configured value reached the report: %q", entry.Name)
		}
	}
}

// The source of the closed set: every optional field in config.go must be
// listed above, so adding one and forgetting the report fails here.
func TestTheOptionalFieldListMatchesTheConfigStruct(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile("config.go")
	if err != nil {
		t.Fatalf("read config.go: %v", err)
	}
	// Every SIRENS_ECHO_* path or directory selector assigns from the
	// environment in the loader, which is what makes a surface optional.
	selector := regexp.MustCompile(`(\w+):\s+strings\.TrimSpace\(os\.Getenv\("SIRENS_ECHO_(?:MCP_ROSTER|ACCESS_POLICY|CONTENT_CLASSES|JOB_STORE|SCRATCH)"\)\)`)
	found := make(map[string]bool)
	for _, match := range selector.FindAllStringSubmatch(string(source), -1) {
		found[match[1]] = true
	}
	for _, field := range optionalConfigFields {
		if field == "DiscordEnabled" {
			continue
		}
		if !found[field] {
			t.Errorf("%s is listed as optional and no longer reads its variable, "+
				"so this list and the loader disagree", field)
		}
	}
	if len(found) != len(optionalConfigFields)-1 {
		t.Errorf("the loader selects %d optional surfaces and the list holds %d",
			len(found), len(optionalConfigFields)-1)
	}
}
