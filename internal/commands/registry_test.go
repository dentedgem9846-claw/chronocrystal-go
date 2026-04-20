package commands

import (
	"context"
	"strings"
	"testing"

	"github.com/chronocrystal/chronocrystal-go/internal/presenter"
)

// mockHandler returns a fixed string for testing.
func mockHandler(output string) CommandHandler {
	return func(_ context.Context, _ []string, _ string) (string, error) {
		return output, nil
	}
}

// mockErrorHandler returns an error for testing.
func mockErrorHandler(name string) CommandHandler {
	return func(_ context.Context, _ []string, _ string) (string, error) {
		return "", &commandError{name: name, msg: "failed"}
	}
}

// commandError is a simple error for testing.
type commandError struct {
	name string
	msg  string
}

func (e *commandError) Error() string {
	return e.msg
}

func TestRegisterAndHelp(t *testing.T) {
	r := NewRegistry(presenter.Options{}, nil, nil)
	help := r.Help()
	if len(help) == 0 {
		t.Error("Help() returned empty map, expected registered commands")
	}

	// Check that built-in commands are present.
	builtins := []string{"cat", "ls", "write", "see", "grep", "stat", "memory", "shell", "help", "tool"}
	for _, name := range builtins {
		if _, ok := help[name]; !ok {
			t.Errorf("Help() missing built-in command: %s", name)
		}
	}
}

func TestExecSingleCommand(t *testing.T) {
	r := NewRegistry(presenter.Options{}, nil, nil)
	r.Register("echo", "test echo", mockHandler("hello world"))

	output, err := r.Exec(context.Background(), "echo", "")
	if err != nil {
		t.Fatalf("Exec() error: %v", err)
	}
	if !strings.Contains(output, "hello world") {
		t.Errorf("Exec(\"echo\") = %q, want output containing 'hello world'", output)
	}
}

func TestExecPipeCommand(t *testing.T) {
	r := NewRegistry(presenter.Options{}, nil, nil)
	r.Register("echo", "test echo", func(_ context.Context, args []string, stdin string) (string, error) {
		if stdin != "" {
			return "piped: " + stdin, nil
		}
		return strings.Join(args, " "), nil
	})
	r.Register("upper", "test upper", func(_ context.Context, _ []string, stdin string) (string, error) {
		return strings.ToUpper(stdin), nil
	})

	output, err := r.Exec(context.Background(), "echo hello | upper", "")
	if err != nil {
		t.Fatalf("Exec() error: %v", err)
	}
	// The pipe should pass echo's output as stdin to upper.
	if !strings.Contains(output, "HELLO") {
		t.Errorf("Exec(\"echo hello | upper\") = %q, want output containing 'HELLO'", output)
	}
}

func TestExecAndChain(t *testing.T) {
	r := NewRegistry(presenter.Options{}, nil, nil)
	calls := 0
	r.Register("step1", "first", func(_ context.Context, _ []string, _ string) (string, error) {
		calls++
		return "ok", nil
	})
	r.Register("step2", "second", func(_ context.Context, _ []string, _ string) (string, error) {
		calls++
		return "done", nil
	})

	output, err := r.Exec(context.Background(), "step1 && step2", "")
	if err != nil {
		t.Fatalf("Exec() error: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
	if !strings.Contains(output, "done") {
		t.Errorf("output = %q, want 'done'", output)
	}
}

func TestExecAndChainSkip(t *testing.T) {
	r := NewRegistry(presenter.Options{}, nil, nil)
	step2Called := false
	r.Register("fail", "fail cmd", func(_ context.Context, _ []string, _ string) (string, error) {
		return "", &commandError{name: "fail", msg: "boom"}
	})
	r.Register("step2", "second", func(_ context.Context, _ []string, _ string) (string, error) {
		step2Called = true
		return "should not run", nil
	})

	_, _ = r.Exec(context.Background(), "fail && step2", "")
	if step2Called {
		t.Error("step2 should have been skipped because fail failed")
	}
}

func TestExecOrChain(t *testing.T) {
	r := NewRegistry(presenter.Options{}, nil, nil)
	step2Called := false
	r.Register("ok", "ok cmd", func(_ context.Context, _ []string, _ string) (string, error) {
		return "success", nil
	})
	r.Register("fallback", "fallback cmd", func(_ context.Context, _ []string, _ string) (string, error) {
		step2Called = true
		return "fallback", nil
	})

	output, err := r.Exec(context.Background(), "ok || fallback", "")
	if err != nil {
		t.Fatalf("Exec() error: %v", err)
	}
	if step2Called {
		t.Error("fallback should have been skipped because ok succeeded")
	}
	if !strings.Contains(output, "success") {
		t.Errorf("output = %q, want 'success'", output)
	}
}

func TestExecOrChainFallback(t *testing.T) {
	r := NewRegistry(presenter.Options{}, nil, nil)
	r.Register("fail", "fail cmd", func(_ context.Context, _ []string, _ string) (string, error) {
		return "", &commandError{name: "fail", msg: "error"}
	})
	r.Register("fallback", "fallback cmd", func(_ context.Context, _ []string, _ string) (string, error) {
		return "recovered", nil
	})

	output, err := r.Exec(context.Background(), "fail || fallback", "")
	if err != nil {
		t.Fatalf("Exec() error: %v", err)
	}
	if !strings.Contains(output, "recovered") {
		t.Errorf("output = %q, want 'recovered'", output)
	}
}

func TestExecUnknownCommand(t *testing.T) {
	r := NewRegistry(presenter.Options{}, nil, nil)

	output, err := r.Exec(context.Background(), "nonexistent", "")
	if err != nil {
		t.Fatalf("Exec() error: %v", err)
	}
	if !strings.Contains(output, "[error]") {
		t.Errorf("output = %q, want error message", output)
	}
	if !strings.Contains(output, "unknown command") {
		t.Errorf("output = %q, want 'unknown command'", output)
	}
	if !strings.Contains(output, "Available:") {
		t.Errorf("output = %q, want 'Available:' list", output)
	}
}

func TestToolDescription(t *testing.T) {
	r := NewRegistry(presenter.Options{}, nil, nil)
	desc := r.ToolDescription()
	if !strings.Contains(desc, "Your ONLY tool") {
		t.Errorf("ToolDescription() missing 'Your ONLY tool': %q", desc)
	}
	if !strings.Contains(desc, "cat") {
		t.Errorf("ToolDescription() missing 'cat' command: %q", desc)
	}
	if !strings.Contains(desc, "shell") {
		t.Errorf("ToolDescription() missing 'shell' command: %q", desc)
	}
	if !strings.Contains(desc, "Available commands:") {
		t.Errorf("ToolDescription() missing 'Available commands:': %q", desc)
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"hello world", []string{"hello", "world"}},
		{`echo "hello world"`, []string{"echo", `"hello world"`}},
		{`echo 'hello world'`, []string{"echo", `'hello world'`}},
		{`cat --props {"name": "x"}`, []string{"cat", "--props", `{"name": "x"}`}},
		{`cat --data [1, 2, 3]`, []string{"cat", "--data", "[1, 2, 3]"}},
		{`  ls   -la  `, []string{"ls", "-la"}},
		{`grep pattern file.txt`, []string{"grep", "pattern", "file.txt"}},
	}

	for _, tt := range tests {
		got := tokenize(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("tokenize(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("tokenize(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}