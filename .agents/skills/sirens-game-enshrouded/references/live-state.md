# Reading the live-state surface

Absent this surface, a question about the running Sirens world is an unknown
and saying so is the answer. Do not substitute what is generally true of the
game: a member cannot tell that reply from a verified one, which is what makes
it worse than none.

The surface answers whether the world is up, how many of the sixteen slots are
in use, who is on by name, savegame freshness, the last backup, free disk, and
a verdict. Three things decide whether a reply from it is true.

**It answering is not the world being up.** It runs on the machine hosting the
server rather than inside it, so a reply proves that machine is awake. Whether
the world is up is the `up` field and nothing else.

**Unreachable is not down.** That machine is a workstation that sleeps, so
silence is expected some of the time and is an **unknown**. Reporting it as
"the server is down" is a false claim rather than a cautious one.

**Names lag the count by up to five minutes.** The count is live, while names
come from the last five-minute autosave, so someone who joined since raises the
count without appearing. Give the count as current and the names as of that
autosave.

A verdict of **degraded** means the world answers while its savegame has gone
stale, which is worth saying plainly to whoever is on: their progress is at
risk while everything looks normal.
