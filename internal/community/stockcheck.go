package community

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// StockCheck compares a reply's stock claims with the find_trade results the turn
// read (COI-3446). It measures and never refuses: the reply is delivered either way.
type StockCheck struct {
	Claims         int
	Supported      int
	Unresolved     int      // a quantity claim whose item or store could not be named
	Unsupported    []string // an item at a store with a quantity no offer in the results has
	Contradictions []string // an item listed in stock and named out of stock
	NoStore        []string // a list line with a price and no store
	// UnresolvedClaims says which claims could not be named: "item=? store=? (N units)".
	UnresolvedClaims []string
	// OffersSeen and the JSON counts say what the check had to read: no offers with a
	// failed parse is a truncated result, not a reply that invented its numbers.
	OffersSeen, JSONOK, JSONFailed int
}

var (
	unitsClaim = regexp.MustCompile(`(\d[\d,]*) units?\b`)
	outOfStock = regexp.MustCompile(`(?i)out of stock|sold out`)
	priceToken = regexp.MustCompile(`\d+(?:\.\d+)?\s+[A-Z][A-Za-z]+`)
)

type stockOffer struct {
	item, store string
	qty         int
}

// stockRead is what stockOffers found in the executed find_trade results.
type stockRead struct {
	offers   []stockOffer
	jsonOK   int // result JSON objects that parsed
	jsonFail int // result lines that opened a JSON object and did not parse
}

// stockOffers reads every offer in every find_trade result the turn executed, on any
// board: an offer is any object with a store, an itemPretty and a quantity.
func stockOffers(executed []ExecutedTool) stockRead {
	var read stockRead
	for _, call := range executed {
		if call.Original != "find_trade" {
			continue
		}
		for _, line := range strings.Split(call.Result, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "{") {
				continue
			}
			var doc any
			if json.Unmarshal([]byte(line), &doc) != nil {
				read.jsonFail++
				continue
			}
			read.jsonOK++
			collectStockOffers(doc, &read.offers)
		}
	}
	return read
}

func collectStockOffers(node any, out *[]stockOffer) {
	switch value := node.(type) {
	case map[string]any:
		store, _ := value["store"].(string)
		item, _ := value["itemPretty"].(string)
		if qty, ok := value["quantity"].(float64); ok && store != "" && item != "" {
			*out = append(*out, stockOffer{
				item:  strings.ToLower(item),
				store: strings.ToLower(store),
				qty:   int(qty + 0.5),
			})
		}
		for _, child := range value {
			collectStockOffers(child, out)
		}
	case []any:
		for _, child := range value {
			collectStockOffers(child, out)
		}
	}
}

// CheckStockClaims measures reply against the find_trade results in executed.
func CheckStockClaims(reply string, executed []ExecutedTool) StockCheck {
	read := stockOffers(executed)
	items, stores := map[string]bool{}, map[string]bool{}
	have := map[stockOffer]bool{}
	for _, o := range read.offers {
		items[o.item], stores[o.store] = true, true
		have[o] = true
	}
	check := StockCheck{OffersSeen: len(read.offers), JSONOK: read.jsonOK, JSONFailed: read.jsonFail}
	listed := map[string]int{} // item -> quantity claimed
	for _, line := range strings.Split(strings.ToLower(reply), "\n") {
		lastItem, lastStore, prev := "", "", 0
		for _, loc := range unitsClaim.FindAllStringSubmatchIndex(line, -1) {
			span := line[prev:loc[1]]
			prev = loc[1]
			qty, _ := strconv.Atoi(strings.ReplaceAll(line[loc[2]:loc[3]], ",", ""))
			if item := longestIn(span, items); item != "" {
				lastItem = item
			}
			if store := longestIn(span, stores); store != "" {
				lastStore = store
			} else if !strings.Contains(span, "same") {
				lastStore = ""
			}
			check.Claims++
			switch {
			case lastItem == "" || lastStore == "":
				check.Unresolved++
				check.UnresolvedClaims = append(check.UnresolvedClaims,
					fmt.Sprintf("item=%q store=%q (%d units)", lastItem, lastStore, qty))
			case have[stockOffer{lastItem, lastStore, qty}]:
				check.Supported++
				listed[lastItem] += qty
			default:
				check.Unsupported = append(check.Unsupported,
					fmt.Sprintf("%s at %s (%d units)", lastItem, lastStore, qty))
				listed[lastItem] += qty
			}
		}
	}
	for _, sentence := range splitSentences(strings.ToLower(reply)) {
		if !outOfStock.MatchString(sentence) {
			continue
		}
		for _, name := range maximalNames(sentence, listed) {
			check.Contradictions = append(check.Contradictions,
				fmt.Sprintf("%s listed at %d units and named out of stock", name, listed[name]))
		}
	}
	for _, line := range strings.Split(reply, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(line)
		listLine := strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "*") ||
			strings.HasPrefix(trimmed, "•") || strings.Contains(trimmed, " — ")
		if listLine && priceToken.MatchString(line) && longestIn(lower, stores) == "" &&
			!strings.Contains(lower, "same store") {
			check.NoStore = append(check.NoStore, boundStockText(trimmed))
		}
	}
	return check
}

// longestIn is the longest key of set that occurs in text, or "".
func longestIn(text string, set map[string]bool) string {
	best := ""
	for name := range set {
		if name != "" && len(name) > len(best) && strings.Contains(text, name) {
			best = name
		}
	}
	return best
}

// maximalNames are the keys of set found in text, less any that sit inside a longer
// found key ("meat" inside "scrap meat").
func maximalNames(text string, set map[string]int) []string {
	var found []string
	for name, qty := range set {
		if name != "" && qty > 0 && strings.Contains(text, name) {
			found = append(found, name)
		}
	}
	var out []string
	for _, name := range found {
		inside := false
		for _, other := range found {
			if other != name && strings.Contains(other, name) {
				inside = true
			}
		}
		if !inside {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func splitSentences(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '.' })
}

func boundStockText(text string) string {
	if runes := []rune(text); len(runes) > 400 {
		return string(runes[:400]) + "..."
	}
	return text
}

// observeStockClaims logs the check for a reply that read find_trade or claims units.
func (a *Agent) observeStockClaims(ctx context.Context, reply string, executed []ExecutedTool) {
	if read := stockOffers(executed); read.jsonOK+read.jsonFail == 0 && !unitsClaim.MatchString(reply) {
		return
	}
	check := CheckStockClaims(reply, executed)
	a.telemetry.Info(
		ctx,
		"reply.stock.check",
		slog.Int("claims", check.Claims),
		slog.Int("supported", check.Supported),
		slog.Int("unresolved", check.Unresolved),
		slog.String("unresolved_claims", boundStockText(strings.Join(check.UnresolvedClaims, "; "))),
		slog.Int("offers_seen", check.OffersSeen),
		slog.Int("result_json_ok", check.JSONOK),
		slog.Int("result_json_failed", check.JSONFailed),
		slog.Int("unsupported", len(check.Unsupported)),
		slog.String("unsupported_claims", boundStockText(strings.Join(check.Unsupported, "; "))),
		slog.Int("contradictions", len(check.Contradictions)),
		slog.String("contradiction_claims", boundStockText(strings.Join(check.Contradictions, "; "))),
		slog.Int("no_store_lines", len(check.NoStore)),
		slog.String("no_store_text", boundStockText(strings.Join(check.NoStore, " | "))),
	)
}
