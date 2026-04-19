package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type ToolInput struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
}

type ToolOutput struct {
	Success bool        `json:"success"`
	Result  string      `json:"result"`
	Error   string      `json:"error,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

type FileEntry struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir"`
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
			"name":        "file_list",
			"description": "List directory contents, optionally recursively",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the directory to list",
					},
					"recursive": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to walk subdirectories (default false)",
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

	info, err := os.Stat(input.Path)
	if err != nil {
		if os.IsNotExist(err) {
			out := ToolOutput{Success: false, Error: "directory not found"}
			json.NewEncoder(os.Stdout).Encode(out)
			return
		}
		out := ToolOutput{Success: false, Error: fmt.Sprintf("cannot stat path: %v", err)}
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}

	if !info.IsDir() {
		out := ToolOutput{Success: false, Error: "path is not a directory"}
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}

	var entries []FileEntry

	if input.Recursive {
		err = filepath.WalkDir(input.Path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path == input.Path {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(input.Path, path)
			entries = append(entries, FileEntry{
				Name:  rel,
				Size:  info.Size(),
				IsDir: d.IsDir(),
			})
			return nil
		})
	} else {
		dirEntries, err2 := os.ReadDir(input.Path)
		if err2 != nil {
			out := ToolOutput{Success: false, Error: fmt.Sprintf("cannot read directory: %v", err2)}
			json.NewEncoder(os.Stdout).Encode(out)
			return
		}
		for _, d := range dirEntries {
			info, err := d.Info()
			if err != nil {
				continue
			}
			entries = append(entries, FileEntry{
				Name:  d.Name(),
				Size:  info.Size(),
				IsDir: d.IsDir(),
			})
		}
	}

	if err != nil {
		out := ToolOutput{Success: false, Error: fmt.Sprintf("walk error: %v", err)}
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}

	var names []string
	for _, e := range entries {
		prefix := ""
		if e.IsDir {
			prefix = "[DIR] "
		}
		names = append(names, prefix+e.Name)
	}

	out := ToolOutput{
		Success: true,
		Result:  strings.Join(names, "\n"),
		Data:    entries,
	}
	json.NewEncoder(os.Stdout).Encode(out)
}