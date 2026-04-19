# ADR-005: Simple Agent Loop (MVP)

**Status:** Accepted  
**Date:** 2026-04-19  
**Context:** TEMM1E implements a complex agent loop (ORDER → THINK → ACTION → VERIFY → DONE) with task decomposition, self-correction, blueprint matching, and cross-task learning. ChronoCrystal's MVP needs a simpler loop that still supports tool execution.

**Decision:** Implement a simple agent loop for MVP: `classify → context build → LLM with tools → reply`. 

Classification is a single Ollama call that categorizes the message as `chat`, `order`, or `stop`. For `chat`, reply directly. For `order`, enter a tool loop (Ollama Chat with tool declarations → execute tool calls → feed results back → repeat until final text). For `stop`, halt current task.

**What's deferred (not cut, just later):**
- Verification step (Dragon's Sight) — post-action verification that the order was fulfilled
- Cross-task learning extraction — extracting lessons from completed tasks
- Blueprint matching — procedural memory for repeated tasks
- λ-memory decay — fidelity layers and exponential decay
- Task decomposition — breaking compound orders into subtasks
- Self-correction — rotating approach after repeated failures

**Rationale:** The simple loop gets the core value proposition working: talk to an AI agent via SimpleX Chat, it executes tools, remembers conversations in DoltLite. Each deferred feature adds complexity that can be layered on top of the working core without breaking it.

**Alternatives Considered:**

1. **Full TEMM1E loop from day one** — Too complex for MVP. Verification, blueprints, and learning all require the basic loop to be solid first. Shipping all at once means shipping nothing solid.
2. **No classification — always enter tool loop** — Wastes tokens and time on simple chat messages. Classification is cheap (one call) and saves significant latency for casual conversation.
3. **Direct tool execution (no LLM mediation)** — Loses the reasoning capability entirely. The LLM decides which tools to call based on the user's intent — that's the whole point.

**Consequences:**

- **Positive:** Core loop ships fast and works. Each deferred feature has a clear integration point (verification after tool loop, learning after reply, blueprints before context build). The `classify → LLM → tools → reply` flow is testable end-to-end.
- **Negative:** Without verification, the agent may declare success when it failed. Without learning, it won't improve over time. Without blueprints, it re-derives procedures for repeated tasks. These are acceptable tradeoffs for MVP — the operator is present and can verify manually.
- **Future:** Each deferred feature maps to a `internal/agent/` file that hooks into the runtime: `verify.go`, `learn.go`, `blueprints.go`, `lambda.go`.