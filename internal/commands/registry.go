package commands

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/chronocrystal/chronocrystal-go/internal/chain"
	"github.com/chronocrystal/chronocrystal-go/internal/presenter"
	"github.com/chronocrystal/chronocrystal-go/internal/tools"
)

// CommandHandler processes a single command invocation.
type CommandHandler func(ctx context.Context, args []string, stdin string) (string, error)

// Registry routes command strings to built-in handlers and SDK tool invocations.
type Registry struct {
	handlers     map[string]CommandHandler
	descriptions map[string]string
	presenter    presenter.Options
	goRunner     *tools.GoRunner
	memStore     MemoryStore
}

// MemoryStore is the interface for memory operations. Nil means memory is unavailable.
type MemoryStore interface {
	SearchFacts(query string, limit int) ([]Fact, error)
	RecentSummaries(n int) ([]Summary, error)
	StoreFact(note string) (string, error)
	ListFacts() ([]Fact, error)
	ForgetFact(id string) error
}

// Fact represents a stored memory fact.
type Fact struct {
	ID      string
	Content string
}

// Summary represents a recent conversation summary.
type Summary struct {
	Content string
}

// NewRegistry creates a Registry with the given presenter config, Go runner, and memory store.
// goRunner and memStore may be nil (those commands will return availability errors).
func NewRegistry(presenterCfg presenter.Options, runner *tools.GoRunner, memStore MemoryStore) *Registry {
	r := &Registry{
		handlers:     make(map[string]CommandHandler),
		descriptions:  make(map[string]string),
		presenter:     presenterCfg,
		goRunner:      runner,
		memStore:      memStore,
	}

	// Register built-in commands.
	r.Register("cat", "Read a text file", r.catHandler)
	r.Register("ls", "List files in directory", r.lsHandler)
	r.Register("write", "Write to a file", r.writeHandler)
	r.Register("see", "View an image file", r.seeHandler)
	r.Register("grep", "Filter lines matching a pattern", r.grepHandler)
	r.Register("stat", "File metadata", r.statHandler)
	r.Register("memory", "Search or manage memory", r.memoryHandler)
	r.Register("shell", "Execute shell command", r.shellHandler)
	r.Register("help", "List available commands", r.helpHandler)
	r.Register("tool", "Invoke an SDK tool program", r.toolHandler)

	return r
}

// Register adds a command to the registry.
func (r *Registry) Register(name, description string, handler CommandHandler) {
	r.handlers[name] = handler
	r.descriptions[name] = description
}

// Help returns all command names and one-line descriptions, sorted by name.
func (r *Registry) Help() map[string]string {
	result := make(map[string]string, len(r.descriptions))
	for k, v := range r.descriptions {
		result[k] = v
	}
	return result
}

// ToolDescription returns the single `run` tool definition for Ollama.
func (r *Registry) ToolDescription() string {
	var b strings.Builder
	b.WriteString("Your ONLY tool. Execute commands via run(command=\"...\"). ")
	b.WriteString("Supports chaining: cmd1 && cmd2, cmd1 | cmd2, cmd1 || cmd2.\n\nAvailable commands:\n")

	// Sort command names for deterministic output.
	names := make([]string, 0, len(r.descriptions))
	for name := range r.descriptions {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		b.WriteString(fmt.Sprintf("  %s — %s\n", name, r.descriptions[name]))
	}
	return b.String()
}

// execSingle runs one command segment (name + args) against the registry.
// Returns the handler output, whether the command succeeded, and any error.
func (r *Registry) execSingle(ctx context.Context, name string, args []string, stdin string) (output string, failed bool) {
	handler, ok := r.handlers[name]
	if !ok {
		available := r.availableList()
		return fmt.Sprintf("[error] unknown command: %s. Available: %s", name, available), true
	}

	result, err := handler(ctx, args, stdin)
	if err != nil {
		return fmt.Sprintf("[error] %s: %s", name, err.Error()), true
	}
	return result, false
}

