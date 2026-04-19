package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiscoverEmptyDir(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(dir)
	decls, err := reg.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover on empty dir: %v", err)
	}
	if len(decls) != 0 {
		t.Errorf("expected 0 declarations from empty dir, got %d", len(decls))
	}
}

func TestDiscoverNonexistentDir(t *testing.T) {
	reg := NewRegistry("/nonexistent/tools/dir")
	decls, err := reg.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover on nonexistent dir should not error: %v", err)
	}
	if len(decls) != 0 {
		t.Errorf("expected 0 declarations, got %d", len(decls))
	}
}

func TestDiscoverDirWithoutGoFiles(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "empty_tool")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	reg := NewRegistry(dir)
	decls, err := reg.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(decls) != 0 {
		t.Errorf("expected 0 declarations, got %d", len(decls))
	}
}

func TestDiscoverCaches(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(dir)

	decls1, err := reg.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	decls2, err := reg.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover (cached): %v", err)
	}
	if len(decls1) != len(decls2) {
		t.Errorf("cache mismatch: first=%d, second=%d", len(decls1), len(decls2))
	}
}

func TestGet(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(dir)

	_, err := reg.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	_, ok := reg.Get("nonexistent")
	if ok {
		t.Error("Get(\"nonexistent\") should return false")
	}
}

func TestDeclarationsReturnsEmptyInitially(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(dir)

	decls := reg.Declarations()
	if decls != nil {
		t.Errorf("expected nil declarations before Discover, got %v", decls)
	}
}

// createTestTool sets up a temporary Go module with a discoverable tool subdirectory.
// The tool is part of the parent module so `go run ./tools/<name>` works.
func createTestTool(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	// Parent go.mod so `go run ./tools/<name>` works.
	goMod := "module test_root\n\ngo 1.26.1\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("WriteFile go.mod: %v", err)
	}

	toolsDir := filepath.Join(root, "tools", "echo_tool")
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
			"name":        "echo_tool",
			"description": "A test echo tool",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "The command to execute",
					},
				},
				"required": []string{"command"},
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

	return root
}

func TestDiscoverWithToolSource(t *testing.T) {
	root := createTestTool(t)
	t.Chdir(root)

	reg := NewRegistry(filepath.Join(root, "tools"))
	decls, err := reg.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(decls) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(decls))
	}
	if decls[0].Name != "echo_tool" {
		t.Errorf("declaration name = %q, want %q", decls[0].Name, "echo_tool")
	}
	if decls[0].Description != "A test echo tool" {
		t.Errorf("declaration description = %q, want %q", decls[0].Description, "A test echo tool")
	}

	// Verify Get works.
	d, ok := reg.Get("echo_tool")
	if !ok {
		t.Error("Get(\"echo_tool\") not found")
	}
	if d.Name != "echo_tool" {
		t.Errorf("Get name = %q, want %q", d.Name, "echo_tool")
	}
}

func TestToolDeclarationJSON(t *testing.T) {
	root := createTestTool(t)
	t.Chdir(root)

	reg := NewRegistry(filepath.Join(root, "tools"))
	decls, err := reg.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	for _, d := range decls {
		var params map[string]interface{}
		if err := json.Unmarshal(d.Parameters, &params); err != nil {
			t.Errorf("tool %q: invalid parameters JSON: %v", d.Name, err)
		}
		if typ, ok := params["type"]; ok {
			if typ != "object" {
				t.Errorf("tool %q: parameters type = %q, want %q", d.Name, typ, "object")
			}
		}
	}
}

