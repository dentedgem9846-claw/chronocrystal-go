# ADR-001: go run SDK Layer for Tool Execution

**Status:** Accepted  
**Date:** 2026-04-19  
**Context:** ChronoCrystal needs to execute tools on behalf of the LLM agent. Tools must be extensible without recompiling the core binary, and tool failures must be isolated from the agent process.

**Decision:** Use `go run ./tools/<name>` subprocess execution with JSON stdin/stdout I/O as the tool execution model.

Each tool is a standalone Go program in `tools/<name>/main.go`. The agent constructs JSON input, pipes it to the tool's stdin, and reads JSON output from stdout. Tool declarations are self-describing: running `go run ./tools/<name> --describe` outputs the tool's JSON schema.

**Alternatives Considered:**

1. **In-process tool execution** — Tools compiled into the main binary via plugin registry. Faster execution (no subprocess overhead) but: a crash in a tool crashes the entire agent, adding tools requires recompilation, and Go plugins have compatibility issues.
2. **Shell commands** — Direct shell command execution via `exec.Command("sh", "-c", cmd)`. Simpler but: unstructured output parsing, no permission gating, injection risks, no self-description capability.
3. **MCP (Model Context Protocol)** — Standard protocol for tool communication. More complex than needed for MVP, adds a network layer that's unnecessary for local execution.

**Consequences:**

- **Positive:** Tools are fully isolated — a crash doesn't affect the agent. Adding a new tool is adding a directory — no recompilation. Structured JSON I/O means no parsing ambiguity. Self-description means tool declarations are always in sync with implementation.
- **Negative:** First invocation of each tool compiles (~2-5s). Mitigated by pre-compiling with `go build -o` in production. Subprocess overhead per invocation (~5-10ms after compilation cache). No shared state between tool and agent without serialization.
- **Neutral:** The tool I/O contract (JSON stdin → JSON stdout) must be documented and stable.