// availableList returns a sorted comma-separated list of registered command names.
func (r *Registry) availableList() string {
	names := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// Exec parses and executes a command string (with chain operators).
// Returns the final presentation-layer output for the LLM.
func (r *Registry) Exec(ctx context.Context, command string, stdin string) (string, error) {
	segments := chain.Parse(command)
	if len(segments) == 0 {
		return "[error] empty command", nil
	}

	var output string
	failed := false
	prevOp := chain.OpNone

	for i, seg := range segments {
		raw := seg.Raw

		// Chain operator logic from the previous segment's Op:
		// - OpAnd (&&): skip this segment if the previous one failed
		// - OpOr (||): skip this segment if the previous one succeeded
		// - OpSeq (;) and OpPipe (|): always execute
		switch prevOp {
		case chain.OpAnd:
			if failed {
				// Previous command failed, skip this one.
				prevOp = seg.Op
				continue
			}
		case chain.OpOr:
			if !failed {
				// Previous command succeeded, skip this one.
				prevOp = seg.Op
				continue
			}
		}

		// Tokenize the segment.
		tokens := tokenize(raw)
		if len(tokens) == 0 {
			prevOp = seg.Op
			continue
		}

		name := tokens[0]
		args := tokens[1:]

		// Determine stdin for this segment.
		segStdin := ""
		if i == 0 {
			segStdin = stdin
		} else if prevOp == chain.OpPipe {
			segStdin = output
		}

		result, cmdFailed := r.execSingle(ctx, name, args, segStdin)
		output = result
		failed = cmdFailed
		prevOp = seg.Op
	}

	// Apply presenter for final output formatting.
	return r.formatOutput(output), nil
}

// formatOutput applies presenter-level formatting to the final output.
func (r *Registry) formatOutput(output string) string {
	result := presenter.Present(
		[]byte(output), "", 0, 0, "",
		r.presenter,
	)
	return presenter.FormatResult(result)
}

// tokenize splits a command string into tokens, respecting:
// - Single and double quotes (content inside is one token)
// - Brace nesting: { and [ start a nested region that continues until balanced } or ]
func tokenize(input string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := rune(0) // 0 = not in quotes; otherwise the quote char
	braceDepth := 0    // tracks nesting of { and [

	runes := []rune(input)
	i := 0

	for i < len(runes) {
		r := runes[i]

		// Inside quotes: only end-quote closes.
		if inQuote != 0 {
			if r == inQuote {
				current.WriteRune(r)
				inQuote = 0
			} else {
				current.WriteRune(r)
			}
			i++
			continue
		}

		// Inside brace nesting: accumulate until balanced.
		if braceDepth > 0 {
			switch r {
			case '{', '[':
				braceDepth++
				current.WriteRune(r)
			case '}':
				braceDepth--
				current.WriteRune(r)
			case ']':
				braceDepth--
				current.WriteRune(r)
			case '"', '\'':
				inQuote = r
				current.WriteRune(r)
			default:
				current.WriteRune(r)
			}
			i++
			continue
		}

		// Start brace nesting.
		if r == '{' || r == '[' {
			braceDepth++
			current.WriteRune(r)
			i++
			continue
		}

		// Start quote.
		if r == '"' || r == '\'' {
			inQuote = r
			current.WriteRune(r)
			i++
			continue
		}

		// Whitespace: flush current token.
		if r == ' ' || r == '\t' {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			i++
			continue
		}

		current.WriteRune(r)
		i++
	}

	// Flush remaining token.
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// toolHandler dispatches to the GoRunner for SDK tool programs.
func (r *Registry) toolHandler(ctx context.Context, args []string, stdin string) (string, error) {
	if r.goRunner == nil {
		return "", fmt.Errorf("tool runner not available")
	}
	if len(args) == 0 {
		return "usage: tool <name> [args]", nil
	}

	toolName := args[0]
	toolArgs := ""
	if len(args) > 1 {
		toolArgs = strings.Join(args[1:], " ")
	}

	input := tools.ToolInput{
		Command: toolArgs,
	}

	result, err := r.goRunner.Run(ctx, toolName, input)
	if err != nil {
		return "", fmt.Errorf("tool %q: %w", toolName, err)
	}

	if !result.Success {
		return "", fmt.Errorf("%s", result.Error)
	}
	return result.Result, nil
}
// shellHandler executes a shell command via sh -c.
func (r *Registry) shellHandler(ctx context.Context, args []string, _ string) (string, error) {
	if len(args) == 0 {
		return "usage: shell <command>", nil
	}

	cmdStr := strings.Join(args, " ")
	timeout := 30 * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := execShellCommand(ctx, cmdStr)
	result := cmd.Run()
	return formatShellOutput(result), nil
}