package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	describeTimeout = 30 * time.Second
	defaultCacheTTL = 5 * time.Minute
)

// Registry discovers and caches tool declarations from the tools directory.
type Registry struct {
	toolsDir     string
	declarations []ToolDeclaration
	cache        map[string]ToolDeclaration
	cacheTime    time.Time
	cacheTTL     time.Duration
}

// NewRegistry creates a tool registry that discovers tools in the given directory.
func NewRegistry(toolsDir string) *Registry {
	return &Registry{
		toolsDir: toolsDir,
		cacheTTL: defaultCacheTTL,
		cache:    make(map[string]ToolDeclaration),
	}
}

// Discover scans the tools directory and runs --describe on each tool subdirectory
// containing main.go. Results are cached for 5 minutes to avoid recompiling.
func (r *Registry) Discover(ctx context.Context) ([]ToolDeclaration, error) {
	if time.Since(r.cacheTime) < r.cacheTTL && r.declarations != nil {
		return r.declarations, nil
	}

	r.declarations = nil
	r.cache = make(map[string]ToolDeclaration)

	entries, err := os.ReadDir(r.toolsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read tools directory %s: %w", r.toolsDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		mainGo := filepath.Join(r.toolsDir, entry.Name(), "main.go")
		if _, err := os.Stat(mainGo); err != nil {
			continue
		}

		decl, err := r.describeTool(ctx, entry.Name())
		if err != nil {
			log.Printf("tools: skipping %q: %v", entry.Name(), err)
			continue
		}

		r.declarations = append(r.declarations, decl)
		r.cache[decl.Name] = decl
	}

	r.cacheTime = time.Now()
	return r.declarations, nil
}

// Declarations returns the currently cached tool declarations.
func (r *Registry) Declarations() []ToolDeclaration {
	return r.declarations
}

// Get looks up a tool declaration by name from the cache.
func (r *Registry) Get(name string) (ToolDeclaration, bool) {
	decl, ok := r.cache[name]
	return decl, ok
}

// describeTool runs `go run <toolPath> --describe` and parses the output.
func (r *Registry) describeTool(ctx context.Context, name string) (ToolDeclaration, error) {
	toolPath := filepath.Join(r.toolsDir, name)

	descCtx, cancel := context.WithTimeout(ctx, describeTimeout)
	defer cancel()

	cmd := exec.CommandContext(descCtx, "go", "run", toolPath, "--describe")
	output, err := cmd.Output()
	if err != nil {
		return ToolDeclaration{}, fmt.Errorf("go run %s --describe: %w", toolPath, err)
	}

	var decl ToolDeclaration
	if err := json.Unmarshal(output, &decl); err != nil {
		return ToolDeclaration{}, fmt.Errorf("parse --describe output from %s: %w", name, err)
	}

	if decl.Name == "" {
		return ToolDeclaration{}, fmt.Errorf("tool %s: declaration missing name", name)
	}

	return decl, nil
}