package channel

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/chronocrystal/chronocrystal-go/internal/config"
)

const (
	quitTimeout = 5 * time.Second
)

// Simplex manages a simplex-chat subprocess.
type Simplex struct {
	cfg    config.ChannelConfig
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	events chan Event
	errCh  chan error

	mu     sync.Mutex
	cancel context.CancelFunc
	userID int64
}

// NewSimplex creates a new SimpleX subprocess manager.
func NewSimplex(cfg config.ChannelConfig) *Simplex {
	return &Simplex{
		cfg:    cfg,
		events: make(chan Event, 128),
		errCh:  make(chan error, 16),
	}
}

// Events returns the channel of parsed SimpleX events.
func (s *Simplex) Events() <-chan Event {
	return s.events
}

// Errors returns the channel of operational errors.
func (s *Simplex) Errors() <-chan error {
	return s.errCh
}

// Start launches the simplex-chat subprocess and begins reading events.
// It also starts the reconnect loop so the subprocess is restarted on crash.
func (s *Simplex) Start(ctx context.Context) error {
	ctx, s.cancel = context.WithCancel(ctx)

	if err := s.startProcess(ctx); err != nil {
		return fmt.Errorf("initial subprocess start: %w", err)
	}

	go s.reconnectLoop(ctx)
	return nil
}

// startProcess launches the subprocess and wires up stdout/stderr readers.
func (s *Simplex) startProcess(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cmd := exec.CommandContext(ctx, s.cfg.SimplexPath, "--db", s.cfg.DBPath)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}

	s.cmd = cmd
	s.stdin = stdinPipe

	go s.readStdout(ctx, bufio.NewScanner(stdoutPipe))
	go s.readStderr(stderrPipe)

	return nil
}

// readStdout reads lines from the subprocess stdout, parses them, and sends events.
func (s *Simplex) readStdout(ctx context.Context, scanner *bufio.Scanner) {
	for scanner.Scan() {
		line := scanner.Text()
		parsed, err := ParseResponse(line)
		if err != nil {
			select {
			case s.errCh <- fmt.Errorf("parse response: %w", err):
			default:
			}
			continue
		}

		if parsed == nil {
			continue
		}

		s.discoverUserID(parsed)

		evt, ok := s.toEvent(parsed)
		if !ok {
			continue
		}

		select {
		case s.events <- evt:
		case <-ctx.Done():
			return
		}
	}

	if err := scanner.Err(); err != nil {
		select {
		case s.errCh <- fmt.Errorf("stdout read: %w", err):
		default:
		}
	}
}

// readStderr logs subprocess stderr output.
func (s *Simplex) readStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		log.Printf("[simplex-stderr] %s", scanner.Text())
	}
}

// discoverUserID extracts the userID from the first response that carries it.
func (s *Simplex) discoverUserID(parsed interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.userID != 0 {
		return
	}

	type hasUser struct {
		User User `json:"user"`
	}

	var wrapper hasUser
	raw, err := json.Marshal(parsed)
	if err != nil {
		return
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return
	}
	if wrapper.User.UserID != 0 {
		s.userID = wrapper.User.UserID
	}
}

// toEvent converts a parsed response into an Event.
func (s *Simplex) toEvent(parsed interface{}) (Event, bool) {
	switch v := parsed.(type) {
	case NewChatItemsResponse:
		evt := Event{Type: EventNewChatItems}
		for _, raw := range v.ChatItems {
			evt.ChatItems = append(evt.ChatItems, ChatItem{
				ChatInfo: ChatInfo{
					ContactID:   raw.ChatInfo.ContactID,
					DisplayName: raw.ChatInfo.DisplayName,
				},
				Content: MsgContent{Text: raw.ChatItem.Content.Text},
			})
		}
		return evt, true
	case ContactConnectedResponse:
		return Event{
			Type:    EventContactConnected,
			Contact: v.Contact,
		}, true
	case ContactSndReadyResponse:
		return Event{
			Type:    EventContactSndReady,
			Contact: v.Contact,
		}, true
	default:
		return Event{}, false
	}
}

