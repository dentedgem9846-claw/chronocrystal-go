package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chronocrystal/chronocrystal-go/internal/config"
	"github.com/chronocrystal/chronocrystal-go/internal/memory"
	"github.com/chronocrystal/chronocrystal-go/internal/skills"
	"github.com/chronocrystal/chronocrystal-go/internal/tools"
)

func setupTestContext(t *testing.T) (*ContextBuilder, *memory.Store) {
	t.Helper()

	cfg := &config.Config{
		Agent: config.AgentConfig{
			Model:              "test-model",
			ContextWindow:      4096,
			RecentMessagesKeep: 5,
		},
		Memory: config.MemoryConfig{
			DBPath:         ":memory:",
			AutoCommit:     false,
			LambdaDecay:    0.01,
			GoneThreshold:  0.01,
			LambdaBudgetPct: 0.15,
		},
		Provider: config.ProviderConfig{
			URL: "http://localhost:11434",
		},
		Channel: config.ChannelConfig{
			SimplexPath: "simplex-chat",
		},
		Tools: config.ToolsConfig{
			Dir: "./tools",
		},
	}

	store, err := memory.Open(cfg.Memory)
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	cb := NewContextBuilder(cfg, store, nil, nil)
	return cb, store
}

func TestBuildContext(t *testing.T) {
	cb, store := setupTestContext(t)

	// Create a conversation and add messages.
	conv, err := store.CreateConversation("test-contact", "TestUser")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	_, err = store.StoreMessage(conv.ID, memory.RoleUser, "Hello", 0.5, 2)
	if err != nil {
		t.Fatalf("StoreMessage: %v", err)
	}

	_, err = store.StoreMessage(conv.ID, memory.RoleAssistant, "Hi there!", 0.7, 3)
	if err != nil {
		t.Fatalf("StoreMessage: %v", err)
	}

	msgs, err := cb.Build(context.Background(), conv.ID, "What's up?")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Should have: system prompt + history messages + current user message.
	if len(msgs) < 3 {
		t.Errorf("expected at least 3 messages, got %d", len(msgs))
	}

	// First message should be system.
	if msgs[0].Role != "system" {
		t.Errorf("first message role = %q, want %q", msgs[0].Role, "system")
	}
	if msgs[0].Content == "" {
		t.Error("system message content is empty")
	}

	// Last message should be the current user message.
	lastMsg := msgs[len(msgs)-1]
	if lastMsg.Role != "user" {
		t.Errorf("last message role = %q, want %q", lastMsg.Role, "user")
	}
	if lastMsg.Content != "What's up?" {
		t.Errorf("last message content = %q, want %q", lastMsg.Content, "What's up?")
	}
}

func TestBuildContextCustomSystemPrompt(t *testing.T) {
	cfg := &config.Config{
		Agent: config.AgentConfig{
			Model:              "test-model",
			ContextWindow:      4096,
			RecentMessagesKeep: 5,
			SystemPrompt:       "You are a test assistant.",
		},
		Memory: config.MemoryConfig{
			DBPath:         ":memory:",
			AutoCommit:     false,
			LambdaDecay:    0.01,
			GoneThreshold:  0.01,
			LambdaBudgetPct: 0.15,
		},
		Provider: config.ProviderConfig{URL: "http://localhost:11434"},
		Channel:  config.ChannelConfig{SimplexPath: "simplex-chat"},
		Tools:    config.ToolsConfig{Dir: "./tools"},
	}

	store, err := memory.Open(cfg.Memory)
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	cb := NewContextBuilder(cfg, store, nil, nil)
	msgs, err := cb.Build(context.Background(), "nonexistent-conv", "test")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if msgs[0].Content != "You are a test assistant." {
		t.Errorf("system prompt = %q, want %q", msgs[0].Content, "You are a test assistant.")
	}
}

func TestTokenBudget(t *testing.T) {
	cb, store := setupTestContext(t)

	conv, err := store.CreateConversation("test-contact", "TestUser")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	// Add many messages to potentially exceed the budget.
	for i := 0; i < 50; i++ {
		content := "This is a somewhat longer message to consume token budget space in the context window"
		_, err := store.StoreMessage(conv.ID, memory.RoleUser, content, 0.5, len(content)/4)
		if err != nil {
			t.Fatalf("StoreMessage %d: %v", i, err)
		}
	}

	msgs, err := cb.Build(context.Background(), conv.ID, "current message")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// With a 4096 context window, we should not get all 50+1 messages back.
	// The exact count depends on token estimation, but it should be bounded.
	totalTokens := 0
	for _, m := range msgs {
		totalTokens += len(m.Content) / 4
	}
	// Token budget is 4096 - 1000 (output reserve) = 3096 for context.
	// Total should not vastly exceed this.
	if totalTokens > 5000 {
		t.Errorf("total estimated tokens = %d, expected budget-respecting amount", totalTokens)
	}
}

func TestOutputReserve(t *testing.T) {
	// Verify the constant is exported and matches the documented value.
	if outputTokenReserve != 1000 {
		t.Errorf("outputTokenReserve = %d, want 1000", outputTokenReserve)
	}
}

