# ChronoCrystal

A Go AI agent runtime with D&D time-dragon theming. ChronoCrystal is an ancient time dragon whose essence has crystallized into form — it perceives past, present, and future as one, acting on your orders through SimpleX Chat with Ollama-powered reasoning.

The Mind speaks one language: `run(command="...")`.

```
┌──────────────────────────────────────────────────────────┐
│                      ChronoCrystal                       │
│                   The Time Dragon's Lair                  │
├──────────────┬───────────────┬────────────────────────────┤
│   Channel    │  Agent Loop   │   Command Execution        │
│   SimpleX    │  (The Mind)   │   (The Breath)             │
│              │               │                             │
│  simplex-    │  Classify     │  run(command="cat file")    │
│  chat bot    │  → Think      │  run(command="ls | grep x") │
│  protocol    │  → Act        │  Built-in + SDK tools       │
│              │  → Reply      │  Chain: | && || ;           │
├──────────────┴───────────────┴────────────────────────────┤
│                     Memory Layer                          │
│              DoltLite (version-controlled)                │
│  conversations · messages · learnings · blueprints        │
│  dolt_commit after every meaningful state change          │
└──────────────────────────────────────────────────────────┘
```

## The *nix Agent Philosophy

Unix made text streams its interface 50 years ago. LLMs made text their only language. ChronoCrystal brings these together: the LLM sees a single `run` tool and composes commands like a terminal operator — `cat file | grep error | wc -l` replaces three tool calls with one.

**Chain operators**: `|` (pipe), `&&` (and), `||` (or), `;` (seq) — compose commands within a single tool call.

**Two-layer architecture**: The execution layer is raw Unix — pipes carry data losslessly. The presentation layer sits between output and the LLM: binary guard, overflow truncation, metadata footer `[exit:0 | 12ms]`, stderr attachment on failure.

**Progressive discovery**: `help` shows all commands. `memory` shows usage. `memory search` shows parameters. Every error suggests the right command.

See [ADR-006](docs/architecture/adr/006-single-run-tool.md) for the full rationale.

## Quick Start

### Prerequisites

