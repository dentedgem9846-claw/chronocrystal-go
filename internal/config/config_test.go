package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadValidConfig(t *testing.T) {
	// Create a temporary config file.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[agent]
model = "llama3.2"
max_tool_iterations = 20
tool_timeout = 60
context_window = 8192
recent_messages_keep = 10

[provider]
url = "http://localhost:11434"
timeout = "2m0s"

[channel]
simplex_path = "simplex-chat"
db_path = "simplex.db"
auto_accept = true

[memory]
db_path = "chronocrystal.db"
auto_commit = true
lambda_decay = 0.01
gone_threshold = 0.01
lambda_budget_pct = 0.15

[logging]
level = "info"

[tools]
dir = "./tools"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Agent.Model != "llama3.2" {
		t.Errorf("Model = %q, want %q", cfg.Agent.Model, "llama3.2")
	}
	if cfg.Agent.ContextWindow != 8192 {
		t.Errorf("ContextWindow = %d, want 8192", cfg.Agent.ContextWindow)
	}
	if cfg.Provider.URL != "http://localhost:11434" {
		t.Errorf("Provider.URL = %q, want %q", cfg.Provider.URL, "http://localhost:11434")
	}
	if cfg.Provider.Timeout != 120*time.Second {
		t.Errorf("Provider.Timeout = %v, want %v", cfg.Provider.Timeout, 120*time.Second)
	}
	if cfg.Memory.DBPath != "chronocrystal.db" {
		t.Errorf("Memory.DBPath = %q, want %q", cfg.Memory.DBPath, "chronocrystal.db")
	}
	if cfg.Memory.LambdaDecay != 0.01 {
		t.Errorf("Memory.LambdaDecay = %f, want 0.01", cfg.Memory.LambdaDecay)
	}
	if cfg.Tools.Dir != "./tools" {
		t.Errorf("Tools.Dir = %q, want %q", cfg.Tools.Dir, "./tools")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.toml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Agent.Model != "llama3.2" {
		t.Errorf("default Model = %q, want %q", cfg.Agent.Model, "llama3.2")
	}
	if cfg.Agent.ContextWindow != 8192 {
		t.Errorf("default ContextWindow = %d, want 8192", cfg.Agent.ContextWindow)
	}
	if cfg.Agent.MaxToolIterations != 20 {
		t.Errorf("default MaxToolIterations = %d, want 20", cfg.Agent.MaxToolIterations)
	}
	if cfg.Provider.URL != "http://localhost:11434" {
		t.Errorf("default Provider.URL = %q, want %q", cfg.Provider.URL, "http://localhost:11434")
	}
	if cfg.Provider.Timeout != 120*time.Second {
		t.Errorf("default Provider.Timeout = %v, want %v", cfg.Provider.Timeout, 120*time.Second)
	}
	if cfg.Memory.DBPath != "chronocrystal.db" {
		t.Errorf("default Memory.DBPath = %q, want %q", cfg.Memory.DBPath, "chronocrystal.db")
	}
	if cfg.Memory.AutoCommit != true {
		t.Error("default Memory.AutoCommit should be true")
	}
	if cfg.Memory.LambdaDecay != 0.01 {
		t.Errorf("default LambdaDecay = %f, want 0.01", cfg.Memory.LambdaDecay)
	}
	if cfg.Memory.GoneThreshold != 0.01 {
		t.Errorf("default GoneThreshold = %f, want 0.01", cfg.Memory.GoneThreshold)
	}
	if cfg.Tools.Dir != "./tools" {
		t.Errorf("default Tools.Dir = %q, want %q", cfg.Tools.Dir, "./tools")
	}
	if cfg.Channel.AutoAccept != true {
		t.Error("default Channel.AutoAccept should be true")
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("default Logging.Level = %q, want %q", cfg.Logging.Level, "info")
	}
}

func TestValidationMissingModel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agent.Model = ""
	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for empty model")
	}
}

func TestValidationMissingProviderURL(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Provider.URL = ""
	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for empty provider URL")
	}
}

func TestValidationMissingDBPath(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Memory.DBPath = ""
	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for empty db_path")
	}
}

func TestValidationZeroContextWindow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agent.ContextWindow = 0
	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for zero context_window")
	}
}

func TestValidationZeroLambdaDecay(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Memory.LambdaDecay = 0
	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for zero lambda_decay")
	}
}

func TestValidationMissingToolsDir(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Tools.Dir = ""
	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for empty tools dir")
	}
}

func TestValidationMissingSimplexPath(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Channel.SimplexPath = ""
	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for empty simplex_path")
	}
}

func TestValidationNegativeMaxToolIterations(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agent.MaxToolIterations = -1
	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for negative max_tool_iterations")
	}
}

func TestValidConfig(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("default config should be valid: %v", err)
	}
}