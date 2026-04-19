package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ToolInput struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
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
			"name":        "file_read",
			"description": "Read file contents with optional line offset and limit",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the file to read",
					},
					"offset": map[string]interface{}{
						"type":        "integer",
						"description": "Line number to start reading from (0-based, default 0)",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of lines to read (0 = all, default 100)",
					},
				},
				"required": []string{"path"},
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

	f, err := os.Open(input.Path)
	if err != nil {
		if os.IsNotExist(err) {
			out := ToolOutput{Success: false, Error: "file not found"}
			json.NewEncoder(os.Stdout).Encode(out)
			return
		}
		out := ToolOutput{Success: false, Error: fmt.Sprintf("cannot open file: %v", err)}
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}
	defer f.Close()

	limit := input.Limit
	if limit == 0 {
		limit = 100
	}

	scanner := bufio.NewScanner(f)
	var lines []string
	lineNum := 0
	count := 0

	for scanner.Scan() {
		if lineNum < input.Offset {
			lineNum++
			continue
		}
		lines = append(lines, scanner.Text())
		count++
		if count >= limit {
			break
		}
		lineNum++
	}

	if err := scanner.Err(); err != nil {
		out := ToolOutput{Success: false, Error: fmt.Sprintf("read error: %v", err)}
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}

	out := ToolOutput{Success: true, Result: strings.Join(lines, "\n")}
	json.NewEncoder(os.Stdout).Encode(out)
}