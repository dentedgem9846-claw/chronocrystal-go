package provider

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chronocrystal/chronocrystal-go/internal/config"
	"github.com/ollama/ollama/api"
)

const (
	classifySystemPrompt = "You are a message classifier. Classify the message as: 'chat' (casual conversation), 'order' (command/task request), or 'stop' (request to stop/shutdown). Respond with ONLY one word."
)

type CircuitState int

const (
	CircuitClosed   CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

type Provider struct {
	client       *api.Client
	cfg          config.ProviderConfig
	model        string
	circuitState CircuitState
	circuitMu    sync.Mutex
	failureCount int
lastFailure  time.Time
threshold    int
cooldown     time.Duration
}

func NewProvider(agentCfg config.AgentConfig, providerCfg config.ProviderConfig) (*Provider, error) {
	u, err := url.Parse(providerCfg.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid provider URL %q: %w", providerCfg.URL, err)
	}

	client := api.NewClient(u, nil)

	return &Provider{
		client:       client,
		cfg:          providerCfg,
		model:        agentCfg.Model,
		circuitState: CircuitClosed,
		threshold:    providerCfg.CircuitThreshold,
		cooldown:     providerCfg.CircuitCooldown,
	}, nil
}

// Chat sends a non-streaming chat request with optional tools and returns the
// accumulated assistant message. It applies the circuit breaker before calling
// the Ollama API.
func (p *Provider) Chat(ctx context.Context, messages []api.Message, tools api.Tools) (*api.Message, error) {
	if err := p.circuitBreakerCheck(); err != nil {
		return nil, err
	}

	stream := false
	req := &api.ChatRequest{
		Model:    p.model,
		Messages: messages,
		Tools:    tools,
		Stream:   &stream,
	}

	var accumulated api.Message
	accumulated.Role = "assistant"

	err := p.client.Chat(ctx, req, func(resp api.ChatResponse) error {
		accumulated.Content += resp.Message.Content
		if len(resp.Message.ToolCalls) > 0 {
			accumulated.ToolCalls = append(accumulated.ToolCalls, resp.Message.ToolCalls...)
		}
		return nil
	})
	if err != nil {
		p.recordFailure()
		return nil, fmt.Errorf("ollama chat request failed: %w", err)
	}

	p.recordSuccess()
	return &accumulated, nil
}

// Classify sends a single-message classification request and returns one of
// "chat", "order", or "stop".
func (p *Provider) Classify(ctx context.Context, message string) (string, error) {
	if err := p.circuitBreakerCheck(); err != nil {
		return "", err
	}

	stream := false
	req := &api.ChatRequest{
		Model: p.model,
		Messages: []api.Message{
			{Role: "system", Content: classifySystemPrompt},
			{Role: "user", Content: message},
		},
		Stream: &stream,
	}

	var content string
	err := p.client.Chat(ctx, req, func(resp api.ChatResponse) error {
		content += resp.Message.Content
		return nil
	})
	if err != nil {
		p.recordFailure()
		return "", fmt.Errorf("ollama classify request failed: %w", err)
	}

	p.recordSuccess()

	result := strings.TrimSpace(strings.ToLower(content))
	switch result {
	case "chat", "order", "stop":
		return result, nil
	default:
		// If the model produced something unexpected, default to chat.
		return "chat", nil
	}
}

// TokenCount returns an estimated token count for text.
func (p *Provider) TokenCount(text string) int {
	return len(text) / 4
}

func (p *Provider) circuitBreakerCheck() error {
	p.circuitMu.Lock()
	defer p.circuitMu.Unlock()

	switch p.circuitState {
	case CircuitClosed:
		return nil
	case CircuitOpen:
		if time.Since(p.lastFailure) >= p.cooldown {
			p.circuitState = CircuitHalfOpen
			return nil
		}
		return fmt.Errorf("circuit breaker open; retry after %v", p.cooldown-time.Since(p.lastFailure))
	case CircuitHalfOpen:
		return nil
	default:
		return fmt.Errorf("unknown circuit state: %d", p.circuitState)
	}
}

func (p *Provider) recordSuccess() {
	p.circuitMu.Lock()
	defer p.circuitMu.Unlock()
	p.failureCount = 0
	p.circuitState = CircuitClosed
}

func (p *Provider) recordFailure() {
	p.circuitMu.Lock()
	defer p.circuitMu.Unlock()
	p.failureCount++
	if p.failureCount >= p.threshold {
		p.circuitState = CircuitOpen
		p.lastFailure = time.Now()
	}
}