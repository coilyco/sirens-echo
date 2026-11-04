package community

import "bytes"

// LiveRunMarker opens every dataset a run against a live roster writes. The recorder
// puts it there and the commit check refuses it. docs/sirens-echo-testing.md has why.
const LiveRunMarker = "# eval-source: live"

// HasLiveRunMarker reports a body that records a live run, wherever the marker sits,
// since a pasted copy keeps it mid-file.
func HasLiveRunMarker(body []byte) bool {
	return bytes.Contains(body, []byte(LiveRunMarker))
}
