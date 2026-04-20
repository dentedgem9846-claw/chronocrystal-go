package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/chronocrystal/chronocrystal-go/internal/config"
	"github.com/ollama/ollama/api"
)

// newTestProvider creates a Provider pointing at the given httptest.Server.
func newTestProvider(srv *httptest.Server) (*Provider, error) {
	u, _ := url.Parse(srv.URL)
	client := api.NewClient(u, srv.Client())

	p := &Provider{
		client:       client,
		cfg:          config.ProviderConfig{URL: srv.URL},
		model:        "llama3.2",
		circuitState: CircuitClosed,
		threshold:    3,
		cooldown:     30 * time.Second,
	}
	return p, nil
}

func TestNewProviderValidURL(t *testing.T) {
	p, err := NewProvider(
		config.AgentConfig{Model: "llama3.2"},
		config.ProviderConfig{URL: "http://localhost:11434"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if p.circuitState != CircuitClosed {
		t.Errorf("expected CircuitClosed, got %d", p.circuitState)
	}
}

func TestNewProviderInvalidURL(t *testing.T) {
	_, err := NewProvider(
		config.AgentConfig{Model: "llama3.2"},
		config.ProviderConfig{URL: "://bad"},
	)
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

func TestTokenCount(t *testing.T) {
	p, _ := NewProvider(
		config.AgentConfig{Model: "llama3.2"},
		config.ProviderConfig{URL: "http://localhost:11434"},
	)
	got := p.TokenCount("hello world")
	// len("hello world") = 11, 11/4 = 2
	if got != 2 {
		t.Errorf("expected 2, got %d", got)
	}
}

func TestTokenCountEmpty(t *testing.T) {
	p, _ := NewProvider(
		config.AgentConfig{Model: "llama3.2"},
		config.ProviderConfig{URL: "http://localhost:11434"},
	)
	if got := p.TokenCount(""); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestCircuitBreakerClosedByDefault(t *testing.T) {
	p, _ := NewProvider(
		config.AgentConfig{Model: "llama3.2"},
		config.ProviderConfig{URL: "http://localhost:11434", CircuitThreshold: 3, CircuitCooldown: 30 * time.Second},
	)
	if p.circuitState != CircuitClosed {
		t.Errorf("expected CircuitClosed (%d), got %d", CircuitClosed, p.circuitState)
	}
}

func TestCircuitBreakerOpensAfterThreshold(t *testing.T) {
	// Server returns 500 with an error body so the ollama client reports failure.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}` + "\n"))
	}))
	defer srv.Close()

	p, _ := newTestProvider(srv)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Call Chat enough times to trip the breaker (threshold = 3).
	for i := 0; i < p.threshold; i++ {
		_, _ = p.Chat(ctx, nil, nil)
	}

	// Verify circuit is now Open.
	p.circuitMu.Lock()
	state := p.circuitState
	p.circuitMu.Unlock()
	if state != CircuitOpen {
		t.Fatalf("expected CircuitOpen, got %d", state)
	}

	// Next call should be blocked by the circuit breaker (Open state, cooldown not elapsed).
	_, err := p.Chat(ctx, nil, nil)
	if err == nil {
		t.Fatal("expected circuit breaker error after threshold failures")
	}
}

func TestCircuitBreakerHalfOpenAfterCooldown(t *testing.T) {
	p, _ := NewProvider(
		config.AgentConfig{Model: "llama3.2"},
		config.ProviderConfig{URL: "http://localhost:11434", CircuitThreshold: 3, CircuitCooldown: 30 * time.Second},
	)

	// Force the circuit open with a lastFailure in the past (cooldown elapsed).
	p.circuitMu.Lock()
	p.failureCount = p.threshold
	p.circuitState = CircuitOpen
	p.lastFailure = time.Now().Add(-p.cooldown - time.Second)
	p.circuitMu.Unlock()

	// circuitBreakerCheck should transition to HalfOpen and allow the request.
	err := p.circuitBreakerCheck()
	if err != nil {
		t.Fatalf("expected no error after cooldown (HalfOpen), got: %v", err)
	}

	p.circuitMu.Lock()
	state := p.circuitState
	p.circuitMu.Unlock()
	if state != CircuitHalfOpen {
		t.Errorf("expected CircuitHalfOpen, got %d", state)
	}
}

func TestCircuitBreakerOpenBlocksRequests(t *testing.T) {
	p, _ := NewProvider(
		config.AgentConfig{Model: "llama3.2"},
		config.ProviderConfig{URL: "http://localhost:11434", CircuitThreshold: 3, CircuitCooldown: 30 * time.Second},
	)

	// Force the circuit open with a recent failure time (cooldown not elapsed).
	p.circuitMu.Lock()
	p.circuitState = CircuitOpen
	p.lastFailure = time.Now()
	p.circuitMu.Unlock()

	err := p.circuitBreakerCheck()
	if err == nil {
		t.Fatal("expected circuit breaker to block request when Open and cooldown not elapsed")
	}
}

func TestCircuitBreakerRecoverOnSuccess(t *testing.T) {
	p, _ := NewProvider(
		config.AgentConfig{Model: "llama3.2"},
		config.ProviderConfig{URL: "http://localhost:11434", CircuitThreshold: 3, CircuitCooldown: 30 * time.Second},
	)

	// Put the circuit in HalfOpen state.
	p.circuitMu.Lock()
	p.circuitState = CircuitHalfOpen
	p.failureCount = p.threshold
	p.circuitMu.Unlock()

	// Simulate a success.
	p.recordSuccess()

	p.circuitMu.Lock()
	state := p.circuitState
	failures := p.failureCount
	p.circuitMu.Unlock()

	if state != CircuitClosed {
		t.Errorf("expected CircuitClosed after success, got %d", state)
	}
	if failures != 0 {
		t.Errorf("expected failureCount 0 after success, got %d", failures)
	}
}

func TestClassifyDefaultOnProviderError(t *testing.T) {
	// Server returns 500 with error body — Classify should record a failure.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}` + "\n"))
	}))
	defer srv.Close()

	p, _ := newTestProvider(srv)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := p.Classify(ctx, "hello")
	if err == nil {
		t.Fatal("expected error from Classify with 500 server")
	}

	// After a single Classify call, failureCount should be 1.
	p.circuitMu.Lock()
	fc := p.failureCount
	p.circuitMu.Unlock()
	if fc != 1 {
		t.Errorf("expected failureCount 1, got %d", fc)
	}
}