func TestDiscoverSkipsInvalidTool(t *testing.T) {
	root := t.TempDir()
	goMod := "module test_root\n\ngo 1.26.1\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("WriteFile go.mod: %v", err)
	}

	toolsDir := filepath.Join(root, "tools", "broken_tool")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	mainGo := `package main

func main() {
	// This tool doesn't support --describe
	_ = 1 / 0 // intentional compile error: div by zero is actually fine but let's not produce valid --describe output
}
`
	if err := os.WriteFile(filepath.Join(toolsDir, "main.go"), []byte(mainGo), 0644); err != nil {
		t.Fatalf("WriteFile main.go: %v", err)
	}

	t.Chdir(root)

	reg := NewRegistry(filepath.Join(root, "tools"))
	decls, err := reg.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	// Broken tool should be skipped, not cause an error.
	if len(decls) != 0 {
		t.Errorf("expected 0 declarations from broken tool, got %d", len(decls))
	}
}

func TestGoRunnerDescribe(t *testing.T) {
	root := createTestTool(t)
	t.Chdir(root)

	runner := NewGoRunner(30 * time.Second)
	decl, err := runner.RunDescribe(context.Background(), "echo_tool")
	if err != nil {
		t.Fatalf("RunDescribe: %v", err)
	}
	if decl.Name != "echo_tool" {
		t.Errorf("name = %q, want %q", decl.Name, "echo_tool")
	}
	if decl.Description == "" {
		t.Error("description is empty")
	}
}

func TestGoRunnerRunTool(t *testing.T) {
	root := t.TempDir()

	goMod := "module test_root\n\ngo 1.26.1\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("WriteFile go.mod: %v", err)
	}

	toolsDir := filepath.Join(root, "tools", "echo_tool")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	mainGo := `package main

import (
	"encoding/json"
	"os"
)

type ToolInput struct {
	Command string ` + "`json:\"command\"`" + `
}

type ToolOutput struct {
	Success bool   ` + "`json:\"success\"`" + `
	Result  string ` + "`json:\"result\"`" + `
}

func main() {
	var input ToolInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		out := ToolOutput{Success: false, Result: err.Error()}
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}
	out := ToolOutput{Success: true, Result: input.Command}
	json.NewEncoder(os.Stdout).Encode(out)
}
`
	if err := os.WriteFile(filepath.Join(toolsDir, "main.go"), []byte(mainGo), 0644); err != nil {
		t.Fatalf("WriteFile main.go: %v", err)
	}

	t.Chdir(root)

	runner := NewGoRunner(30 * time.Second)
	input := ToolInput{
		Command: "echo hello",
		Params:  json.RawMessage(`{}`),
	}

	output, err := runner.Run(context.Background(), "echo_tool", input)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !output.Success {
		t.Errorf("tool failed: error=%s", output.Error)
	}
	if output.Result != "echo hello" {
		t.Errorf("result = %q, want %q", output.Result, "echo hello")
	}
}

func TestGoRunnerTimeout(t *testing.T) {
	root := t.TempDir()

	goMod := "module test_root\n\ngo 1.26.1\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("WriteFile go.mod: %v", err)
	}

	toolsDir := filepath.Join(root, "tools", "sleep_tool")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	mainGo := `package main

import "time"

func main() {
	time.Sleep(10 * time.Minute)
`
	if err := os.WriteFile(filepath.Join(toolsDir, "main.go"), []byte(mainGo), 0644); err != nil {
		t.Fatalf("WriteFile main.go: %v", err)
	}

	t.Chdir(root)

	runner := NewGoRunner(2 * time.Second)
	input := ToolInput{Command: "sleep", Params: json.RawMessage(`{}`)}

	_, err := runner.Run(context.Background(), "sleep_tool", input)
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}

func TestGoRunnerInvalidTool(t *testing.T) {
	root := t.TempDir()

	goMod := "module test_root\n\ngo 1.26.1\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("WriteFile go.mod: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(root, "tools"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	t.Chdir(root)

	runner := NewGoRunner(5 * time.Second)
	input := ToolInput{Command: "nonexistent", Params: json.RawMessage(`{}`)}

	_, err := runner.Run(context.Background(), "nonexistent_tool", input)
	if err == nil {
		t.Error("expected error for nonexistent tool, got nil")
	}
}