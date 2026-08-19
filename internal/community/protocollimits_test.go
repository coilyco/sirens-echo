package community

import "testing"

// These are not dials. Each is fixed by a system this service does not own, so
// a change to one is refused by Discord rather than tuned. See #659 and #660.

// A reply must fit Discord's own ceiling, which is 2000 characters. The
// harness extends a reply after composing it, so the constant sits under it.
func TestTheReplyLimitStaysUnderDiscordsOwn(t *testing.T) {
	t.Parallel()
	const discordMessageCeiling = 2000
	if discordReplyLimit >= discordMessageCeiling {
		t.Errorf("discordReplyLimit is %d and Discord refuses above %d. This is "+
			"not a tuning value: raising it makes every long reply fail to send",
			discordReplyLimit, discordMessageCeiling)
	}
}

// Discord refuses a longer thread name outright, so this cannot be raised and
// lowering it only truncates names the API would have accepted.
func TestTheThreadNameCapIsDiscordsCap(t *testing.T) {
	t.Parallel()
	const discordThreadNameCeiling = 100
	if threadTitleRunes > discordThreadNameCeiling {
		t.Errorf("threadTitleRunes is %d and Discord refuses above %d, so a thread "+
			"would fail to create rather than get a longer name",
			threadTitleRunes, discordThreadNameCeiling)
	}
}

// auto_archive_duration is an enum, not a duration. A value outside it is
// rejected by the API, so this is the one that most looks tunable and is not.
func TestTheArchiveDurationIsOneDiscordAccepts(t *testing.T) {
	t.Parallel()
	accepted := map[int]bool{60: true, 1440: true, 4320: true, 10080: true}
	if !accepted[threadArchiveMinutes] {
		t.Errorf("threadArchiveMinutes is %d and Discord accepts only 60, 1440, "+
			"4320 or 10080. A value between them is refused, not rounded",
			threadArchiveMinutes)
	}
}
