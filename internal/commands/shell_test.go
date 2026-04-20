package commands

import (
	"context"
	"strings"
	"testing"

	"github.com/chronocrystal/chronocrystal-go/internal/presenter"
)

// mockShellCmd is a test double for shell command execution.
type mockShellCmd struct {
	result shellResult
}

func (m *mockShellCmd) Run() shellResult {
	return m.result
}

func TestShellNoArgs(t *testing.T) {
	r := NewRegistry(presenter.Options{}, nil, nil)
	output, err := r.shellHandler(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("shellHandler error: %v", err)
	}
	if output != "usage: shell <command>" {
		t.Errorf("shellHandler no args = %q, want usage", output)
	}
}

func TestShellExecution(t *testing.T) {
	// Override execShellCommand for this test.
	orig := execShellCommand
	defer func() { execShellCommand = orig }()

	execShellCommand = func(_ context.Context, cmdStr string) shellCmd {
		return &mockShellCmd{
			result: shellResult{
				stdout:   "hello from shell",
				stderr:   "",
				exitCode: 0,
			},
		}
	}

	r := NewRegistry(presenter.Options{}, nil, nil)
	output, err := r.shellHandler(context.Background(), []string{"echo", "hello"}, "")
	if err != nil {
		t.Fatalf("shellHandler error: %v", err)
	}
	if !strings.Contains(output, "hello from shell") {
		t.Errorf("shellHandler = %q, want output containing 'hello from shell'", output)
	}
}

func TestShellWithStderr(t *testing.T) {
	orig := execShellCommand
	defer func() { execShellCommand = orig }()

	execShellCommand = func(_ context.Context, cmdStr string) shellCmd {
		return &mockShellCmd{
			result: shellResult{
				stdout:   "",
				stderr:   "warning: something",
				exitCode: 1,
			},
		}
	}

	r := NewRegistry(presenter.Options{}, nil, nil)
	output, err := r.shellHandler(context.Background(), []string{"false"}, "")
	if err != nil {
		t.Fatalf("shellHandler error: %v", err)
	}
	if !strings.Contains(output, "[stderr]") {
		t.Errorf("shellHandler = %q, want stderr in output", output)
	}
	if !strings.Contains(output, "[exit:1]") {
		t.Errorf("shellHandler = %q, want exit code in output", output)
	}
}

func TestHelpCommand(t *testing.T) {
	r := NewRegistry(presenter.Options{}, nil, nil)
	output, err := r.helpHandler(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("helpHandler error: %v", err)
	}
	if !strings.Contains(output, "cat") {
		t.Errorf("helpHandler = %q, want cat listed", output)
	}
	if !strings.Contains(output, "ls") {
		t.Errorf("helpHandler = %q, want ls listed", output)
	}
}

func TestFormatShellOutput(t *testing.T) {
	tests := []struct {
		name   string
		result shellResult
		want   []string // substrings that must appear
	}{
		{
			name: "success with stdout only",
			result: shellResult{stdout: "hello", stderr: "", exitCode: 0},
			want:  []string{"hello", "[exit:0]"},
		},
		{
			name: "failure with stderr",
			result: shellResult{stdout: "", stderr: "error msg", exitCode: 1},
			want:   []string{"[stderr]", "error msg", "[exit:1]"},
		},
		{
			name: "both stdout and stderr",
			result: shellResult{stdout: "output", stderr: "warn", exitCode: 0},
			want:   []string{"output", "[stderr]", "warn", "[exit:0]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatShellOutput(tt.result)
			for _, substr := range tt.want {
				if !strings.Contains(got, substr) {
					t.Errorf("formatShellOutput(%+v) = %q, missing %q", tt.result, got, substr)
				}
			}
		})
	}
}