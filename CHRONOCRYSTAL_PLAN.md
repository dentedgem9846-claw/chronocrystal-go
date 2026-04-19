# ChronoCrystal — Architecture Plan

A Go AI agent runtime with D&D time-dragon theming. Inspired by TEMM1E, simplified and re-architected for its specific constraints.

## Delta from TEMM1E

| Aspect | TEMM1E | ChronoCrystal |
|--------|--------|---------------|
| Language | Rust | Go |
| Channel | Telegram, Discord, WhatsApp, Slack, CLI | SimpleX Chat only |
| AI Provider | Anthropic, OpenAI-compatible | Ollama only |
| Tool Execution | Shell, browser, file ops (Rust impl) | `go run` SDK layer — each tool is a standalone Go program |
| Memory | SQLite + Markdown | DoltLite (version-controlled SQLite) |
| Theming | Tem (cat persona) | ChronoCrystal (D&D time dragon) |

## Core Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     ChronoCrystal                       │
│                  The Time Dragon's Lair                  │
├─────────────┬──────────────┬────────────────────────────┤
│  Channel    │  Agent Loop  │  Tool Execution            │
│  SimpleX    │  (The Mind)  │  (The Breath)              │
│             │              │                             │
│  simplex-   │  Classify    │  go run ./tools/<name>     │
│  chat bot   │  → Think    │  JSON stdin → JSON stdout  │
│  protocol   │  → Act      │  Sandboxed, versioned,     │
│             │  → Verify   │  permission-gated           │
│             │  → Learn    │                             │
├─────────────┴──────────────┴────────────────────────────┤
│                     Memory Layer                        │
│              DoltLite (version-controlled)              │
│  conversations, learnings, blueprints, user profiles    │
│  dolt_commit after every meaningful state change        │
└─────────────────────────────────────────────────────────┘
```

## Message Flow

```
SimpleX Chat ──► Channel (receive NewChatItems)
     │
     ▼
  Agent Loop
     │
     ├── Classify: single Ollama call (chat? order? stop?)
     │   └── If chat → reply directly, done
     │
     ├── Context Build: assemble prompt within token budget
     │   system prompt + history + tool defs + blueprints + λ-memory
     │
     ├── Tool Loop:
     │   ┌── Ollama Chat (with tools) ──┐
     │   │                              │
     │   │   tool_use? ──► go run ./tools/<name>
     │   │                    JSON in → JSON out
     │   │                    result fed back to LLM
     │   │                              │
     │   └── final text? ───────────────┘
     │
     ├── Post-Task: store memories, extract learnings, update blueprint
     │
     └── Send reply via SimpleX Chat
