package community

import "testing"

func TestSeenMessagesDeduplicatesAndEvicts(t *testing.T) {
	t.Parallel()
	seen := newSeenMessages(2)
	if !seen.Add("one") {
		t.Fatal("first message was not accepted")
	}
	if seen.Add("one") {
		t.Fatal("duplicate message was accepted")
	}
	if !seen.Add("two") || !seen.Add("three") {
		t.Fatal("new messages were not accepted")
	}
	if !seen.Add("one") {
		t.Fatal("oldest message was not evicted")
	}
}

func TestAppendTrackingLinkKeepsDiscordLimit(t *testing.T) {
	t.Parallel()
	reply := make([]rune, 2100)
	for index := range reply {
		reply[index] = 'a'
	}
	got := appendTrackingLink(string(reply), "https://forgejo.example/issues/1")
	if len([]rune(got)) > 1990 {
		t.Fatalf("reply length = %d", len([]rune(got)))
	}
}
