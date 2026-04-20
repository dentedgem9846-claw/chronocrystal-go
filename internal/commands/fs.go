package commands

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/chronocrystal/chronocrystal-go/internal/presenter"
)

// workspaceDir returns the WORKSPACE_DIR env var, or "." as default.
func workspaceDir() string {
	if dir := os.Getenv("WORKSPACE_DIR"); dir != "" {
		return dir
	}
	return "."
}

// resolvePath resolves a relative path within WORKSPACE_DIR with path traversal protection.
func resolvePath(path string) (string, error) {
	base, err := filepath.Abs(workspaceDir())
	if err != nil {
		return "", fmt.Errorf("resolving workspace dir: %w", err)
	}

	// Join the path with the workspace base.
	joined := filepath.Join(base, path)
	cleaned, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("cleaning path: %w", err)
	}

	// Ensure the resolved path is within the workspace.
	if !strings.HasPrefix(cleaned, base+string(os.PathSeparator)) && cleaned != base {
		return "", fmt.Errorf("path traversal denied: %s escapes workspace", path)
	}
	return cleaned, nil
}

// isBinaryData checks whether data appears to be binary content.
func isBinaryData(data []byte) bool {
	checkLen := len(data)
	if checkLen > 8192 {
		checkLen = 8192
	}
	chunk := data[:checkLen]

	// Null byte check.
	for _, b := range chunk {
		if b == 0x00 {
			return true
		}
	}

	// UTF-8 validity.
	if !utf8.Valid(data) {
		return true
	}

	// Control character ratio: >10% non-space control chars.
	nonSpace := 0
	control := 0
	for _, r := range string(chunk) {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		nonSpace++
		if r >= 0x00 && r <= 0x1F {
			control++
		}
	}
	if nonSpace > 0 && float64(control)/float64(nonSpace) > 0.10 {
		return true
	}

	return false
}

var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true,
	".gif": true, ".webp": true, ".svg": true,
}

func isImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return imageExts[ext]
}

func formatFileSize(n int64) string {
	const kb = 1024
	const mb = kb * 1024
	switch {
	case n >= int64(mb):
		return fmt.Sprintf("%.1fMB", float64(n)/float64(mb))
	case n >= int64(kb):
		return fmt.Sprintf("%.1fKB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// catHandler reads file content. Options: -b for base64 (binary files).
func (r *Registry) catHandler(_ context.Context, args []string, _ string) (string, error) {
	if len(args) == 0 {
		return "usage: cat <path> [-b]", nil
	}

	base64Mode := false
	pathArg := ""
	for _, arg := range args {
		if arg == "-b" {
			base64Mode = true
		} else if pathArg == "" {
			pathArg = arg
		}
	}

	if pathArg == "" {
		return "usage: cat <path> [-b]", nil
	}

	resolved, err := resolvePath(pathArg)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("cat: %w", err)
	}

	if base64Mode {
		return base64.StdEncoding.EncodeToString(data), nil
	}

	if isBinaryData(data) {
		return fmt.Sprintf("[error] binary file (%s). Use: cat -b %s for base64 or see %s for images",
			formatFileSize(int64(len(data))), pathArg, pathArg), nil
	}

	return string(data), nil
}

// lsHandler lists files in a directory. Default is current directory.
func (r *Registry) lsHandler(_ context.Context, args []string, _ string) (string, error) {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	resolved, err := resolvePath(dir)
	if err != nil {
		return "", err
	}

	entries, err := os.ReadDir(resolved)
	if err != nil {
		return "", fmt.Errorf("ls: %w", err)
	}

	if len(entries) == 0 {
		return "(empty directory)", nil
	}

	// Sort: directories first, then files, both alphabetically.
	sort.Slice(entries, func(i, j int) bool {
		iDir := entries[i].IsDir()
		jDir := entries[j].IsDir()
		if iDir != jDir {
			return iDir // directories first
		}
		return entries[i].Name() < entries[j].Name()
	})

	var b strings.Builder
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			// Skip entries we can't stat.
			continue
		}

		prefix := "f"
		name := entry.Name()
		if entry.IsDir() {
			prefix = "d"
			name += "/"
		}
		b.WriteString(fmt.Sprintf("%s  %s  %s\n", prefix, formatFileSize(info.Size()), name))
	}

	result := strings.TrimRight(b.String(), "\n")
	return result, nil
}