```

## Package Structure

```
chronocrystal-go/
├── cmd/
│   └── chronocrystal/
│       └── main.go              # CLI entry point (cobra)
├── internal/
│   ├── channel/                 # SimpleX Chat integration
│   │   ├── simplex.go           # SimpleX Chat bot protocol client
│   │   └── types.go             # SimpleX event/command types
│   ├── agent/                  # Agent runtime (The Dragon's Mind)
│   │   ├── runtime.go           # Main agent loop
│   │   ├── classify.go         # Single-call classifier
│   │   ├── context.go          # Context builder with token budget
│   │   ├── executor.go         # Tool dispatch + go-run execution
│   │   ├── verify.go           # Post-action verification
│   │   └── learn.go            # Cross-task learning extraction
│   ├── provider/               # Ollama integration
│   │   └── ollama.go           # Ollama API client (chat, generate, embed)
│   ├── memory/                 # DoltLite-backed memory
│   │   ├── doltlite.go         # DoltLite SQL driver wrapper
│   │   ├── conversations.go    # Conversation CRUD
│   │   ├── learnings.go        # Learning storage + retrieval
│   │   ├── blueprints.go       # Blueprint CRUD + matching
│   │   ├── lambda.go           # λ-Memory decay + fidelity
│   │   └── migrations.go       # Schema initialization
│   ├── tools/                  # Tool registry + go-run SDK layer
│   │   ├── registry.go         # Tool discovery + declaration
│   │   ├── gorunner.go         # go run execution engine
│   │   └── schema.go           # Tool I/O JSON schema types
│   └── config/
│       └── config.go           # TOML config loading
├── tools/                      # Tool programs (each is `go run`-able)
│   ├── shell/
│   │   └── main.go             # Shell command execution
│   ├── file_read/
│   │   └── main.go             # Read file contents
│   ├── file_write/
│   │   └── main.go             # Write file contents
│   ├── file_list/
│   │   └── main.go             # List directory
│   ├── web_search/
│   │   └── main.go             # Web search
│   └── web_fetch/
│       └── main.go             # Fetch URL content
├── config.example.toml
├── go.mod
├── go.sum
└── Makefile
```

## Key Design Decisions

### 1. SimpleX Chat Integration

SimpleX Chat bots communicate via the `simplex-chat` process over a local TCP socket or stdin/stdout JSON protocol. The bot:

1. Starts `simplex-chat` as a subprocess (or connects to an existing instance)
2. Sends API commands as JSON over the protocol
3. Receives events as JSON (NewChatItems, ContactConnected, etc.)
4. Processes messages and sends replies via `/_send` command

Key SimpleX concepts:
- **Contacts**: 1:1 conversations (auto-accept via address settings)
- **Groups**: Multi-user conversations
- **ChatRef**: Reference to a contact or group (used for sending)
- **ComposedMessage**: Message with text, file, or image content

### 2. Ollama Integration

Use `github.com/ollama/ollama/api` client directly. Single provider:

```go
client, _ := api.ClientFromEnvironment()
client.Chat(ctx, &api.ChatRequest{
    Model:    cfg.Model,
    Messages: messages,
    Tools:    toolDeclarations,
}, func(resp api.ChatResponse) error {
    // handle streaming response
    return nil
})
```

The Ollama API supports tool calling natively via the `Tools` field on `ChatRequest`. Tool results are fed back as `api.Message` with `Role: "tool"`.

### 3. `go run` Tool Execution (The Breath)

Inspired by Rhys Sullivan's SDK execution layer concept. Instead of raw shell access or in-process tool implementations, each tool is a standalone Go program in `tools/<name>/main.go`. The execution model:

```
Agent constructs JSON input
  → exec.Command("go", "run", "./tools/shell", "-input", jsonInput)
  → Tool program reads JSON from stdin or flag
  → Tool program writes JSON result to stdout
  → Agent parses JSON output
```

Benefits:
- **Permissions**: Each tool declares its resource needs; the runner gates access
- **Isolation**: Tool code runs in its own process; a crash doesn't take down the agent
- **Structure**: JSON I/O means no parsing ambiguity; the LLM sees clean schemas
- **Extensibility**: New tools = new directory + Go file; no recompilation of core
- **Auditing**: Every tool invocation has a clear input/output log

Tool I/O contract:
```go
type ToolInput struct {
    Command string `json:"command"`  // tool-specific
    // ... tool-specific fields
}

