package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type ToolInput struct {
	Command string `json:"command"`
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
			"name":        "shell",
			"description": "Execute shell commands",
			"env_vars": map[string]interface{}{
				"WORKSPACE_DIR": "If set, shell commands run in this directory",
				"TOOL_TIMEOUT": "Override command timeout (Go duration format, default 60s)",
			},
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "The shell command to execute",
					},
				},
				"required": []string{"command"},
			},
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(desc)
		return
	}

	var input ToolInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		out := ToolOutput{Success: false, Error: fmt.Sprintf("invalid input: %v", err)}
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}

	if input.Command == "" {
		out := ToolOutput{Success: false, Error: "command is required"}
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}

	timeout := 60 * time.Second
	if ts := os.Getenv("TOOL_TIMEOUT"); ts != "" {
		if d, err := time.ParseDuration(ts); err == nil {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", input.Command)
	if wd := os.Getenv("WORKSPACE_DIR"); wd != "" {
		cmd.Dir = wd
	}
	outBytes, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		out := ToolOutput{Success: false, Error: fmt.Sprintf("command timed out after %s", timeout)}
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}

	if err != nil {
		// Non-zero exit: result gets stdout content, error gets the exit info
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		errMsg := fmt.Sprintf("exit code %d", cmd.ProcessState.ExitCode())
		if stderr != "" {
			errMsg = stderr
		}
		out := ToolOutput{
			Success: false,
			Result:  string(outBytes),
			Error:   errMsg,
		}
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}

	out := ToolOutput{Success: true, Result: string(outBytes)}
	json.NewEncoder(os.Stdout).Encode(out)
}