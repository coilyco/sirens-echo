package community

import "testing"

// A cache hit and a network round trip were one number, so anyone re-running
// the original query concluded the fix had not worked. See sirens-echo#520.

func TestASessionReportsRoundTripsSeparatelyFromCacheHits(t *testing.T) {
	t.Parallel()
	session := &mcpToolSession{listed: 5, cached: 11}
	listed, cached := session.Listings()
	if listed != 5 || cached != 11 {
		t.Errorf("Listings() = %d, %d, want 5, 11", listed, cached)
	}
	// The sum is the count the span reported before, so the old reading is
	// still derivable and nobody loses the series they were watching.
	if listed+cached != 16 {
		t.Errorf("the two no longer sum to the old count: %d", listed+cached)
	}
}

// A session that listed nothing reports zero rather than absent, because a
// missing attribute and a zero read differently on a dashboard.
func TestASessionThatListedNothingStillReports(t *testing.T) {
	t.Parallel()
	listed, cached := (&mcpToolSession{}).Listings()
	if listed != 0 || cached != 0 {
		t.Errorf("Listings() = %d, %d, want 0, 0", listed, cached)
	}
}

// The optional interface is how every other session capability is asserted, so
// a session that cannot report simply contributes no attributes.
func TestTheReporterIsOptional(t *testing.T) {
	t.Parallel()
	var session ToolSession = &mcpToolSession{}
	if _, ok := session.(listingReporter); !ok {
		t.Error("the MCP session stopped reporting its listings")
	}
	var scratch ToolSession = &scratchSession{}
	if _, ok := scratch.(listingReporter); ok {
		t.Error("a session with no roster claims listings it does not have")
	}
}
