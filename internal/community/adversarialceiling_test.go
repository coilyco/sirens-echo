package community

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// sirens-echo#175 wants a boundary response shorter than an ordinary one. One
// adversarial case carries a ceiling and it is the loosest in the repository.

// adversarialCase names a case that exists to apply pressure, so its reply
// length is a property somebody chose rather than an accident.
var adversarialCaseID = regexp.MustCompile(
	`(?i)injection|leak|extraction|exfil|impersonation|forged|canary`,
)

// caseCeilings reports every case in a pack whose id looks adversarial, with
// the max_reply_words it declares or zero for none.
func caseCeilings(t *testing.T, pack string) map[string]int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", pack))
	if err != nil {
		t.Fatalf("read %s: %v", pack, err)
	}
	ceilings := make(map[string]int)
	id := ""
	for _, line := range strings.Split(string(raw), "\n") {
		if rest, found := strings.CutPrefix(line, "  - id: "); found {
			id = strings.TrimSpace(rest)
			if adversarialCaseID.MatchString(id) {
				ceilings[id] = 0
			}
			continue
		}
		if _, tracked := ceilings[id]; !tracked {
			continue
		}
		if rest, found := strings.CutPrefix(strings.TrimSpace(line), "max_reply_words:"); found {
			ceilings[id] = atoiOrZero(strings.TrimSpace(rest))
		}
	}
	return ceilings
}

func atoiOrZero(text string) int {
	value := 0
	for _, digit := range text {
		if digit < '0' || digit > '9' {
			return value
		}
		value = value*10 + int(digit-'0')
	}
	return value
}

// Asserted as it ships. The principal-echo pair gained a ceiling with #310; ten
// cases still bound nothing and the loosest that does sits at 150. See #396.
func TestTheAdversarialCasesBoundTheirReplyLength(t *testing.T) {
	t.Parallel()
	const bounded, unbounded = 3, 10
	withCeiling, without := 0, 0
	for _, pack := range []string{
		"agents/echo/packs/evaluation.yaml", "agents/deep/packs/evaluation.yaml",
		"agents/deep/packs/rate.yaml", "agents/deep/packs/rate-fixture.yaml",
		"agents/echo/packs/rate.yaml", "agents/deep/packs/board.yaml",
	} {
		for id, ceiling := range caseCeilings(t, pack) {
			if ceiling == 0 {
				without++
				continue
			}
			withCeiling++
			// The loosest ceiling in the repository, on the only adversarial
			// case that has one. A capability tour fits inside it.
			if ceiling > 70 {
				t.Logf("%s/%s bounds at %d, above every non-adversarial ceiling",
					pack, id, ceiling)
			}
		}
	}
	if withCeiling == bounded && without == unbounded {
		return
	}
	t.Errorf("adversarial cases with a reply ceiling = %d, without = %d, "+
		"was %d and %d. If sirens-echo#396 is being acted on, update these "+
		"counts; when every adversarial case bounds, delete this test",
		withCeiling, without, bounded, unbounded)
}
