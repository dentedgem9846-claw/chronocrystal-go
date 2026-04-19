# Tools Guide

ChronoCrystal executes tools via the go-run SDK layer. This guide covers how tools work, the I/O contract, and how to create new tools.

## How Tools Work

Each tool is a standalone Go program in `tools/<name>/main.go`. When the LLM requests a tool call, ChronoCrystal:

1. Constructs a `ToolInput` JSON object with the function name and parameters
2. Runs `go run ./tools/<name>` with the JSON on stdin
3. Reads a `ToolOutput` JSON object from stdout
4. Feeds the result back to the LLM

The LLM decides which tools to call and with what arguments based on the tool declarations provided in the chat request. The agent runtime handles the dispatch loop.

### Self-Description

Every tool must support the `--describe` flag. When run with `--describe`, the tool prints its JSON schema to stdout and exits:

```bash
go run ./tools/shell --describe
```

Output:

```json
{
  "name": "shell",
  "description": "Execute shell commands",
  "parameters": {
    "type": "object",
    "properties": {
      "command": {
        "type": "string",
        "description": "The shell command to execute"
      }
    },
    "required": ["command"]
  }
}
```

The tool registry discovers all tools at startup by scanning `tools/` for subdirectories containing `main.go`, running each with `--describe`, and caching the declarations. These declarations are converted to Ollama's `api.Tools` format and passed to the LLM during the tool loop.

### Tool Discovery

Tools are discovered at startup by `tools.Registry.Discover()`. It:

1. Reads `config.tools.dir` (default `./tools`)
2. For each subdirectory containing `main.go`
3. Runs `go run <path> --describe` with a 30-second timeout
4. Caches the `ToolDeclaration` for 5 minutes

Add a new tool directory and restart ChronoCrystal to pick it up. No recompilation of the core binary is required.

## Tool I/O Schema

### ToolInput

The JSON object sent to a tool via stdin:

```go
type ToolInput struct {
    Command string          `json:"command"` // The action the tool should perform
    Params  json.RawMessage `json:"params"`   // Tool-specific parameters
}
```

For direct tool calls from the LLM, `command` is the tool name and `params` contains the LLM's function arguments.

### ToolOutput

The JSON object a tool must write to stdout:

```go
type ToolOutput struct {
    Success bool            `json:"success"`
    Result  string          `json:"result,omitempty"`
    Error   string          `json:"error,omitempty"`
    Data    json.RawMessage `json:"data,omitempty"`
}
```

- `Success` — required, indicates whether the operation succeeded
- `Result` — human-readable result text (or stdout content on failure)
- `Error` — error message if `Success` is false
- `Data` — optional structured data for programmatic consumers

### ToolDeclaration

The JSON object a tool emits when run with `--describe`:

```go
type ToolDeclaration struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    Parameters  json.RawMessage `json:"parameters"` // JSON Schema object
}
```

The `parameters` field must be a valid JSON Schema object describing the tool's input parameters. This is translated directly into Ollama's tool calling format.

## Creating a New Tool

### Step 1: Create the Tool Directory

```bash
mkdir -p tools/http_get
```

### Step 2: Write main.go

```go
package main

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "time"
)

type ToolInput struct {
    URL     string `json:"url"`
    Timeout int    `json:"timeout"` // seconds, default 10
}

type ToolOutput struct {
    Success bool        `json:"success"`
    Result  string      `json:"result"`
    Error   string      `json:"error,omitempty"`
    Data    interface{} `json:"data,omitempty"`
}

func main() {
    if len(os.Args) > 1 && os.Args[1] == "--describe" {
        desc := map[string]interface{}{
            "name":        "http_get",
            "description": "Fetch the content of a URL via HTTP GET",
            "parameters": map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "url": map[string]interface{}{
                        "type":        "string",
                        "description": "The URL to fetch",
                    },
                    "timeout": map[string]interface{}{
                        "type":        "integer",
                        "description": "Request timeout in seconds (default 10)",
                    },
                },
                "required": []string{"url"},
            },
        }
        enc := json.NewEncoder(os.Stdout)
        enc.SetIndent("", "  ")
        enc.Encode(desc)
        return
    }

    var input ToolInput
    if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
        json.NewEncoder(os.Stdout).Encode(ToolOutput{
            Success: false,
            Error:   fmt.Sprintf("invalid input: %v", err),
        })
        return
    }

    if input.URL == "" {
        json.NewEncoder(os.Stdout).Encode(ToolOutput{
            Success: false,
            Error:   "url is required",
        })
        return
    }

    timeout := input.Timeout
    if timeout <= 0 {
        timeout = 10
    }

    client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
    resp, err := client.Get(input.URL)
    if err != nil {
        json.NewEncoder(os.Stdout).Encode(ToolOutput{
            Success: false,
            Error:   fmt.Sprintf("request failed: %v", err),
        })
        return
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        json.NewEncoder(os.Stdout).Encode(ToolOutput{
            Success: false,
            Error:   fmt.Sprintf("read body failed: %v", err),
        })
        return
    }

    result := string(body)
    if len(result) > 10000 {
        result = result[:10000] + "\n... (truncated)"
    }

    json.NewEncoder(os.Stdout).Encode(ToolOutput{
        Success: true,
        Result:  result,
    })
}
```

