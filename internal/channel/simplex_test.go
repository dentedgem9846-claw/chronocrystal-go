package channel

import (
	"testing"
	"time"

	"github.com/chronocrystal/chronocrystal-go/internal/config"
)

func TestNewSimplex(t *testing.T) {
	cfg := config.ChannelConfig{
		SimplexPath:    "simplex-chat",
		DBPath:         "test.db",
		AutoAccept:     true,
		MaxRetries:     20,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
		BackoffFactor:  2.0,
	}
	s := NewSimplex(cfg)
	if s == nil {
		t.Fatal("expected non-nil Simplex")
	}
	if s.cfg.SimplexPath != "simplex-chat" {
		t.Errorf("expected simplex-chat, got %q", s.cfg.SimplexPath)
	}
	if cap(s.events) != 128 {
		t.Errorf("expected events channel cap 128, got %d", cap(s.events))
	}
	if cap(s.errCh) != 16 {
		t.Errorf("expected error channel cap 16, got %d", cap(s.errCh))
	}
}

func TestEventsChannel(t *testing.T) {
	s := NewSimplex(config.ChannelConfig{})
	ch := s.Events()
	if ch == nil {
		t.Fatal("expected non-nil events channel")
	}
	// Verify it's a receive-only channel — reading should not block
	// (channel is empty and open, but no data to read)
	select {
	case <-ch:
		t.Fatal("expected events channel to be empty")
	default:
		// correct: channel is empty
	}
}

func TestErrorsChannel(t *testing.T) {
	s := NewSimplex(config.ChannelConfig{})
	ch := s.Errors()
	if ch == nil {
		t.Fatal("expected non-nil errors channel")
	}
	select {
	case <-ch:
		t.Fatal("expected errors channel to be empty")
	default:
	}
}

func TestSendRawNotRunning(t *testing.T) {
	s := NewSimplex(config.ChannelConfig{})
	err := s.SendRaw("/_test command")
	if err == nil {
		t.Fatal("expected error when subprocess not running")
	}
	if err.Error() != "simplex subprocess not running" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestShutdownNotStarted(t *testing.T) {
	s := NewSimplex(config.ChannelConfig{})
	err := s.Shutdown()
	if err != nil {
		t.Fatalf("unexpected error on shutdown with no process: %v", err)
	}
	// Channels should be closed after shutdown
	_, ok := <-s.Events()
	if ok {
		t.Error("expected events channel to be closed")
	}
	_, ok = <-s.Errors()
	if ok {
		t.Error("expected errors channel to be closed")
	}
}

func TestReconnectConstants(t *testing.T) {
	cfg := config.ChannelConfig{
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
		BackoffFactor:  2.0,
		MaxRetries:     20,
	}
	s := NewSimplex(cfg)
	if s.cfg.InitialBackoff != 1*time.Second {
		t.Errorf("expected InitialBackoff 1s, got %v", s.cfg.InitialBackoff)
	}
	if s.cfg.MaxBackoff != 30*time.Second {
		t.Errorf("expected MaxBackoff 30s, got %v", s.cfg.MaxBackoff)
	}
	if s.cfg.BackoffFactor != 2.0 {
		t.Errorf("expected BackoffFactor 2.0, got %v", s.cfg.BackoffFactor)
	}
	if s.cfg.MaxRetries != 20 {
		t.Errorf("expected MaxRetries 20, got %d", s.cfg.MaxRetries)
	}
}