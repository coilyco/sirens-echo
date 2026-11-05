package community

import (
	"strings"
	"testing"
)

// findTradeResult is a find_trade result as the harness records it: the text view,
// then the JSON. The names are invented.
func findTradeResult(offers ...string) []ExecutedTool {
	rows := make([]string, 0, len(offers))
	for _, o := range offers {
		rows = append(rows, `{"offers":[`+o+`]}`)
	}
	body := `{"cheapest":[` + strings.Join(rows, ",") + `]}`
	return []ExecutedTool{{Original: "find_trade", Result: "**Partial view.** x\n\n" + body}}
}

func offerJSON(item, store string, qty int) string {
	return `{"itemPretty":"` + item + `","store":"` + store + `","quantity":` + itoa(qty) + `}`
}

func sampleResults() []ExecutedTool {
	return findTradeResult(
		offerJSON("Fern Tea", "Mossbank's Fern & Co", 40),
		offerJSON("Fern Tea", "Tamsin's Quill Corner", 12),
		offerJSON("Bog Loaf", "Odalys's Quill Annex", 7),
		offerJSON("Bog Loaf", "Mossbank's Fern & Co", 0),
	)
}

func TestAStockClaimThatMatchesAnOfferIsSupported(t *testing.T) {
	reply := "- Fern Tea — 0.4 Embers at Mossbank's Fern & Co (40 units)\n" +
		"- Bog Loaf — 5.0 Barter at Odalys's Quill Annex (7 units)"

	check := CheckStockClaims(reply, sampleResults())

	if check.Claims != 2 || check.Supported != 2 || len(check.Unsupported) != 0 {
		t.Fatalf("check = %+v, want 2 claims both supported", check)
	}
}

func TestAQuantityNoOfferHasIsUnsupported(t *testing.T) {
	reply := "- Fern Tea — 0.4 Embers at Mossbank's Fern & Co (400 units)"

	check := CheckStockClaims(reply, sampleResults())

	if len(check.Unsupported) != 1 || !strings.Contains(check.Unsupported[0], "400 units") {
		t.Fatalf("check = %+v, want the 400-unit claim unsupported", check)
	}
}

func TestSameStoreCarriesTheStoreInOneParagraphLine(t *testing.T) {
	reply := "Fern Tea 0.4 Embers (Mossbank's Fern & Co, 40 units), Bog Loaf 5.0 Barter (same store, 0 units)."

	check := CheckStockClaims(reply, sampleResults())

	if check.Claims != 2 || check.Supported != 2 {
		t.Fatalf("check = %+v, want both claims supported through 'same store'", check)
	}
}

func TestAClaimWithNoNamedStoreIsUnresolvedNotUnsupported(t *testing.T) {
	reply := "- Fern Tea — 0.4 Embers (40 units)"

	check := CheckStockClaims(reply, sampleResults())

	if check.Unresolved != 1 || len(check.Unsupported) != 0 {
		t.Fatalf("check = %+v, want one unresolved claim", check)
	}
}

func TestAnItemListedInStockAndNamedOutOfStockIsAContradiction(t *testing.T) {
	reply := "- Fern Tea — 0.4 Embers at Mossbank's Fern & Co (40 units)\n" +
		"A few items are out of stock right now: Fern Tea and Moss Cake."

	check := CheckStockClaims(reply, sampleResults())

	if len(check.Contradictions) != 1 || !strings.Contains(check.Contradictions[0], "fern tea") {
		t.Fatalf("check = %+v, want fern tea flagged", check)
	}
}

func TestAnItemOnlyNamedOutOfStockIsNotAContradiction(t *testing.T) {
	reply := "- Fern Tea — 0.4 Embers at Mossbank's Fern & Co (40 units)\nBog Loaf is sold out."

	if got := CheckStockClaims(reply, sampleResults()).Contradictions; len(got) != 0 {
		t.Fatalf("contradictions = %v, want none", got)
	}
}

func TestAListLineWithAPriceAndNoStoreIsFlaggedAndProseIsNot(t *testing.T) {
	reply := "- Fern Tea — 0.4 Embers at Mossbank's Fern & Co (40 units)\n" +
		"- Huckleberries — 0.08 Embers\n" +
		"The cheapest food is Fern Tea at 0.4 Embers."

	check := CheckStockClaims(reply, sampleResults())

	if len(check.NoStore) != 1 || !strings.Contains(check.NoStore[0], "Huckleberries") {
		t.Fatalf("check = %+v, want only the huckleberries list line flagged", check)
	}
}

func TestAReplyThatReadNoFindTradeResultsHasNoSupportedClaim(t *testing.T) {
	check := CheckStockClaims("- Fern Tea — 0.4 Embers at Mossbank's Fern & Co (40 units)", nil)

	if check.Claims != 1 || check.Supported != 0 || check.Unresolved != 1 {
		t.Fatalf("check = %+v, want one unresolved claim", check)
	}
}

func TestAnOfferOnAnyBoardSupportsAClaim(t *testing.T) {
	body := `{"arbitrage":[{"itemPretty":"Fern Tea","buyFrom":` + offerJSON("Fern Tea", "Mossbank's Fern & Co", 40) +
		`,"sellTo":` + offerJSON("Fern Tea", "Tamsin's Quill Corner", 9) + `}],` +
		`"cheapest":[{"soldOutOffers":[` + offerJSON("Bog Loaf", "Odalys's Quill Annex", 0) + `]}]}`
	executed := []ExecutedTool{{Original: "find_trade", Result: "text\n" + body}}
	reply := "- Fern Tea — 0.4 Embers at Mossbank's Fern & Co (40 units)\n" +
		"- Fern Tea — 0.9 Embers at Tamsin's Quill Corner (9 units)"

	check := CheckStockClaims(reply, executed)

	if check.Supported != 2 || check.OffersSeen != 3 {
		t.Fatalf("check = %+v, want both claims supported from arbitrage offers, 3 offers seen", check)
	}
}

func TestATruncatedResultIsCountedNotTakenForAnInventedReply(t *testing.T) {
	executed := []ExecutedTool{{Original: "find_trade", Result: `text` + "\n" + `{"cheapest":[{"offers":[{"itemPr`}}

	check := CheckStockClaims("- Fern Tea — 0.4 Embers at Mossbank's Fern & Co (40 units)", executed)

	if check.JSONFailed != 1 || check.OffersSeen != 0 || check.Unresolved != 1 {
		t.Fatalf("check = %+v, want one failed parse, no offers, one unresolved claim", check)
	}
}

func TestAnUnresolvedClaimSaysWhatItCouldNotName(t *testing.T) {
	check := CheckStockClaims("- Fern Tea — 0.4 Embers at Nowhere Shop (40 units)", sampleResults())

	if len(check.UnresolvedClaims) != 1 || !strings.Contains(check.UnresolvedClaims[0], `item="fern tea" store=""`) {
		t.Fatalf("check = %+v, want the unnamed store reported", check)
	}
}
