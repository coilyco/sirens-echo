package community

import "testing"

// eco-app renamed get_eco_server_status to get_server_status (3d23bf2). The "checked"
// claim must ground on eco-game__get_server_status, or a true reply is refused. COI-2787
func TestACheckedClaimIsGroundedByTheLiveServerStatusTool(t *testing.T) {
	t.Parallel()
	const reply = "I checked the server status and Eco is online."
	for _, probe := range []struct {
		name     string
		executed []ExecutedTool
		refused  bool
	}{
		{"live server status tool", []ExecutedTool{{Name: "eco-game__get_server_status"}}, false},
		{"tracker search still grounds it", []ExecutedTool{{Name: "forgejo__search_issues"}}, false},
		{"the retired tool name no longer grounds it", []ExecutedTool{{Name: "eco__get_eco_server_status"}}, true},
		{"no tool call", nil, true},
	} {
		t.Run(probe.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateGrounding(reply, "", probe.executed...)
			if probe.refused && err == nil {
				t.Fatalf("%q was accepted with executed tools %v", reply, probe.executed)
			}
			if !probe.refused && err != nil {
				t.Fatalf("%q was refused with executed tools %v: %v", reply, probe.executed, err)
			}
		})
	}
}
