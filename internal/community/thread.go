package community

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

// A long answer gets somewhere of its own. The channel keeps the progress line,
// which is what points at it. See docs/sirens-echo-threads.md.

// threadNameFallback is every thread's name. The harness authors it so the
// channel never shows another member's truncated sentence (#1036).
const threadNameFallback = "a longer answer"

// hardTrimRunes cuts without an ellipsis. truncateRunes spends a rune saying it
// truncated, which is the thing this bound exists to avoid.
func hardTrimRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return strings.TrimSpace(string(runes[:limit]))
}

// threadCreationName is authored here rather than derived from the summoning
// message, which put another member's truncated sentence in the channel (#1036).
func threadCreationName() string {
	return hardTrimRunes(threadNameFallback, threadTitleRunes)
}

// threadForReply returns where a long turn's reply lands. No failure here is
// worth failing a turn over. See docs/sirens-echo-threads.md.
func threadForReply(
	session *discordgo.Session,
	message *discordgo.Message,
) (channelID string, threaded bool) {
	if session == nil || message == nil {
		return "", false
	}
	// A turn already inside a thread has one. Nesting is not permitted by
	// Discord and would be the wrong shape even if it were.
	if session.State != nil {
		channel, err := session.State.Channel(message.ChannelID)
		if err == nil && channel != nil && channel.IsThread() {
			return "", false
		}
	}
	thread, err := session.MessageThreadStartComplex(
		message.ChannelID,
		message.ID,
		&discordgo.ThreadStart{
			Name:                threadCreationName(),
			AutoArchiveDuration: threadArchiveMinutes,
			Invitable:           false,
		},
	)
	if err != nil || thread == nil || thread.ID == "" {
		return "", false
	}
	return thread.ID, true
}
