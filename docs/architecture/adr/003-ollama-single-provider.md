# ADR-003: Ollama as Sole AI Provider

**Status:** Accepted  
**Date:** 2026-04-19  
**Context:** ChronoCrystal needs an LLM provider for classification, chat, and tool-calling. TEMM1E supports multiple providers (Anthropic, OpenAI-compatible), but ChronoCrystal targets single-user self-hosted deployment.

**Decision:** Ollama is the only AI provider. The `Provider` interface has a single implementation: `OllamaProvider`.

Ollama runs locally, serves models via HTTP on `localhost:11434`, and has a Go client library (`github.com/ollama/ollama/api`) with native tool calling support.

**Alternatives Considered:**

1. **Multi-provider abstraction** — Support Anthropic, OpenAI, Ollama behind a common interface. Adds complexity, indirection, and testing burden. The single-user self-hosted deployment model makes Ollama the natural choice — the operator controls which model to run.
2. **OpenAI API directly** — Requires API key, internet access, and per-token cost. Conflicts with the self-hosted, no-external-dependency constraint.
3. **Anthropic API directly** — Same issues as OpenAI, plus Anthropic's tool calling format differs from Ollama's.

**Consequences:**

- **Positive:** Simplest possible provider layer — one implementation, no routing logic. Ollama is free, local, and the operator controls model selection. The `ollama/api` Go client handles streaming, tool calls, and error types natively.
- **Negative:** Tied to Ollama's API surface. If Ollama changes its tool calling format, we must adapt. No fallback to other providers if Ollama is down (mitigated by circuit breaker). Model quality depends on what the operator downloads.
- **Future:** Adding a second provider is straightforward — implement the `Provider` interface for it and add a routing field to config. The interface boundary is already clean.