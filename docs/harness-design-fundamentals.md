# Harness design fundamentals

Working list of reusable principles for Sirens Echo and future harness changes.

- Models own natural-language judgment.
- Harnesses own permissions, execution, and budgets.
- Capability providers own the meaning and behavior of their tools.
- Harnesses progressively disclose executable capabilities.
- External actions fail closed.
- Tool, repair, and retry loops stay bounded.
- Untrusted input and tool output cannot change harness policy.
- The harness confirms an action before the model may claim it happened.
- Harness behavior can be evaluated without its delivery surface.
- Decisions, tool use, and failures remain observable.
- State and memory are explicit, source-owned, and bounded.
