package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// GoRunner executes tool programs via `go run`.
type GoRunner struct {
	timeout time.Duration
}

// NewGoRunner creates a runner with the given per-invocation timeout.
func NewGoRunner(timeout time.Duration) *GoRunner {
	return &GoRunner{timeout: timeout}
}

// Run executes `go run ./tools/<toolName>` with input marshaled as JSON on stdin.
// It returns the parsed ToolOutput from stdout. If stderr contains content, it is
// included in the error. On timeout the process is killed.
func (g *GoRunner) Run(ctx context.Context, toolName string, input ToolInput) (ToolOutput, error) {
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return ToolOutput{}, fmt.Errorf("gorunner: marshal input: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "run", "./tools/"+toolName)
	cmd.Stdin = bytes.NewReader(inputJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Distinguish timeout from other failures.
		if ctx.Err() == context.DeadlineExceeded {
			return ToolOutput{}, fmt.Errorf("gorunner: tool %q timed out after %s: %s", toolName, g.timeout, stderr.String())
		}
		return ToolOutput{}, fmt.Errorf("gorunner: tool %q failed: %w\nstderr: %s", toolName, err, stderr.String())
	}

	var output ToolOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return ToolOutput{}, fmt.Errorf("gorunner: tool %q produced invalid JSON: %w\nstdout: %s\nstderr: %s",
			toolName, err, stdout.String(), stderr.String())
	}

	if stderr.Len() > 0 && output.Error == "" {
		output.Error = stderr.String()
	}

	return output, nil
}

// RunDescribe runs `go run ./tools/<toolName> --describe` and returns the
// tool's self-declaration (JSON schema for the LLM).
func (g *GoRunner) RunDescribe(ctx context.Context, toolName string) (ToolDeclaration, error) {
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "run", "./tools/"+toolName, "--describe")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return ToolDeclaration{}, fmt.Errorf("gorunner: describe %q timed out after %s: %s", toolName, g.timeout, stderr.String())
		}
		return ToolDeclaration{}, fmt.Errorf("gorunner: describe %q failed: %w\nstderr: %s", toolName, err, stderr.String())
	}

	var decl ToolDeclaration
	if err := json.Unmarshal(stdout.Bytes(), &decl); err != nil {
		return ToolDeclaration{}, fmt.Errorf("gorunner: describe %q produced invalid JSON: %w\nstdout: %s\nstderr: %s",
			toolName, err, stdout.String(), stderr.String())
	}

	return decl, nil
}