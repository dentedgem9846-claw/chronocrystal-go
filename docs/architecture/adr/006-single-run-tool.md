# ADR-006: Single `run` Tool with Unix-Style Command Routing

**Status:** Accepted  
**Date:** 2026-04-19  
**Supersedes:** ADR-001 (partially; go-run retained as extension mechanism)

**Context:**

ChronoCrystal exposes tools to the LLM agent so it can act on the user's behalf. The initial design (ADR-001) declared each capability as a separate tool — `read_file`, `write_file`, `list_directory`, `shell`, etc. This mirrors how GUI toolbars work: one button per action.

LLMs are not GUI users. They carry extensive knowledge of Unix CLI conventions — `cat`, `ls`, `grep`, pipes, and redirections are deeply represented in training data. Presenting a multi-tool catalog forces the model to spend context on selection overhead and produces fragmented tool-call sequences where a single pipeline would suffice.

Production experience from agent-clip validated a single `run` tool: fewer selection errors, fewer iterations, and natural pipe composition. Rhys Sullivan's SDK layer pattern provides structure and permissions over raw CLI, proving that a thin command router can offer safety without sacrificing composability.

The key observations:

1. **Cognitive load scales with tool count.** Each additional tool declaration consumes context tokens and increases selection ambiguity. A single tool with subcommands collapses this to one declaration.
2. **Unix pipes compose naturally.** `cat file | grep pattern` is one tool call. With separate tools, the same operation requires two round-trips with manual data threading.
3. **LLMs already know CLI.** Training data covers Unix commands extensively. Reusing familiar names (`cat`, `ls`, `grep`) reduces instruction overhead versus inventing custom tool names.
4. **Progressive disclosure works.** `help` lists available commands. Error messages suggest alternatives. The model discovers capabilities the same way a human does — by exploring.

**Decision:**

Replace the multi-tool catalog with a single `run(command, stdin)` tool that routes to built-in commands and SDK tool programs.

**Command routing:**

- **Built-in commands** execute in-process: `cat`, `ls`, `write`, `see`, `grep`, `memory`, `shell`, `help`, `stat`.
- **SDK tools** execute via go-run as `tool <name>` subcommand, preserving ADR-001's subprocess isolation and self-description model.

**Chain operators:**

- `|` — pipe: stdout of left becomes stdin of right
- `&&` — and: run right only if left succeeded
- `||` — or: run right only if left failed
- `;` — sequence: run right unconditionally

**Two-layer architecture:**

1. **Routing layer** (`internal/commands/`) — tokenizes the command string, resolves the handler, dispatches.
2. **Presentation layer** (`internal/presenter/`) — truncates, detects overflow, formats metadata footer. Applied once after the full pipeline completes.

**Progressive help and navigable errors:**

- `help` lists all registered commands with one-line descriptions.
- Unknown commands return `[error] unknown command: X. Available: cat, grep, help, ls, ...` — the error itself becomes a discovery mechanism.
- SDK tool discovery is deferred to `tool <name>` invocation — no upfront declaration cost.

**Alternatives Considered:**

1. **Multi-tool catalog (status quo)** — Each capability is a separate tool declaration. Familiar from OpenAI function-calling patterns but imposes selection overhead, prevents pipe composition, and inflates context usage with per-tool schemas.
2. **Raw shell passthrough** — `run(cmd: "sh -c '...'")`. Maximum flexibility but no permission gating, no overflow handling, no structured error recovery. Injection risk is unbounded.
3. **MCP (Model Context Protocol)** — Standard protocol for tool communication. More complex than needed; adds a network layer unnecessary for local execution. Doesn't solve the selection-overhead problem.

**Consequences:**

- **Positive:**
  - Single tool declaration — minimal context overhead for tool selection.
  - Pipe composition reduces round-trips: `cat file | grep pattern` is one call, not two.
  - LLMs leverage existing CLI knowledge — less instruction needed.
  - Extensible without recompilation: SDK tools via `tool <name>` subcommand.
  - Presentation layer applies once after pipeline — consistent truncation and overflow.
  - Error messages serve as navigation — failed commands teach available options.

- **Negative:**
  - Command routing logic is more complex than a flat tool map — tokenizer, chain parser, operator evaluation.
  - Injection risk exists for `shell` command. Mitigated by `WORKSPACE_DIR` scoping and presenter-level output limits.
  - Typed API consumers (non-LLM callers) lose per-tool JSON schemas. The single `run` tool accepts/returns strings. For structured I/O, SDK tools via go-run retain typed JSON contracts.
  - Debugging requires tracing through the routing layer rather than inspecting a single handler.

**Key Packages:**

- `internal/chain/` — chain operator parsing and segment representation
- `internal/commands/` — command registry, routing, built-in handlers
- `internal/presenter/` — output truncation, overflow, metadata formatting
- `internal/tools/gorunner.go` — retained for SDK tool subprocess execution