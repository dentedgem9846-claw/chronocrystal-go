package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/chronocrystal/chronocrystal-go/internal/memory"
	"github.com/chronocrystal/chronocrystal-go/internal/provider"
	"github.com/ollama/ollama/api"
)

const extractLearningPrompt = `Analyze this task interaction and extract a learning. Respond ONLY with valid JSON:

{
  "task_type": "one-word category",
  "approach": "brief description of what was tried",
  "outcome": "success or partial or failure",
  "lesson": "what was learned, one sentence"
}

User order: %s
Tools used: %s
Final response: %s`

// learningResponse is the JSON structure we expect from the LLM.
type learningResponse struct {
	TaskType string `json:"task_type"`
	Approach string `json:"approach"`
	Outcome  string `json:"outcome"`
	Lesson   string `json:"lesson"`
}

// ExtractLearning analyzes a completed task conversation and stores a learning.
// It sends a summary of the interaction to Ollama and parses the structured
// response into a Learning record. Failures are logged but non-fatal.
func ExtractLearning(ctx context.Context, p *provider.Provider, conversationID string, store *memory.Store) error {
	// Gather conversation context for the prompt.
	msgs, err := store.GetMessages(conversationID, 20)
	if err != nil {
		return fmt.Errorf("getting messages for learning extraction: %w", err)
	}

	var userOrder string
	toolCallCount := 0
	var finalResponse string

	for _, m := range msgs {
		switch m.Role {
		case memory.RoleUser:
			if userOrder == "" {
				userOrder = m.Content
			}
		case memory.RoleTool:
			toolCallCount++
		case memory.RoleAssistant:
			if m.Content != "" {
				finalResponse = m.Content
			}
		}
	}

	if userOrder == "" {
		return nil // nothing to learn from
	}

	// Truncate long content to keep the prompt focused.
	if len(userOrder) > 500 {
		userOrder = userOrder[:500]
	}
	if len(finalResponse) > 500 {
		finalResponse = finalResponse[:500]
	}
	toolSummary := "none"
	if toolCallCount > 0 {
		toolSummary = fmt.Sprintf("%d tool calls", toolCallCount)
	}

	prompt := fmt.Sprintf(extractLearningPrompt, userOrder, toolSummary, finalResponse)

	messages := []api.Message{
		{Role: "system", Content: "You extract structured learnings from task interactions. Output only valid JSON."},
		{Role: "user", Content: prompt},
	}

	resp, err := p.Chat(ctx, messages, nil)
	if err != nil {
		log.Printf("[learn] LLM call for learning extraction failed: %v", err)
		return nil // non-fatal
	}

	content := strings.TrimSpace(resp.Content)
	// Strip markdown code fences if present.
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var lr learningResponse
	if err := json.Unmarshal([]byte(content), &lr); err != nil {
		log.Printf("[learn] failed to parse learning JSON: %v, raw: %q", err, content)
		return nil // non-fatal
	}

	// Validate outcome field.
	switch lr.Outcome {
	case "success", "partial", "failure":
	default:
		lr.Outcome = "partial"
	}

	if lr.TaskType == "" || lr.Lesson == "" {
		log.Printf("[learn] incomplete learning extracted: %+v", lr)
		return nil
	}

	learning := memory.Learning{
		TaskType:       lr.TaskType,
		Approach:       lr.Approach,
		Outcome:        lr.Outcome,
		Lesson:         lr.Lesson,
		RelevanceScore: 1.0,
	}

	if err := store.StoreLearning(learning); err != nil {
		return fmt.Errorf("storing learning: %w", err)
	}

	return nil
}

// InjectLearnings retrieves relevant learnings for a given task type and
// returns them as a formatted string suitable for injection into the context.
// Returns an empty string if no learnings are found.
func InjectLearnings(store *memory.Store, taskType string) string {
	if taskType == "" {
		return ""
	}

	learnings, err := store.GetRelevantLearnings(taskType, 3)
	if err != nil {
		return ""
	}

	if len(learnings) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Past learnings for task type '%s':\n", taskType))
	for _, l := range learnings {
		b.WriteString(fmt.Sprintf("- [%s] %s (approach: %s)\n", l.Outcome, l.Lesson, l.Approach))
	}
	return b.String()
}