- Go 1.22+ (with CGO for sqlite3)
- [Ollama](https://ollama.com) running locally with a model pulled (e.g. `ollama pull llama3.2`)
- [SimpleX Chat](https://github.com/simplex-chat/simplex-chat) CLI binary

### Install

```bash
git clone https://github.com/chronocrystal/chronocrystal-go.git
cd chronocrystal-go
make build
```

Or with Docker:

```bash
docker compose -f docker-compose.dev.yml up --build
```

### Configure

```bash
cp config.example.toml config.toml
# Edit config.toml — set your Ollama model and adjust paths
```

The default config works with Ollama on `localhost:11434` and the `simplex-chat` binary on `$PATH`.

### Run

```bash
./chronocrystal start
```

ChronoCrystal starts the SimpleX Chat subprocess, creates an address, and begins listening. Connect from your SimpleX Chat client using the address printed in the logs.

For other commands:

```bash
./chronocrystal version   # Print version
```

## Project Structure

```
chronocrystal-go/
├── cmd/chronocrystal/       # CLI entry point
├── internal/
│   ├── agent/               # Agent runtime (The Mind)
│   │   ├── runtime.go       # Main loop: classify → context → LLM → reply
│   │   ├── classify.go      # Single-call message classifier
│   │   ├── context.go       # Context builder with token budget
│   │   ├── learn.go         # Learning extraction after task completion
│   │   └── blueprint.go     # Blueprint procedural memory extraction
│   ├── chain/                # Chain parser (|, &&, ||, ;)
│   ├── commands/             # Command registry (The Breath)
│   │   ├── registry.go      # Route + tokenize + exec chain
│   │   ├── fs.go            # Built-in: cat, ls, write, see, grep, stat
│   │   ├── memory_cmd.go    # Built-in: memory search/recent/store/facts/forget
│   │   ├── shell.go         # Built-in: shell (escape hatch)
│   │   ├── help.go          # Built-in: help
│   │   └── store_adapter.go # Memory store interface bridge
│   ├── channel/             # SimpleX Chat integration
│   │   ├── simplex.go       # Subprocess manager with reconnect
│   │   └── types.go         # Event/command types and parsers
│   ├── config/              # TOML config loading and validation
│   ├── memory/              # DoltLite-backed memory (The Hoard)
│   │   ├── doltlite.go      # Store wrapper with auto-commit
│   │   ├── conversations.go # Conversation CRUD
│   │   ├── messages.go      # Message storage with fidelity levels
│   │   ├── lambda.go        # λ-Memory decay and context selection
│   │   ├── learnings.go     # Learning extraction and storage
│   │   ├── blueprints.go    # Blueprint procedural memory
│   │   └── migrations.go    # Schema initialization
│   ├── presenter/            # Two-layer output presentation
│   │   └── presenter.go     # Binary guard, overflow, metadata footer
│   ├── provider/             # Ollama client with circuit breaker
│   ├── skills/               # Skill discovery and matching
│   └── tools/                # SDK tool execution (go-run layer)
│       ├── registry.go       # Discovery and caching
│       ├── gorunner.go       # `go run` subprocess execution
│       └── schema.go         # ToolInput / ToolOutput / ToolDeclaration
├── tools/                   # Directory for future SDK tool programs (each is `go run`-able)
├── skills/                  # Skill markdown files (YAML frontmatter)
├── config.example.toml      # Example configuration
├── Dockerfile               # Multi-stage build
├── docker-compose.dev.yml   # Development Docker setup
└── Makefile                 # Build, test, vet targets
```

## Key Concepts

### The Mind (Agent Loop)

The Mind is ChronoCrystal's reasoning core. Every incoming message is classified as **chat** (casual conversation), **order** (a task requiring tool execution), or **stop** (halt current work). Chat messages get a direct reply; orders enter the tool loop where the LLM iteratively calls `run`, examines results, and responds. The loop runs until the LLM produces a final text response or hits the iteration limit.

After each order, the Mind extracts **learnings** (what worked, what didn't) and **blueprints** (reusable procedures for multi-step tasks) from the conversation, storing them for future reference.

### The Breath (Command Execution)

The Breath is how ChronoCrystal acts on the world. The LLM sees a single `run` tool:

```
run(command="cat notes.md")
run(command="cat log.txt | grep ERROR | wc -l")
run(command="ls && cat README.md || echo 'not found'")
run(command="memory search 'deployment issue'")
run(command="tool http_get --describe")
```

**Built-in commands** (`cat`, `ls`, `write`, `see`, `grep`, `memory`, `shell`, `help`) run in-process for speed. **SDK tools** (`tool <name> <args>`) run via `go run` for isolation and extensibility. Chain operators (`|`, `&&`, `||`, `;`) compose commands within a single call.

**Two-layer architecture**: The execution layer is raw — pipes carry data losslessly. The presentation layer processes output before the LLM sees it: binary guard rejects non-text, overflow truncates large output with exploration hints, metadata footer `[exit:0 | 12ms]` gives the agent success/failure and cost signals, and stderr is attached on failure so the agent never guesses blindly.

### The Hoard (Memory)

The Hoard is ChronoCrystal's memory store, backed by DoltLite — a SQLite fork with Git-like version control. Every state change triggers a `dolt_commit`, giving a full audit trail. Messages are stored with importance scores and fidelity levels. Older, lower-importance messages decay through fidelity layers (full → summary → essence → hash) to stay within the token budget while preserving the most valuable context.

**Chronal Echoes** (λ-memories) apply exponential decay: `importance * e^(-λ * hours)`. Messages below the gone threshold are collapsed to `[gone]`.

**Learnings** capture task outcomes (approach, result, lesson) and are injected into context for similar future tasks.

**Blueprints** store reusable procedures (sequences of tool calls) and are matched to new orders by keyword similarity, giving the agent procedural memory.

### The Lair (Workspace)

The Lair is ChronoCrystal's runtime environment — the working directory, SimpleX Chat connection, and tool execution context.

## Configuration

Configuration is loaded from a TOML file (default `config.toml`).

```toml
[agent]
model = "llama3.2"              # Ollama model name
max_tool_iterations = 20         # Max tool calls per order
tool_timeout = 60                # Per-tool execution timeout (seconds)
context_window = 8192            # Token budget for LLM context
recent_messages_keep = 10        # Messages always kept at full fidelity
system_prompt = ""               # Override default dragon identity (optional)

[provider]
url = "http://localhost:11434"   # Ollama server URL
timeout = "120s"                  # Ollama request timeout
circuit_threshold = 3            # Failures before circuit breaker opens
circuit_cooldown = "30s"         # Time before half-open retry

[channel]
simplex_path = "simplex-chat"   # Path to simplex-chat binary
db_path = "simplex.db"          # SimpleX database file
auto_accept = true               # Auto-accept new contacts
max_retries = 20                 # Max consecutive reconnect failures
initial_backoff = "1s"          # First retry delay
max_backoff = "30s"             # Maximum retry delay
backoff_factor = 2.0             # Delay multiplier

[memory]
db_path = "chronocrystal.db"    # DoltLite database file
auto_commit = true               # dolt_commit after state changes
lambda_decay = 0.01              # λ-Memory exponential decay rate
gone_threshold = 0.01            # Below this score, memories become [gone]
lambda_budget_pct = 0.15         # Fraction of context window for λ-selected messages
learning_decay_factor = 0.95      # Score multiplier per decay cycle

[logging]
level = "info"                   # debug, info, warn, error
# file = ""                      # Log to file instead of stdout

[tools]
dir = "./tools"                  # Directory containing SDK tool programs
precompile = false                # Pre-build tools on startup
```

## Built-in Commands

| Command | Description | Key Flags |
|---------|-------------|-----------|
| `cat` | Read a text file | `-b` for base64 |
| `ls` | List files in directory | |
| `write` | Write to a file | `-b` for base64 input |
| `see` | View an image file | |
| `grep` | Filter lines matching a pattern | `-i`, `-v`, `-c` |
| `stat` | File info (size, MIME, mtime) | |
| `memory` | Search or manage memory | `search`, `recent`, `store`, `facts`, `forget` |
| `shell` | Execute shell command | Respects `WORKSPACE_DIR`, `TOOL_TIMEOUT` |
| `tool` | Invoke an SDK tool program | `tool <name> <args>` |
| `help` | List available commands | |

File commands enforce path traversal protection. When `WORKSPACE_DIR` is set, file operations are restricted to that directory tree.

See [docs/tools-guide.md](docs/tools-guide.md) for creating new SDK tools and the `run` command system.

## Skills

Skills are markdown files with YAML frontmatter stored in the `skills/` directory. They inject specialized knowledge into the system prompt when trigger keywords match the user's message.

See [docs/skills-guide.md](docs/skills-guide.md) for the skill format and creation guide.

## Further Reading

- [Getting Started](docs/getting-started.md) — detailed setup and first-run guide
- [Tools Guide](docs/tools-guide.md) — the `run` command system, chain operators, SDK tool development
- [Skills Guide](docs/skills-guide.md) — skill system and creation
- [Architecture Decisions](docs/architecture/adr/) — ADRs for key design choices
  - [ADR-001](docs/architecture/adr/001-go-run-sdk-layer.md) — go-run SDK layer
  - [ADR-006](docs/architecture/adr/006-single-run-tool.md) — Single `run` tool (*nix Agent)

## License

MIT