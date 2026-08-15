package community

import (
	"github.com/bwmarrin/discordgo"
)

// Inside a thread the whole thread is the context, not a window into it. See
// docs/sirens-echo-thread-prefill.md.

// inThread reports whether this turn is already inside a thread. It reads
// cached gateway state and makes no API call, like the nesting check does.
func inThread(session *discordgo.Session, message *discordgo.Message) bool {
	if session == nil || session.State == nil || message == nil {
		return false
	}
	channel, err := session.State.Channel(message.ChannelID)
	return err == nil && channel != nil && channel.IsThread()
}

// messagePager fetches one page older than `before`. Taken as a function so
// the page bound is testable without a live session.
type messagePager func(before string) ([]*discordgo.Message, error)

// threadMessages pages backwards to the start of the thread. Bounded by
// threadPrefillPages, so one very long thread cannot spend the rate limit.
func threadMessages(
	session *discordgo.Session, message *discordgo.Message,
) ([]*discordgo.Message, error) {
	return pageBack(func(before string) ([]*discordgo.Message, error) {
		return session.ChannelMessages(
			message.ChannelID, threadPrefillPageSize, before, "", "")
	}, message.ID)
}

// pageBack walks older until the thread starts or the page bound is reached.
func pageBack(fetch messagePager, before string) ([]*discordgo.Message, error) {
	collected := make([]*discordgo.Message, 0, threadPrefillPageSize)
	for page := 0; page < threadPrefillPages; page++ {
		batch, err := fetch(before)
		if err != nil {
			return nil, err
		}
		collected = append(collected, batch...)
		// A short page is the start of the thread. Discord returns newest
		// first, so the last entry is the oldest one seen.
		if len(batch) < threadPrefillPageSize {
			break
		}
		before = batch[len(batch)-1].ID
	}
	return collected, nil
}

// boundPrefill drops oldest first until the rendered history fits, and reports
// how many went. Silent truncation would make a thin answer unattributable.
func boundPrefill(history []TranscriptEntry, limit int) ([]TranscriptEntry, int) {
	total := 0
	for _, entry := range history {
		total += prefillCost(entry)
	}
	dropped := 0
	for dropped < len(history) && total > limit {
		total -= prefillCost(history[dropped])
		dropped++
	}
	return history[dropped:], dropped
}

// prefillCost counts what one entry contributes. Author and content, because
// both are rendered into the context. See docs/sirens-echo-thread-prefill.md.
func prefillCost(entry TranscriptEntry) int {
	return len([]rune(entry.Author)) + len([]rune(entry.Content))
}
