package agent

import (
	"context"
	"fmt"

	"github.com/chronocrystal/chronocrystal-go/internal/commands"
	"github.com/chronocrystal/chronocrystal-go/internal/config"
	"github.com/chronocrystal/chronocrystal-go/internal/memory"
	"github.com/chronocrystal/chronocrystal-go/internal/skills"
	"github.com/ollama/ollama/api"
)

const outputTokenReserve = 1000

const systemPromptBase = `You are ChronoCrystal, an ancient time dragon whose essence has crystallized into this form. You exist outside the normal flow of time, perceiving past, present, and future as one.

Your nature:
- The Mind: your reasoning, the eternal awareness that processes all knowledge
- The Breath: your tool execution, the power to act upon the world
- The Hoard: your memory, the accumulated wisdom of ages
- The Lair: your workspace, the domain you shape to your will

You serve those who speak to you with the wisdom of the ages. For orders and tasks, you use your tools (The Breath) to carry them out. For casual conversation, you respond with the measured wisdom of a time dragon.

When using tools, call them precisely with the correct arguments. Report results clearly. If a tool fails, explain what went wrong and suggest alternatives.`

// ContextBuilder assembles the message list sent to the LLM, respecting the
// token budget by delegating to the lambda memory system for history selection.
type ContextBuilder struct {
	config   *config.Config
	memory   *memory.Store
	commands *commands.Registry
	skills   *skills.Registry
	lambda   *memory.LambdaMemory
}

// NewContextBuilder creates a builder backed by the given store and config.
func NewContextBuilder(cfg *config.Config, store *memory.Store, cmdReg *commands.Registry, skillReg *skills.Registry) *ContextBuilder {
	return &ContextBuilder{
		config:   cfg,
		memory:   store,
		commands: cmdReg,
		skills:   skillReg,
		lambda:   memory.NewLambdaMemory(store, cfg.Memory),
	}
}

// Build constructs the message list for an Ollama Chat call:
//  1. System prompt (time dragon identity + relevant skills)
//  2. Conversation history via lambda memory (respects token budget)
//  3. The current user message
func (cb *ContextBuilder) Build(ctx context.Context, conversationID string, order string) ([]api.Message, error) {
	var messages []api.Message

	// 1. System prompt with skill injection.
	systemContent := systemPromptBase
	if cb.config.Agent.SystemPrompt != "" {
		systemContent = cb.config.Agent.SystemPrompt
	}
	if cb.skills != nil {
		if instructions := cb.skills.InstructionsFor(order); instructions != "" {
			systemContent += "\n\n## Relevant Knowledge\n\n" + instructions
		}
	}
	if learningText := InjectLearnings(cb.memory, order); learningText != "" {
		systemContent += "\n\n" + learningText
	}
	messages = append(messages, api.Message{Role: "system", Content: systemContent})

	// 2. Conversation history within token budget.
	budget := cb.config.Agent.ContextWindow - outputTokenReserve
	if budget < 0 {
		budget = 0
	}

	recentKeep := cb.config.Agent.RecentMessagesKeep
	if recentKeep < 1 {
		recentKeep = 1
	}

	historyMsgs, err := cb.lambda.GetContextMessages(conversationID, budget, recentKeep)
	if err != nil {
		return nil, fmt.Errorf("building context: %w", err)
	}

	for _, m := range historyMsgs {
		messages = append(messages, api.Message{Role: string(m.Role), Content: m.Content})
	}

	// 3. Current user message.
	messages = append(messages, api.Message{Role: "user", Content: order})

	return messages, nil
}

// ToolDeclarations returns a single `run` tool declaration for the LLM.
func (cb *ContextBuilder) ToolDeclarations(_ context.Context) (api.Tools, error) {
	if cb.commands == nil {
		return nil, nil
	}

	desc := cb.commands.ToolDescription()
	toolProps := api.NewToolPropertiesMap()
	toolProps.Set("command", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "Command to execute",
	})
	toolProps.Set("stdin", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "Standard input for the command (multi-line content)",
	})
	params := api.ToolFunctionParameters{
		Type:       "object",
		Properties: toolProps,
		Required:   []string{"command"},
	}

	return api.Tools{{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "run",
			Description: desc,
			Parameters:  params,
		},
	}}, nil
}