### Step 3: Test

Test the describe flag:

```bash
echo '' | go run ./tools/http_get --describe
```

Test tool execution:

```bash
echo '{"url":"https://example.com"}' | go run ./tools/http_get
```

### Step 4: Restart ChronoCrystal

ChronoCrystal discovers tools at startup. Restart to pick up the new tool:

```bash
./chronocrystal start
```

The LLM will now see `http_get` in its available tools and can call it.

## Security

### Path Traversal Protection

File tools (`file_read`, `file_write`, `file_list`) enforce path safety via the `isPathSafe` function:

1. `filepath.Clean` and `filepath.Abs` normalize the path
2. Paths containing `..` are rejected
3. When the `WORKSPACE_DIR` environment variable is set, all file operations are restricted to that directory tree

This means you can sandbox the agent's file access by setting `WORKSPACE_DIR`:

```bash
export WORKSPACE_DIR=/home/agent/workspace
./chronocrystal start
```

### Tool Isolation

Each tool runs as a separate process. If a tool crashes or hangs, the GoRunner's context timeout kills it after `tool_timeout` seconds (default 60). The agent continues running.

### No Shared State

Tools communicate only through JSON stdin/stdout. There is no shared memory, global state, or direct access to the agent process. This isolation means:

- A tool bug cannot corrupt the agent's memory
- Tools cannot access the SimpleX connection or Ollama client
- Tool resource usage (CPU, memory) is bounded by OS process limits

## Existing Tools

### shell

Execute shell commands.

```json
// Input
{"command": "ls -la /tmp"}

// Output (success)
{"success": true, "result": "total 8\ndrwxrwxrwt 2 root root 4096 ..."}

// Output (failure)
{"success": false, "result": "...", "error": "exit code 1"}
```

- 60-second timeout
- `WORKSPACE_DIR` is not enforced for shell commands — use with caution

### file_read

Read file contents with optional line offset and limit.

```json
// Input
{"path": "/var/log/syslog", "offset": 0, "limit": 50}

// Output
{"success": true, "result": "...file contents..."}
```

- `offset` — 0-based line number to start from (default 0)
- `limit` — maximum lines to read (default 100, 0 means 100)
- Path traversal protection enforced
- `WORKSPACE_DIR` restricts allowed paths when set

### file_write

Write content to a file, creating parent directories if needed.

```json
// Input
{"path": "/tmp/notes.txt", "content": "Hello, world"}

// Output
{"success": true, "result": "wrote 12 bytes to /tmp/notes.txt"}
```

- Path traversal protection enforced
- `WORKSPACE_DIR` restricts allowed paths when set
- Creates intermediate directories with `os.MkdirAll`

### file_list

List directory contents, optionally recursively.

```json
// Input
{"path": "/tmp", "recursive": false}

// Output
{
  "success": true,
  "result": "[DIR] subdir\nfile.txt\n",
  "data": [{"name": "subdir", "size": 4096, "is_dir": true}, ...]
}
```

- Directories are prefixed with `[DIR]` in the text result
- `recursive` — walk subdirectories (default false)
- Path traversal protection enforced
- `WORKSPACE_DIR` restricts allowed paths when set