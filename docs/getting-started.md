# Getting Started

This guide walks you through installing, configuring, and running ChronoCrystal for the first time.

## Prerequisites

### Go 1.22+

ChronoCrystal requires Go with CGO enabled (the sqlite3 driver uses CGO).

```bash
go version
# Ensure CGO works:
CGO_ENABLED=1 go test -c /dev/null 2>/dev/null && echo "CGO OK" || echo "CGO not available"
```

On Debian/Ubuntu, install the C compiler if missing:

```bash
sudo apt install gcc libc6-dev
```

### Ollama

[Ollama](https://ollama.com) runs LLMs locally. Install it and pull a model:

```bash
# Install (macOS/Linux)
curl -fsSL https://ollama.com/install.sh | sh

# Pull the default model
ollama pull llama3.2

# Verify it's running
curl http://localhost:11434/api/tags
```

ChronoCrystal connects to Ollama at `http://localhost:11434` by default. If Ollama is on a different host or port, set `[provider]` URL in config.

### SimpleX Chat

SimpleX Chat is ChronoCrystal's messaging channel. It provides end-to-end encryption with no phone number or account required.

```bash
# Download from https://github.com/simplex-chat/simplex-chat
# Ensure it's on your PATH:
which simplex-chat
```

Install the SimpleX Chat mobile app on your phone from the [App Store](https://apps.apple.com/us/app/simplex-chat/id1605651084) or [Google Play](https://play.google.com/store/apps/details?id=com.simplex.chat).

## Installation

### From Source

```bash
git clone https://github.com/chronocrystal/chronocrystal-go.git
cd chronocrystal-go
make build
```

This produces the `chronocrystal` binary in the project root.

### With Docker

```bash
docker compose -f docker-compose.dev.yml up --build
```

The Docker image compiles with CGO enabled. It mounts `config.toml`, `data/`, `skills/`, and `tools/` as volumes.

## Configuration

### Create Your Config

```bash
cp config.example.toml config.toml
```

### Minimal Config

At minimum, set your model:

```toml
[agent]
model = "llama3.2"
```

All other values have sensible defaults. See [Configuration](../README.md#configuration) for the full reference.

### Key Settings

| Section | Setting | Default | Purpose |
|---------|---------|---------|---------|
| `[agent]` | `model` | `llama3.2` | Ollama model to use |
| `[agent]` | `max_tool_iterations` | `20` | Max tool calls before stopping |
| `[agent]` | `context_window` | `8192` | Token budget for LLM context |
| `[provider]` | `url` | `http://localhost:11434` | Ollama server URL |
| `[channel]` | `simplex_path` | `simplex-chat` | Path to simplex-chat binary |
| `[memory]` | `db_path` | `chronocrystal.db` | Database file path |

### Ollama Model Selection

Any model that supports tool calling works. Test with:

```bash
ollama run llama3.2 "What is 2+2?"
```

Other models known to work with Ollama tool calling: `mistral`, `qwen2.5`, `command-r`.

### Memory Tuning

The lambda memory system controls how ChronoCrystal manages conversation history within the token budget:

- `lambda_decay` (default `0.01`): Exponential decay rate. Higher values make memories fade faster.
- `gone_threshold` (default `0.01`): Below this decayed importance, memories collapse to `[gone]`.
- `lambda_budget_pct` (default `0.15`): Fraction of the context window reserved for lambda-selected older messages.
- `recent_messages_keep` (default `10`): Number of recent messages always kept at full fidelity.

## First Run

### Start ChronoCrystal

```bash
./chronocrystal start
```

You should see:

```
[main] the crystal awakens
```

### Create a SimpleX Address

ChronoCrystal automatically creates a SimpleX address on startup when `auto_accept = true`. Check the logs for the address link, or connect from your SimpleX Chat app by scanning the QR code or pasting the connection link.

### Connect from SimpleX Chat

1. Open the SimpleX Chat app on your phone
2. Tap "Add contact" or "Connect via link / QR code"
3. Paste the connection address from the ChronoCrystal logs or scan the QR code
4. ChronoCrystal auto-accepts the connection (when `auto_accept = true`)

### Send a Message

Once connected, send a message from SimpleX Chat:

- **"Hello there"** — classified as `chat`, ChronoCrystal responds conversationally
- **"List the files in /tmp"** — classified as `order`, enters the tool loop
- **"stop"** — classified as `stop`, halts current processing

The classification is handled by a single Ollama call. Casual messages skip the tool loop entirely, saving latency and tokens.

### Stopping

Send SIGINT (Ctrl+C) or SIGTERM:

```
[signal] received shutdown signal
[main] the crystal dims. goodbye.
```

ChronoCrystal gracefully shuts down: it closes contact channels, waits for in-flight messages, stops the SimpleX subprocess, and closes the database.

## Basic Usage

### Chat Messages

Simple conversational messages are classified as `chat` and get a direct LLM reply without tool use:

```
You: What's the weather like?
ChronoCrystal: I perceive time in branches, not forecasts. But I can help you check the weather if you give me a specific order.
```

### Orders

Task-oriented messages are classified as `order` and enter the tool loop. ChronoCrystal calls tools iteratively:

```
You: Read the file /var/log/syslog and summarize it
ChronoCrystal: [calls cat command] [processes result] Here's a summary of your syslog...
```

The tool loop continues until the LLM produces a final text response or hits `max_tool_iterations`.

### Stop Command

Sending "stop" (or a message classified as `stop`) halts the current processing. ChronoCrystal acknowledges:

```
ChronoCrystal: The crystal dims. I shall rest.
```

### Logs

ChronoCrystal logs to stdout (or a file if `logging.file` is set). Key log prefixes:

| Prefix | Source |
|--------|--------|
| `[the mind]` | Agent runtime |
| `[the breath]` | Tool execution |
| `[simplex-stderr]` | SimpleX Chat subprocess output |

## Troubleshooting

### Ollama Connection Refused

```
ollama chat request failed: dial tcp 127.0.0.1:11434: connect: connection refused
```

Ensure Ollama is running: `ollama serve`

### SimpleX Chat Not Found

```
channel: initial subprocess start: exec: "simplex-chat": executable file not found in $PATH
```

Install simplex-chat or set `channel.simplex_path` to its full path.

### Circuit Breaker Open

After 3 consecutive Ollama failures, the circuit breaker opens for 30 seconds:

```
circuit breaker open; retry after 29s
```

Check that Ollama is responsive and the model is pulled.

### Database Lock Errors

If you see `database is locked`, ensure only one ChronoCrystal instance is running against the same database file. DoltLite uses WAL mode but concurrent writes from multiple processes will conflict.