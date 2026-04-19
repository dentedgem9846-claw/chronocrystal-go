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

const extractBlueprintPrompt = `You are a procedure extraction engine. A task was completed successfully using multiple tool calls. Create a reusable blueprint from this execution.

Respond with ONLY a JSON object (no markdown, no explanation):
{
  "name": "short kebab-case name for this type of task",
  "description": "one-sentence description of what this procedure accomplishes",
  "steps": [
    {
      "tool": "tool name used",
      "input": "brief description of what was passed to the tool",
      "output": "brief description of what the tool returned",
      "success": true
    }
  ]
}`

// ExtractBlueprint analyzes a completed tool-loop conversation and asks the LLM
// to produce a Blueprint. It stores the result via memory.StoreBlueprint.
func ExtractBlueprint(ctx context.Context, p *provider.Provider, conversationID string, store *memory.Store, toolCalls []toolCallRecord) error {
	if len(toolCalls) < 2 {
		return nil
	}

	// Build a summary of the tool calls for the LLM.
	var sb strings.Builder
	for i, tc := range toolCalls {
		sb.WriteString(fmt.Sprintf("Step %d: tool=%q input=%q output=%q success=%v\n",
			i+1, tc.Tool, tc.Input, tc.Output, tc.Success))
	}

	messages := []api.Message{
		{Role: "system", Content: extractBlueprintPrompt},
		{Role: "user", Content: sb.String()},
	}

	resp, err := p.Chat(ctx, messages, nil)
	if err != nil {
		return fmt.Errorf("extracting blueprint from LLM: %w", err)
	}

	// Parse the LLM response.
	raw := strings.TrimSpace(resp.Content)
	// Strip markdown code fences if present.
	if strings.HasPrefix(raw, "```") {
		start := strings.Index(raw, "\n")
		end := strings.LastIndex(raw, "```")
		if start != -1 && end > start {
			raw = raw[start+1 : end]
		}
	}

	var parsed struct {
		Name        string                  `json:"name"`
		Description  string                  `json:"description"`
		Steps        []memory.BlueprintStep `json:"steps"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return fmt.Errorf("parsing blueprint JSON: %w", err)
	}

	if parsed.Name == "" || len(parsed.Steps) == 0 {
		return fmt.Errorf("blueprint extraction returned empty name or steps")
	}

	bp := memory.Blueprint{
		Name:         parsed.Name,
		Description:  parsed.Description,
		Steps:        parsed.Steps,
		FitnessScore: 0.5,
		UseCount:     0,
	}

	_, err = store.StoreBlueprint(bp)
	if err != nil {
		return fmt.Errorf("storing blueprint: %w", err)
	}

	log.Printf("[the mind] blueprint extracted: %s (%d steps)", bp.Name, len(bp.Steps))
	return nil
}

// FindRelevantBlueprints returns blueprints that match the given description
// via keyword matching, ordered by fitness score descending.
func FindRelevantBlueprints(description string, store *memory.Store) ([]memory.Blueprint, error) {
	return store.GetMatchingBlueprints(description, 5)
}

// FormatBlueprintForContext formats a blueprint as a string suitable for
// injection into the system prompt context.
func FormatBlueprintForContext(bp memory.Blueprint) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Blueprint: %s (fitness: %.2f, used %dx)\n", bp.Name, bp.FitnessScore, bp.UseCount))
	sb.WriteString("Steps:\n")
	for i, step := range bp.Steps {
		status := "ok"
		if !step.Success {
			status = "failed"
		}
		sb.WriteString(fmt.Sprintf("%d. %s: %s -> %s [%s]\n", i+1, step.Tool, step.Input, step.Output, status))
	}
	return sb.String()
}

// toolCallRecord captures a single tool invocation for blueprint extraction.
type toolCallRecord struct {
	Tool    string
	Input   string
	Output  string
	Success bool
}