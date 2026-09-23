# What one Sirens Deep turn sends to the model

2026-09-20. The question: is a turn one message, and how many times is the
history block paid for inside a turn. This measures the bytes of every request a
turn sends. It calls no model and touches no GPU, since a local test server
stands in for the model and the only network is localhost.

## Method
* `measure_test.go.txt` is a throwaway Go test. Copy it to
  `internal/community/zz_measure_test.go` in sirens-echo at `c49c593` and run
  `go test ./internal/community -run TestMeasureDeepTurn -v`. It is named `.txt`
  because this repository runs no Go, and sirens-echo is not this seat's to edit.
* It runs the real `runTurn` and the real `ProxyClient` against a local server that
  records each request body. The system prompt is Deep's rendered prompt from
  `agents/deep/rendered/prompt.txt`, and the budget is Deep's own: 12 tool rounds,
  16384-byte tool results, 3600 to 14400 completion tokens.
* Tool results are synthetic text at the 16 KB cap and history entries are synthetic
  text, so the sizes are right and the wording is not.
* `output.txt` is the verbatim output. A rerun was identical (`diff` exit 0).

## Measured (`output.txt`)
* A turn is one summon, not one model call. The answer is one `Complete` call plus
  one more per tool round, so a turn with 12 tool rounds sends 13 requests.
* Every request in a turn extends the previous one byte for byte, in all 6
  scenarios. Each request is the system prompt, a second system message of 176
  bytes, the turn context holding the history, the member's message, then the
  tool calls and results so far.
* No history: 1 request of 21293 bytes, of which the system prompt is 20995.
* Channel turn, 12 history entries of 1000 characters, the worst case:
  * first request 33475 bytes, history message 12258 (37%)
  * 3 tool rounds: 4 requests, 236518 bytes sent, system prompt 36%, history 21%
  * 12 tool rounds: 13 requests, 1769423 bytes sent, system prompt 15%, history 9%
* Thread turn, 32 KB prefill:
  * first request 54362 bytes, history message 33145 (61%)
  * 12 tool rounds: 13 requests, 2040954 bytes sent, system prompt 13%, history 21%

## Cross-checks
* The per-message sizes of the first request sum to its total: 20995 + 176 + 76 + 46
  is 21293.
* The channel worst case agrees with the repo's own figure. `docs/sirens-echo-telemetry.md`
  gives 15248 bytes for the variable half at a window of 12. Mine is 12480, and
  substituting its 80-rune authors and 2000-rune current message gives 15338, within
  0.6%.

## Read from source, not run
* The other model calls in a turn build their own small prompts and carry no history:
  the content gate (`contentgate.go`), the filing check with 2 stages
  (`filingcheck.go`), and the thread title (`thread.go`). Each sends a fixed system
  prompt plus one message.
* Whether Deep runs the content gate is a deployment fact, since it is enabled by the
  environment variable `SIRENS_ECHO_CONTENT_CLASSES` and not by the definition. I do
  not know how Deep is deployed.
* A channel turn carries at most `max_context_messages` history entries, 12 for Deep,
  each cut at 1000 characters. A thread turn prefills the whole thread up to
  `SIRENS_ECHO_THREAD_PREFILL_BYTES`, 32 KB by default.

## Limits
* These are bytes of the JSON request body, not tokens and not dollars. A token
  estimate is bytes over 4 and JSON escaping inflates it slightly.
* EXPECTED, unmeasured: because each request extends the previous one exactly,
  DeepSeek's automatic prefix cache should serve the system prompt and history from
  round 2 on, so the history is paid at the uncached price once per turn and not per
  round. That needs the provider's cache counters to confirm.
* The 176-byte second system message is probably the clock. I did not confirm that.

## Moved, 2026-09-23
First committed under `evaluations/sirens-deep-turn-2026-09-20/` in `coilyco-flight-deck/housecast`, whose
Git history holds its original timestamps. Moved here unchanged apart from this
section, because an evaluation lives in the repository that consumes its result
and Sirens Deep lives here. Scripts that import `housecast` ran against the
housecast of their date, which a rerun has to install.
