# Harness design feature catalog

Model and tool harness features with the principle behind each one. Harnesses
may adopt them incrementally. This catalog does not claim current implementation.

- **Natural-language judgment** - Models interpret intent without harness keyword maps. ([ref][model-spec])
- **Harness authority** - Harnesses own permissions, execution, and budgets. ([ref][mcp-architecture])
- **Capability ownership** - Providers own tool meaning and behavior. ([ref][mcp-architecture])
- **Role composition** - Harnesses assemble explicit role instructions. ([ref][model-spec])
- **Identity binding** - Identity, attribution, and allowed actions stay aligned. ([ref][mcp-lifecycle])
- **Prompt assembly** - Instruction sources have a defined order and conflict policy. ([ref][model-spec])
- **Trust labeling** - Policy, input, context, and tool output keep distinct trust levels. ([ref][owasp-prompt])
- **Policy isolation** - Untrusted content cannot change harness policy. ([ref][owasp-prompt])
- **Delivery isolation** - Chat transports adapt messages, not model behavior. ([ref][mcp-architecture])
- **Invocation gates** - Delivery surfaces decide what starts a model turn. ([ref][discord-gateway])
- **Conversation continuity** - Context preserves the right thread and audience. ([ref][conversation-state])
- **Progressive disclosure** - Harnesses expose capabilities only when relevant. ([ref][mcp-architecture])
- **Semantic tool negotiation** - Models select capabilities by meaning, not keyword maps. ([ref][mcp-tools])
- **Tool schemas** - Providers define inputs, outputs, effects, and failures. ([ref][mcp-tools])
- **Tool-call budgets** - Harnesses bound executable requests and cost. ([ref][mcp-tools])
- **Tool continuation** - Results return to the same turn with its limits intact. ([ref][mcp-tools])
- **Structured response contracts** - Machine-consumed output follows a validated schema. ([ref][json-schema])
- **Grounding validation** - Tool-based claims must follow from observed output. ([ref][model-spec])
- **Bounded repair** - Repair and retry loops stay small, then fail closed. ([ref][owasp-prompt])
- **Action confirmation** - Models claim actions only after harness confirmation. ([ref][mcp-tools])
- **Human escalation** - Consequential ambiguity crosses a visible decision boundary. ([ref][mcp-elicitation])
- **Mention safety** - Generated text cannot create unintended notifications. ([ref][discord-message])
- **Secret isolation** - Credentials stay outside prompts, logs, and model arguments. ([ref][owasp-secrets])
- **Provider portability** - Model and tool providers sit behind verified contracts. ([ref][mcp-architecture])
- **Harness evaluation layers** - Core behavior and live delivery get separate tests. ([ref][openai-evals])

[model-spec]: https://model-spec.openai.com/
[mcp-architecture]: https://modelcontextprotocol.io/specification/2025-06-18/architecture
[mcp-lifecycle]: https://modelcontextprotocol.io/specification/2025-06-18/basic/lifecycle
[mcp-tools]: https://modelcontextprotocol.io/specification/2025-06-18/server/tools
[mcp-elicitation]: https://modelcontextprotocol.io/specification/2025-06-18/client/elicitation
[owasp-prompt]: https://cheatsheetseries.owasp.org/cheatsheets/LLM_Prompt_Injection_Prevention_Cheat_Sheet.html
[owasp-secrets]: https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html
[json-schema]: https://json-schema.org/understanding-json-schema/basics
[discord-gateway]: https://docs.discord.com/developers/events/gateway
[discord-message]: https://docs.discord.com/developers/resources/message#allowed-mentions-object
[conversation-state]: https://developers.openai.com/api/docs/guides/conversation-state
[openai-evals]: https://developers.openai.com/api/docs/guides/evals