type ToolOutput struct {
    Success bool        `json:"success"`
    Result  string      `json:"result"`
    Error   string      `json:"error,omitempty"`
    Data    interface{} `json:"data,omitempty"`
}
```

### 4. DoltLite Memory (The Hoard)

DoltLite is a SQLite fork with Git-like version control. We use it via CGO + mattn/go-sqlite3 with `libsqlite3` build tag, linking against `libdoltlite.a`.

Schema (first version):

```sql
-- Conversations
CREATE TABLE conversations (
    id TEXT PRIMARY KEY,           -- UUID
    contact_id TEXT NOT NULL,      -- SimpleX contact ID
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

-- Messages
CREATE TABLE messages (
    id TEXT PRIMARY KEY,           -- UUID
    conversation_id TEXT NOT NULL REFERENCES conversations(id),
    role TEXT NOT NULL,            -- user, assistant, tool, system
    content TEXT NOT NULL,
    token_count INTEGER,
    importance REAL DEFAULT 1.0,   -- for λ-decay
    fidelity TEXT DEFAULT 'full',  -- full, summary, essence, hash
    created_at DATETIME NOT NULL,
    FOREIGN KEY (conversation_id) REFERENCES conversations(id)
);

-- Learnings
CREATE TABLE learnings (
    id TEXT PRIMARY KEY,
    task_type TEXT NOT NULL,
    approach TEXT NOT NULL,
    outcome TEXT NOT NULL,
    lesson TEXT NOT NULL,
    relevance_score REAL DEFAULT 1.0,
    created_at DATETIME NOT NULL
);

-- Blueprints (procedural memory)
CREATE TABLE blueprints (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    steps TEXT NOT NULL,           -- JSON array of steps
    fitness_score REAL DEFAULT 0.5,
    use_count INTEGER DEFAULT 0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

-- User profiles
CREATE TABLE user_profiles (
    contact_id TEXT PRIMARY KEY,
    display_name TEXT,
    communication_style TEXT,
    preferences TEXT,              -- JSON
    updated_at DATETIME NOT NULL
);
```

After every conversation turn, tool execution, and learning extraction, we `dolt_commit` the state. This gives us:
- Full history of every state change
- `dolt_diff` to see what changed between turns
- `dolt_reset` to undo a bad action
- `dolt_branch` for experimental reasoning paths

### 5. Agent Loop (The Dragon's Mind)

Adapted from TEMM1E's ORDER → THINK → ACTION → VERIFY cycle:

```
RECEIVE message from SimpleX
  │
  ├── CLASSIFY (single Ollama call)
  │   Is this chat? → reply directly
  │   Is this an order? → enter tool loop
  │   Is this a stop? → halt
  │
  ├── CONTEXT BUILD
  │   Assemble: system prompt + recent messages + tool defs
  │   + relevant blueprints + relevant learnings + λ-memory
  │   All within token budget (model's context window)
  │
  ├── TOOL LOOP
  │   ┌── Ollama Chat (with tools) ────────────────┐
  │   │                                              │
  │   │   response has tool_calls?                   │
  │   │     → execute each via go run               │
  │   │     → feed results back                     │
  │   │     → continue loop                         │
  │   │                                              │
  │   │   response is final text?                   │
  │   │     → break loop                            │
  │   └──────────────────────────────────────────────┘
  │
  ├── VERIFY
  │   If order was executed, verify result
  │   Inject verification prompt if needed
  │
  ├── LEARN
  │   Extract learnings from this interaction
  │   Update blueprints if applicable
  │   Store λ-memories with importance scores
  │
  └── REPLY via SimpleX
      dolt_commit() — version the memory state
```

### 6. D&D Time Dragon Theming

The theming is present in:
- **System prompt identity**: ChronoCrystal is a time dragon — ancient, patient, methodical. It perceives time in branches and commits.
- **Terminology**:
  - The Mind = agent loop (dragon's cognition)
  - The Breath = tool execution (dragon's breath weapon)
  - The Hoard = memory store (dragon's treasure hoard)
  - The Lair = the workspace/runtime
  - Temporal Breath = the go-run execution layer (time manipulation)
  - Chronal Echoes = λ-memories (echoes across time)
  - Dragon's Sight = verification (the dragon sees truth)
- **Log messages**: themed — "The dragon stirs", "Breathing fire: shell tool", "Adding to the hoard: new learning"

## Implementation Phases

### Phase 1: Foundation
- Go module init, project structure
- Config loading (TOML)
- DoltLite integration + schema migrations
- SimpleX Chat client (connect, receive events, send messages)
- Ollama client wrapper
- Basic agent loop (classify → respond, no tools yet)

### Phase 2: The Breath (Tool Execution)
- Tool registry + JSON I/O schema
- go-run execution engine
- Core tools: shell, file_read, file_write, file_list
- Tool loop integration with Ollama

### Phase 3: The Hoard (Memory)
- λ-memory decay + fidelity layers
- Learning extraction
- Blueprint storage + matching
- User profile tracking
- DoltLite commit after state changes

### Phase 4: Hardening
- Graceful shutdown (drain active tasks)
- Channel reconnection with backoff
- Circuit breaker for Ollama
- Token budget enforcement
- Context pruning

## Dependencies

- `github.com/ollama/ollama/api` — Ollama client
- `github.com/BurntSushi/toml` — Config parsing
- `github.com/google/uuid` — UUID generation
- `github.com/spf13/cobra` — CLI framework
- `github.com/mattn/go-sqlite3` — DoltLite access (CGO, libsqlite3 build tag)
- `github.com/pkoukk/tiktoken-go` — Token counting