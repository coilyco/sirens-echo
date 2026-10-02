package coalesce

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/coilyco/sirens-echo/internal/ingest"
)

// Tier selects the model a batch is answered on. Escalation is per batch, so
// one poisoned batch never moves the whole deployment onto the wider model.
type Tier string

const (
	TierStandard Tier = "standard"
	TierPro      Tier = "pro"
)

// Attempt is one try at a batch. The ladder is fixed: as configured, then with
// thinking off, then escalated, and a fourth try is a dead letter instead.
type Attempt struct {
	Number   int
	Tier     Tier
	Thinking bool
}

// MaxAttempts is where the ladder ends. Beyond it the batch is dead-lettered
// so the pool keeps draining.
const MaxAttempts = 3

// attemptFor reports how try n is made. Thinking off is tried before the wider
// model because a timeout is more often spent reasoning than under-powered.
func attemptFor(n int) Attempt {
	switch {
	case n <= 1:
		return Attempt{Number: 1, Tier: TierStandard, Thinking: true}
	case n == 2:
		return Attempt{Number: 2, Tier: TierStandard, Thinking: false}
	default:
		return Attempt{Number: 3, Tier: TierPro, Thinking: false}
	}
}

// Item is one distinct request and every ask that made it. Dedupe collapses
// the work rather than the asks: identical questions still owe an answer each.
type Item struct {
	Locus  string
	Text   string
	Covers []ingest.Ask
}

// Batch is one agent turn's worth of work for one tenant.
type Batch struct {
	Tenant   ingest.Tenant
	OpenedAt time.Time
	Items    []Item
	Attempt  Attempt
}

// Asks returns every ask the batch covers in arrival order, which is what the
// done marks and the dead-letter notice both iterate.
func (b Batch) Asks() []ingest.Ask {
	asks := make([]ingest.Ask, 0, len(b.Items))
	for _, item := range b.Items {
		asks = append(asks, item.Covers...)
	}
	sort.Slice(asks, func(i, j int) bool { return asks[i].Seq < asks[j].Seq })
	return asks
}

// Size counts the asks covered rather than the distinct items, because the
// batch-size metric answers how much arriving work one turn absorbed.
func (b Batch) Size() int {
	total := 0
	for _, item := range b.Items {
		total += len(item.Covers)
	}
	return total
}

// first is the earliest ask in the batch, which orders one flush's batches.
func (b Batch) first() int64 {
	lowest := int64(0)
	for _, item := range b.Items {
		for _, ask := range item.Covers {
			if lowest == 0 || ask.Seq < lowest {
				lowest = ask.Seq
			}
		}
	}
	return lowest
}

// fingerprint is the dedupe key. Normalization stops at case, whitespace, and
// trailing punctuation, because a looser match merges questions that differ.
func fingerprint(locus, text string) string {
	folded := strings.ToLower(strings.TrimSpace(text))
	folded = strings.Join(strings.Fields(folded), " ")
	folded = strings.TrimRightFunc(folded, func(r rune) bool {
		return unicode.IsPunct(r) || unicode.IsSpace(r)
	})
	sum := sha256.Sum256([]byte(locus + "\x00" + folded))
	return hex.EncodeToString(sum[:8])
}

// Criteria renders the batch's acceptance criteria. The lane composes a bundle
// whose curious personality widens a vague ask, so the goal is left no room.
func (b Batch) Criteria() string {
	var out strings.Builder
	fmt.Fprintf(&out, "Answer %d comment(s) from one member in one place.\n", b.Size())
	fmt.Fprintf(&out, "Surface: %s. Channel: %s. Member: %s.\n",
		b.Tenant.Surface, b.Tenant.Channel, b.Tenant.Author)
	out.WriteString("Cover every comment below and nothing beyond them.\n")
	for _, item := range b.Items {
		fmt.Fprintf(&out, "- [%s] %s\n", item.reference(), item.Text)
	}
	out.WriteString("Done means each comment above is answered in one reply, ")
	out.WriteString("and no question outside them is opened.")
	return out.String()
}

// reference names the comments an item covers, so the prompt can be checked
// against what was actually asked.
func (i Item) reference() string {
	ids := make([]string, 0, len(i.Covers))
	for _, ask := range i.Covers {
		ids = append(ids, fmt.Sprintf("#%d", ask.Seq))
	}
	return strings.Join(ids, ",")
}
