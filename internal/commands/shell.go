package commands

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// shellResult holds the output of a shell command execution.
type shellResult struct {
	stdout   string
	stderr   string
	exitCode int
}

// shellCmd abstracts shell command execution for testability.
type shellCmd interface {
	Run() shellResult
}

// realShellCmd runs a real shell command via exec.CommandContext.
type realShellCmd struct {
	cmd *exec.Cmd
	dir string
}

func (c *realShellCmd) Run() shellResult {
	var stdout, stderr bytes.Buffer
	c.cmd.Stdout = &stdout
	c.cmd.Stderr = &stderr
	if c.dir != "" {
		c.cmd.Dir = c.dir
	}

	err := c.cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return shellResult{
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		exitCode: exitCode,
	}
}

// execShellCommand creates a shell command for the given command string.
// This is the production implementation. Tests can override via package-level var.
var execShellCommand = func(ctx context.Context, cmdStr string) shellCmd {
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	dir := os.Getenv("WORKSPACE_DIR")
	return &realShellCmd{cmd: cmd, dir: dir}
}

// formatShellOutput combines stdout and stderr with an exit code footer.
func formatShellOutput(r shellResult) string {
	var b strings.Builder
	if r.stdout != "" {
		b.WriteString(r.stdout)
	}
	if r.stderr != "" {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("[stderr] ")
		b.WriteString(r.stderr)
	}
	b.WriteString(fmt.Sprintf("\n[exit:%d]", r.exitCode))
	return b.String()
}