package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ToolInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type ToolOutput struct {
	Success bool        `json:"success"`
	Result  string      `json:"result"`
	Error   string      `json:"error,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func isPathSafe(p string) bool {
	cleaned := filepath.Clean(p)
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return false
	}
	if strings.Contains(abs, "..") {
		return false
	}
	wkdir := os.Getenv("WORKSPACE_DIR")
	if wkdir != "" {
		wkdirAbs, err := filepath.Abs(filepath.Clean(wkdir))
		if err != nil {
			return false
		}
		if !strings.HasPrefix(abs+"/", wkdirAbs+"/") && abs != wkdirAbs {
			return false
		}
	}
	return true
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--describe" {
		desc := map[string]interface{}{
			"name":        "file_write",
			"description": "Write content to a file, creating parent directories if needed",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the file to write",
					},
					"content": map[string]interface{}{
						"type":        "string",
						"description": "Content to write to the file",
					},
				},
				"required": []string{"path", "content"},
			},
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(desc)
		return
	}

	var input ToolInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		out := ToolOutput{Success: false, Error: fmt.Sprintf("invalid input: %v", err)}
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}

	if input.Path == "" {
		out := ToolOutput{Success: false, Error: "path is required"}
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}

	if !isPathSafe(input.Path) {
		out := ToolOutput{Success: false, Error: "path traversal not allowed"}
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}


	dir := filepath.Dir(input.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		out := ToolOutput{Success: false, Error: fmt.Sprintf("cannot create directory: %v", err)}
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}

	if err := os.WriteFile(input.Path, []byte(input.Content), 0644); err != nil {
		out := ToolOutput{Success: false, Error: fmt.Sprintf("cannot write file: %v", err)}
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}

	out := ToolOutput{
		Success: true,
		Result:  fmt.Sprintf("wrote %d bytes to %s", len(input.Content), input.Path),
	}
	json.NewEncoder(os.Stdout).Encode(out)
}