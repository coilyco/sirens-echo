# Harness design feature catalog

Reusable harness features and the principle behind each one. Harnesses may
adopt them incrementally. This catalog does not claim current implementation.

- **Natural-language judgment** - Models interpret intent without harness keyword maps.
- **Harness authority** - Harnesses own permissions, execution, and budgets.
- **Capability ownership** - Providers own tool meaning and behavior.
- **Role composition** - Harnesses assemble explicit role instructions.
- **Identity binding** - Identity, attribution, and allowed actions stay aligned.
- **Prompt assembly** - Instruction sources have a defined order and conflict policy.
- **Trust labeling** - Policy, input, context, and tool output keep distinct trust levels.
- **Policy isolation** - Untrusted content cannot change harness policy.
- **Delivery isolation** - Chat transports adapt messages, not model behavior.
- **Invocation gates** - Delivery surfaces decide what starts a model turn.
- **Conversation continuity** - Context preserves the right thread and audience.
- **Duplicate suppression** - Delivery retries cannot create duplicate turns or replies.
- **Turn serialization** - Concurrent messages cannot corrupt shared state.
- **Progressive disclosure** - Harnesses expose capabilities only when relevant.
- **Semantic tool negotiation** - Models request capabilities by meaning before schemas appear.
- **Tool schemas** - Providers define inputs, outputs, effects, and failures.
- **Tool-call budgets** - Harnesses bound executable requests and cost.
- **Tool continuation** - Results return to the same turn with its limits intact.
- **Structured responses** - Machine-consumed output follows an explicit contract.
- **Output validation** - Malformed responses stop before delivery or execution.
- **Grounding validation** - Tool-based claims must follow from observed output.
- **Bounded repair** - Format repair, correction, and retries have small limits.
- **Fail-closed behavior** - Missing authority or uncertain effects stop execution.
- **Action confirmation** - Models claim actions only after harness confirmation.
- **Human escalation** - Consequential ambiguity crosses a visible decision boundary.
- **Mention safety** - Generated text cannot create unintended notifications.
- **Secret isolation** - Credentials stay outside prompts, logs, and model arguments.
- **Configuration ownership** - Each tunable lives at its determining layer.
- **Explicit state** - State has named owners and lifetimes.
- **Bounded memory** - Durable memory is intentional, reviewable, and limited.
- **Idempotent actions** - Retries cannot repeat a completed side effect.
- **Cancellation** - Superseded work stops without leaving partial actions.
- **Timeout budgets** - Queues, inference, tools, and delivery have separate deadlines.
- **Cold-start handling** - Slow startup stays distinct from a failed turn.
- **Degraded mode** - Optional failures remain visible without breaking core behavior.
- **Provider portability** - Model and tool providers sit behind verified contracts.
- **Trace correlation** - One request identity connects all turn events.
- **Decision observability** - Capability choices, gates, and failures stay inspectable.
- **Content observability** - Debug evidence balances diagnosis and privacy.
- **Metrics and alerts** - Latency, errors, tool use, and delivery are measurable.
- **Delivery-free evaluation** - Core behavior can be tested without chat transport.
- **Live-path evaluation** - End-to-end tests also cover the real adapter.
- **Rollback** - Changes retain a known path to the prior state.
- **Source-controlled knowledge** - Durable guidance lives beside its system.
- **Feedback capture** - Repeated corrections become reviewed tests or guidance.