// autoAcceptSetup sends address creation and auto-accept commands after userID is known.
func (s *Simplex) autoAcceptSetup() {
	s.mu.Lock()
	uid := s.userID
	s.mu.Unlock()

	if uid == 0 {
		return
	}

	if err := s.SendRaw(BuildCreateAddressCommand(uid)); err != nil {
		select {
		case s.errCh <- fmt.Errorf("auto-accept create address: %w", err):
		default:
		}
		return
	}

	if err := s.SendRaw(BuildSetAddressSettingsCommand(uid, s.cfg.AutoAccept)); err != nil {
		select {
		case s.errCh <- fmt.Errorf("auto-accept set settings: %w", err):
		default:
		}
	}

	log.Printf("[simplex] auto-accept configured for user %d", uid)
}

// Send constructs and sends a message to the given chat reference.
func (s *Simplex) Send(chatRef ChatRef, text string) error {
	return s.SendRaw(BuildSendCommand(chatRef, text))
}

// SendRaw writes a raw command string to the subprocess stdin.
func (s *Simplex) SendRaw(cmd string) error {
	s.mu.Lock()
	stdin := s.stdin
	s.mu.Unlock()

	if stdin == nil {
		return fmt.Errorf("simplex subprocess not running")
	}

	if _, err := fmt.Fprintf(stdin, "%s\n", cmd); err != nil {
		return fmt.Errorf("write command: %w", err)
	}
	return nil
}

// Shutdown sends /quit, waits for the process to exit, then kills if needed.
func (s *Simplex) Shutdown() error {
	if s.cancel != nil {
		s.cancel()
	}

	s.mu.Lock()
	cmd := s.cmd
	stdin := s.stdin
	s.stdin = nil
	s.mu.Unlock()

	if stdin != nil {
		// Best-effort quit; process may already be dead.
		_, _ = fmt.Fprintf(stdin, "/quit\n")
	}

	if cmd == nil || cmd.Process == nil {
		close(s.events)
		close(s.errCh)
		return nil
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		// Process exited cleanly.
	case <-time.After(quitTimeout):
		// Force kill.
		_ = cmd.Process.Kill()
		<-done
	}

	close(s.events)
	close(s.errCh)
	return nil
}

// reconnectLoop monitors the subprocess and restarts it with exponential backoff.
func (s *Simplex) reconnectLoop(ctx context.Context) {
	backoff := s.cfg.InitialBackoff
	retries := 0
	userIDKnown := false

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Wait for the current process to exit.
		s.mu.Lock()
		cmd := s.cmd
		s.mu.Unlock()

		if cmd == nil {
			return
		}

		err := cmd.Wait()
		if err != nil {
			log.Printf("[simplex] subprocess exited: %v", err)
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		// Process is dead; nil out state before restart.
		s.mu.Lock()
		s.stdin = nil
		s.cmd = nil
		s.mu.Unlock()

		log.Printf("[simplex] reconnecting in %v", backoff)

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}

		if err := s.startProcess(ctx); err != nil {
			retries++
			if retries >= s.cfg.MaxRetries {
				log.Printf("[simplex] max retries (%d) exceeded, giving up", s.cfg.MaxRetries)
				close(s.events)
				close(s.errCh)
				return
			}
			log.Printf("[simplex] reconnect failed: %v", err)
			select {
			case s.errCh <- fmt.Errorf("reconnect: %w", err):
			default:
			}

			backoff = time.Duration(float64(backoff) * s.cfg.BackoffFactor)
			if backoff > s.cfg.MaxBackoff {
				backoff = s.cfg.MaxBackoff
			}
			continue
		}

		// Successful reconnect resets backoff and retry count.
		backoff = s.cfg.InitialBackoff
		retries = 0

		// Re-apply auto-accept if it was configured and we know the userID.
		s.mu.Lock()
		uid := s.userID
		s.mu.Unlock()

		if s.cfg.AutoAccept && uid != 0 && !userIDKnown {
			userIDKnown = true
			s.autoAcceptSetup()
		} else if s.cfg.AutoAccept && uid != 0 {
			s.autoAcceptSetup()
		}
	}
}