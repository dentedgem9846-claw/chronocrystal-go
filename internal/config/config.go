package config

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Agent    AgentConfig    `toml:"agent"`
	Provider ProviderConfig  `toml:"provider"`
	Channel  ChannelConfig  `toml:"channel"`
	Memory   MemoryConfig   `toml:"memory"`
	Logging  LoggingConfig  `toml:"logging"`
	Tools    ToolsConfig    `toml:"tools"`
}

type AgentConfig struct {
	Model              string `toml:"model"`
	MaxToolIterations  int    `toml:"max_tool_iterations"`
	ToolTimeout        int    `toml:"tool_timeout"`
	ContextWindow      int    `toml:"context_window"`
	RecentMessagesKeep int    `toml:"recent_messages_keep"`
	SystemPrompt       string `toml:"system_prompt"`
}

type ProviderConfig struct {
	URL     string        `toml:"url"`
	Timeout time.Duration `toml:"timeout"`
}

type ChannelConfig struct {
	SimplexPath string `toml:"simplex_path"`
	DBPath      string `toml:"db_path"`
	AutoAccept  bool   `toml:"auto_accept"`
}

type MemoryConfig struct {
	DBPath     string `toml:"db_path"`
	AutoCommit bool   `toml:"auto_commit"`
	// Lambda memory parameters
	LambdaDecay    float64 `toml:"lambda_decay"`    // decay rate (default 0.01)
	GoneThreshold  float64 `toml:"gone_threshold"`  // below this score, memory is invisible (default 0.01)
	LambdaBudgetPct float64 `toml:"lambda_budget_pct"` // % of context window for lambda memories (default 0.15)
}

type LoggingConfig struct {
	Level string `toml:"level"`
	File  string `toml:"file"`
}

type ToolsConfig struct {
	Dir        string `toml:"dir"`
	Precompile bool   `toml:"precompile"`
}

func DefaultConfig() *Config {
	return &Config{
		Agent: AgentConfig{
			Model:              "llama3.2",
			MaxToolIterations:  20,
			ToolTimeout:        60,
			ContextWindow:      8192,
			RecentMessagesKeep: 10,
			SystemPrompt:       "",
		},
		Provider: ProviderConfig{
			URL:     "http://localhost:11434",
			Timeout: 120 * time.Second,
		},
		Channel: ChannelConfig{
			SimplexPath: "simplex-chat",
			DBPath:      "simplex.db",
			AutoAccept:  true,
		},
		Memory: MemoryConfig{
			DBPath:         "chronocrystal.db",
			AutoCommit:     true,
			LambdaDecay:    0.01,
			GoneThreshold:  0.01,
			LambdaBudgetPct: 0.15,
		},
		Logging: LoggingConfig{
			Level: "info",
			File:  "",
		},
		Tools: ToolsConfig{
			Dir:        "./tools",
			Precompile: false,
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", path)
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Validate() error {
	if c.Agent.Model == "" {
		return fmt.Errorf("agent.model is required")
	}
	if c.Agent.ContextWindow <= 0 {
		return fmt.Errorf("agent.context_window must be positive")
	}
	if c.Agent.MaxToolIterations <= 0 {
		return fmt.Errorf("agent.max_tool_iterations must be positive")
	}
	if c.Provider.URL == "" {
		return fmt.Errorf("provider.url is required")
	}
	if c.Channel.SimplexPath == "" {
		return fmt.Errorf("channel.simplex_path is required")
	}
	if c.Memory.DBPath == "" {
		return fmt.Errorf("memory.db_path is required")
	}
	if c.Memory.LambdaDecay <= 0 {
		return fmt.Errorf("memory.lambda_decay must be positive")
	}
	if c.Tools.Dir == "" {
		return fmt.Errorf("tools.dir is required")
	}
	return nil
}