func TestBuildContextEmptyConversation(t *testing.T) {
	cb, _ := setupTestContext(t)

	// Nonexistent conversation ID should still work (returns just system + current user).
	msgs, err := cb.Build(context.Background(), "nonexistent-id", "hello")
	if err != nil {
		t.Fatalf("Build with nonexistent conversation: %v", err)
	}

	// Should have at least system prompt + current user message.
	if len(msgs) < 2 {
		t.Errorf("expected at least 2 messages, got %d", len(msgs))
	}

	// First message should be system.
	if msgs[0].Role != "system" {
		t.Errorf("first message role = %q, want %q", msgs[0].Role, "system")
	}
}

func TestToolDeclarationsNil(t *testing.T) {
	cb, _ := setupTestContext(t)

	// With nil tools, ToolDeclarations should return nil.
	apiTools, err := cb.ToolDeclarations(context.Background())
	if err != nil {
		t.Fatalf("ToolDeclarations: %v", err)
	}
	if apiTools != nil {
		t.Errorf("expected nil tools when registry is nil, got %v", apiTools)
	}
}

func TestToolDeclarationsWithRegistry(t *testing.T) {
	// Create a temporary project with a discoverable tool.
	// Create a temporary Go module with a discoverable tool subdirectory.
	root := t.TempDir()

	// Parent go.mod so `go run ./tools/test_tool` works.
	rootGoMod := "module test_root\n\ngo 1.26.1\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(rootGoMod), 0644); err != nil {
		t.Fatalf("WriteFile go.mod: %v", err)
	}

	toolsDir := filepath.Join(root, "tools", "test_tool")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}


	mainGo := `package main

import (
	"encoding/json"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--describe" {
		desc := map[string]interface{}{
			"name":        "test_tool",
			"description": "A test tool",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"input": map[string]interface{}{
						"type":        "string",
						"description": "Test input",
					},
				},
				"required": []string{"input"},
			},
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(desc)
	}
}
`
	if err := os.WriteFile(filepath.Join(toolsDir, "main.go"), []byte(mainGo), 0644); err != nil {
		t.Fatalf("WriteFile main.go: %v", err)
	}

	t.Chdir(root)

	// Use absolute path for the tools directory so Registry can find it.
	absToolsDir := filepath.Join(root, "tools")

	cfg := &config.Config{
		Agent: config.AgentConfig{
			Model:              "test-model",
			RecentMessagesKeep: 5,
		},
		Memory: config.MemoryConfig{
			DBPath:         ":memory:",
			AutoCommit:     false,
			LambdaDecay:    0.01,
			GoneThreshold:  0.01,
			LambdaBudgetPct: 0.15,
		},
		Provider: config.ProviderConfig{URL: "http://localhost:11434"},
		Channel:  config.ChannelConfig{SimplexPath: "simplex-chat"},
		Tools:    config.ToolsConfig{Dir: absToolsDir},
	}

	store, err := memory.Open(cfg.Memory)
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	toolReg := tools.NewRegistry(cfg.Tools.Dir)
	cb := NewContextBuilder(cfg, store, toolReg, nil)

	apiTools, err := cb.ToolDeclarations(context.Background())
	if err != nil {
		t.Fatalf("ToolDeclarations: %v", err)
	}

	if len(apiTools) == 0 {
		t.Error("expected tool declarations, got empty")
	}

	for _, tool := range apiTools {
		if tool.Function.Name == "" {
			t.Error("tool declaration missing function name")
		}
		if tool.Function.Description == "" {
			t.Error("tool declaration missing function description")
		}
	}
}

func TestBuildContextWithSkills(t *testing.T) {
	// Create a temp skills directory with a skill file.
	dir := t.TempDir()
	skillContent := "---\nname: test-skill\ndescription: A test skill\ntrigger_keywords: [test]\n---\nTest skill content here"
	if err := os.WriteFile(filepath.Join(dir, "test.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := &config.Config{
		Agent: config.AgentConfig{
			Model:              "test-model",
			ContextWindow:      4096,
			RecentMessagesKeep: 5,
		},
		Memory: config.MemoryConfig{
			DBPath:         ":memory:",
			AutoCommit:     false,
			LambdaDecay:    0.01,
			GoneThreshold:  0.01,
			LambdaBudgetPct: 0.15,
		},
		Provider: config.ProviderConfig{URL: "http://localhost:11434"},
		Channel:  config.ChannelConfig{SimplexPath: "simplex-chat"},
		Tools:    config.ToolsConfig{Dir: "./tools"},
	}

	store, err := memory.Open(cfg.Memory)
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	skillReg := skills.NewRegistry(dir)
	if err := skillReg.Discover(context.Background()); err != nil {
		t.Fatalf("skills.Discover: %v", err)
	}

	cb := NewContextBuilder(cfg, store, nil, skillReg)
	msgs, err := cb.Build(context.Background(), "nonexistent-id", "test message")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// System prompt should include the skill content when the message matches.
	systemContent := msgs[0].Content
	if systemContent == "" {
		t.Error("system content is empty")
	}
	// The skill instructions should be injected because "test message" matches "test" keyword.
	hasSkillContent := false
	for _, m := range msgs {
		if m.Role == "system" && len(m.Content) > len("You are ChronoCrystal") {
			hasSkillContent = true
		}
	}
	if !hasSkillContent {
		t.Error("expected system prompt to include skill content for matching keyword")
	}
}