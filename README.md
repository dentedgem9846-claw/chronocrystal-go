# ChronoCrystal

A Go AI agent runtime with D&D time-dragon theming. ChronoCrystal is an ancient time dragon whose essence has crystallized into form — it perceives past, present, and future as one, acting on your orders through SimpleX Chat with Ollama-powered reasoning and isolated tool execution.

```
┌──────────────────────────────────────────────────────────┐
│                      ChronoCrystal                       │
│                   The Time Dragon's Lair                  │
├──────────────┬───────────────┬────────────────────────────┤
│   Channel    │  Agent Loop   │   Tool Execution           │
│   SimpleX    │  (The Mind)   │   (The Breath)             │
│              │               │                             │
│  simplex-    │  Classify     │  go run ./tools/<name>      │
│  chat bot    │  → Think      │  JSON stdin → JSON stdout   │
│  protocol    │  → Act        │  Sandboxed, self-describing│
│              │  → Reply      │                             │
├──────────────┴───────────────┴────────────────────────────┤
│                     Memory Layer                          │
│              DoltLite (version-controlled)                │
│  conversations · messages · learnings · user profiles     │
│  dolt_commit after every meaningful state change          │
└──────────────────────────────────────────────────────────┘
```

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
├── cmd/chronocrystal/       # CLI entry point (cobra)
├── internal/
│   ├── agent/               # Agent runtime (The Mind)
│   │   ├── runtime.go       # Main loop: classify → context → LLM → reply
│   │   ├── classify.go      # Single-call message classifier
│   │   └── context.go       # Context builder with token budget
│   ├── channel/             # SimpleX Chat integration
│   │   ├── simplex.go       # Subprocess manager with reconnect
│   │   └── types.go         # Event/command types and parsers
│   ├── config/              # TOML config loading and validation
│   ├── memory/              # DoltLite-backed memory (The Hoard)
│   │   ├── doltlite.go      # Store wrapper with auto-commit
│   │   ├── conversations.go # Conversation CRUD
│   │   ├── messages.go      # Message storage with fidelity levels
│   │   ├── lambda.go        # λ-Memory decay and context selection
│   │   └── migrations.go    # Schema initialization
│   ├── provider/             # Ollama client with circuit breaker
│   └── tools/                # Tool registry and go-run executor (The Breath)
│       ├── registry.go       # Discovery and caching
│       ├── gorunner.go       # `go run` subprocess execution
│       └── schema.go         # ToolInput / ToolOutput / ToolDeclaration
├── tools/                   # Tool programs (each is `go run`-able)
│   ├── shell/               # Execute shell commands
│   ├── file_read/           # Read file contents
│   ├── file_write/          # Write file contents
│   └── file_list/           # List directory contents
├── skills/                  # Skill markdown files (YAML frontmatter)
├── config.example.toml      # Example configuration
├── Dockerfile               # Multi-stage build
├── docker-compose.dev.yml   # Development Docker setup
└── Makefile                 # Build, test, vet targets
```

## Key Concepts

### The Mind (Agent Loop)

The Mind is ChronoCrystal's reasoning core. Every incoming message is classified as **chat** (casual conversation), **order** (a task requiring tool execution), or **stop** (halt current work). Chat messages get a direct reply; orders enter the tool loop where the LLM iteratively calls tools, examines results, and responds. The loop runs until the LLM produces a final text response or hits the iteration limit.

### The Breath (Tool Execution)

The Breath is how ChronoCrystal acts on the world. Each tool is a standalone Go program under `tools/<name>/main.go`. The agent constructs JSON input, pipes it to the tool's stdin, and reads JSON output from stdout. Tools are isolated — a crash doesn't affect the agent. Adding a new tool is adding a directory; no recompilation required.

Tools self-describe: running `go run ./tools/<name> --describe` outputs the tool's JSON schema for the LLM.

### The Hoard (Memory)

The Hoard is ChronoCrystal's memory store, backed by DoltLite — a SQLite fork with Git-like version control. Every state change triggers a `dolt_commit`, giving a full audit trail. Messages are stored with importance scores and fidelity levels. Older, lower-importance messages decay through fidelity layers (full → summary → essence → hash) to stay within the token budget while preserving the most valuable context.

Chronal Echoes (λ-memories) apply exponential decay: `importance * e^(-λ * hours)`. Messages below the gone threshold are collapsed to `[gone]`.

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
timeout = 120                    # Ollama request timeout (seconds)

[channel]
simplex_path = "simplex-chat"   # Path to simplex-chat binary
db_path = "simplex.db"          # SimpleX database file
auto_accept = true               # Auto-accept new contacts

[memory]
db_path = "chronocrystal.db"    # DoltLite database file
auto_commit = true               # dolt_commit after state changes
lambda_decay = 0.01              # λ-Memory exponential decay rate
gone_threshold = 0.01            # Below this score, memories become [gone]
lambda_budget_pct = 0.15         # Fraction of context window for λ-selected messages

[logging]
level = "info"                   # debug, info, warn, error
# file = ""                      # Log to file instead of stdout

[tools]
dir = "./tools"                  # Directory containing tool programs
precompile = false                # Pre-build tools on startup
```

## Tools

| Tool | Description | Key Parameters |
|------|-------------|----------------|
| `shell` | Execute shell commands | `command` (string) |
| `file_read` | Read file contents with line offset/limit | `path`, `offset`, `limit` |
| `file_write` | Write content to a file | `path`, `content` |
| `file_list` | List directory contents | `path`, `recursive` |

File tools enforce path traversal protection. When `WORKSPACE_DIR` is set, file operations are restricted to that directory tree.

See [docs/tools-guide.md](docs/tools-guide.md) for creating new tools.

## Skills

Skills are markdown files with YAML frontmatter stored in the `skills/` directory. They inject specialized knowledge into the system prompt when trigger keywords match the user's message.

See [docs/skills-guide.md](docs/skills-guide.md) for the skill format and creation guide.

## Further Reading

- [Getting Started](docs/getting-started.md) — detailed setup and first-run guide
- [Tools Guide](docs/tools-guide.md) — tool development and I/O contract
- [Skills Guide](docs/skills-guide.md) — skill system and creation
- [Architecture Decisions](docs/architecture/adr/) — ADRs for key design choices

## License

MIT