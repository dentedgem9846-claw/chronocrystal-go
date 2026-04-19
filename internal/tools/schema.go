package tools

import (
	"encoding/json"
)

// ToolInput is the JSON sent to a tool via stdin.
type ToolInput struct {
	Command string          `json:"command"` // The action the tool should perform
	Params  json.RawMessage `json:"params"`  // Tool-specific parameters
}

// ToolOutput is the JSON returned from a tool via stdout.
type ToolOutput struct {
	Success bool            `json:"success"`
	Result  string          `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// ToolDeclaration describes a tool for the LLM.
type ToolDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema object
}