// writeHandler writes content to a file. Content from args or stdin.
func (r *Registry) writeHandler(_ context.Context, args []string, stdin string) (string, error) {
	if len(args) == 0 {
		return "usage: write <path> [content]", nil
	}

	pathArg := args[0]
	content := ""
	if len(args) > 1 {
		content = strings.Join(args[1:], " ")
	} else if stdin != "" {
		content = stdin
	}

	resolved, err := resolvePath(pathArg)
	if err != nil {
		return "", err
	}

	// Create parent directories automatically.
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return "", fmt.Errorf("write: creating parent dirs: %w", err)
	}

	if err := os.WriteFile(resolved, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}

	return fmt.Sprintf("wrote %d bytes to %s", len(content), pathArg), nil
}

// seeHandler views an image file. Returns metadata.
func (r *Registry) seeHandler(_ context.Context, args []string, _ string) (string, error) {
	if len(args) == 0 {
		return "usage: see <image-path>", nil
	}

	pathArg := args[0]
	resolved, err := resolvePath(pathArg)
	if err != nil {
		return "", err
	}

	if !isImageFile(pathArg) {
		return fmt.Sprintf("[error] not an image file: %s. Use cat to read text files", pathArg), nil
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("see: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(pathArg))
	result := presenter.Present(
		nil, "", 0, 0, pathArg,
		r.presenter,
	)
	// Override for image: provide metadata.
	_ = result // unused; we build our own output for images
	return fmt.Sprintf("image: %s\nsize: %s\nformat: %s", pathArg, formatFileSize(info.Size()), ext[1:]), nil
}

// grepHandler filters lines matching a pattern. Options: -i (case-insensitive), -v (invert), -c (count).
func (r *Registry) grepHandler(_ context.Context, args []string, stdin string) (string, error) {
	if len(args) == 0 {
		return "usage: grep <pattern> [-i] [-v] [-c] [file]", nil
	}

	caseInsensitive := false
	invert := false
	countMode := false
	pattern := ""
	fileArg := ""

	// Parse flags and positional args. Flags can appear anywhere.
	var positionals []string
	for _, arg := range args {
		switch arg {
		case "-i":
			caseInsensitive = true
		case "-v":
			invert = true
		case "-c":
			countMode = true
		default:
			positionals = append(positionals, arg)
		}
	}

	if len(positionals) > 0 {
		pattern = positionals[0]
	}
	if len(positionals) > 1 {
		fileArg = positionals[1]
	}

	if pattern == "" {
		return "usage: grep <pattern> [-i] [-v] [-c] [file]", nil
	}

	var lines []string
	if fileArg != "" {
		resolved, err := resolvePath(fileArg)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return "", fmt.Errorf("grep: %w", err)
		}
		lines = strings.Split(string(data), "\n")
	} else if stdin != "" {
		lines = strings.Split(stdin, "\n")
	} else {
		return "usage: grep <pattern> [-i] [-v] [-c] [file]", nil
	}

	var matched []string
	for _, line := range lines {
		match := strings.Contains(line, pattern)
		if caseInsensitive {
			match = strings.Contains(strings.ToLower(line), strings.ToLower(pattern))
		}
		if invert {
			match = !match
		}
		if match {
			matched = append(matched, line)
		}
	}

	if countMode {
		return fmt.Sprintf("%d", len(matched)), nil
	}

	return strings.Join(matched, "\n"), nil
}

// statHandler returns file metadata.
func (r *Registry) statHandler(_ context.Context, args []string, _ string) (string, error) {
	if len(args) == 0 {
		return "usage: stat <path>", nil
	}

	pathArg := args[0]
	resolved, err := resolvePath(pathArg)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat: %w", err)
	}

	mode := info.Mode().String()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("  path: %s\n", pathArg))
	b.WriteString(fmt.Sprintf("  size: %s\n", formatFileSize(info.Size())))
	b.WriteString(fmt.Sprintf("  mode: %s\n", mode))
	if info.IsDir() {
		b.WriteString("  type: directory\n")
	} else {
		b.WriteString("  type: file\n")
	}
	b.WriteString(fmt.Sprintf("  modified: %s\n", info.ModTime().Format("2006-01-02 15:04:05")))

	return strings.TrimRight(b.String(), "\n"), nil
}

// fileInfo is a minimal struct for test assertions.
type fileInfo struct {
	DirEntry fs.DirEntry
}

// formatDirEntry formats a directory entry for ls output.
func formatDirEntry(entry fs.DirEntry) string {
	info, err := entry.Info()
	if err != nil {
		return fmt.Sprintf("?  ?  %s", entry.Name())
	}
	prefix := "f"
	name := entry.Name()
	if entry.IsDir() {
		prefix = "d"
		name += "/"
	}
	return fmt.Sprintf("%s  %s  %s", prefix, formatFileSize(info.Size()), name)
}