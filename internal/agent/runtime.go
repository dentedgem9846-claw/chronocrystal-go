package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/chronocrystal/chronocrystal-go/internal/channel"
	"github.com/chronocrystal/chronocrystal-go/internal/commands"
	"github.com/chronocrystal/chronocrystal-go/internal/config"
	"github.com/chronocrystal/chronocrystal-go/internal/memory"
	"github.com/chronocrystal/chronocrystal-go/internal/provider"
	"github.com/chronocrystal/chronocrystal-go/internal/skills"
	"github.com/ollama/ollama/api"
)

// Runtime is the core agent loop — The Mind of ChronoCrystal.
type Runtime struct {
	provider   *provider.Provider
	memory     *memory.Store
	channel    *channel.Simplex
	commands   *commands.Registry
	skills     *skills.Registry
	config     *config.Config
	ctxBuilder *ContextBuilder

	// Per-contact event channels for concurrent message processing.
	convMu sync.Mutex
	convCh map[string]chan channel.ChatItem
	wg     sync.WaitGroup
}

// NewRuntime assembles the agent from its dependencies.
func NewRuntime(
	cfg *config.Config,
	p *provider.Provider,
	store *memory.Store,
	ch *channel.Simplex,
	cmdReg *commands.Registry,
	skillReg *skills.Registry,
) *Runtime {
	r := &Runtime{
		provider: p,
		memory:   store,
		channel:  ch,
		commands: cmdReg,
		skills:   skillReg,
		config:   cfg,
		convCh:   make(map[string]chan channel.ChatItem),
	}
	r.ctxBuilder = NewContextBuilder(cfg, store, cmdReg, skillReg)
	return r
}

// dispatchItem routes a ChatItem to a per-contact goroutine.
// Each contact gets a dedicated channel that serializes message
// processing, so slow Ollama calls for one contact don't block others.
func (r *Runtime) dispatchItem(ctx context.Context, item channel.ChatItem) {
	contactID := item.ChatInfo.ContactID

	r.convMu.Lock()
	ch, exists := r.convCh[contactID]
	if !exists {
		ch = make(chan channel.ChatItem, 64)
		r.convCh[contactID] = ch
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			r.processContact(ctx, ch)
		}()
	}
	r.convMu.Unlock()

	select {
	case ch <- item:
	case <-ctx.Done():
	}
}

// processContact drains the per-contact channel, processing each item sequentially.
func (r *Runtime) processContact(ctx context.Context, ch chan channel.ChatItem) {
	for item := range ch {
		r.handleMessage(ctx, item.ChatInfo, item.Content.Text)
	}
}

// Start runs the main event loop. It blocks until ctx is cancelled or the channel closes.
func (r *Runtime) Start(ctx context.Context) error {
	events := r.channel.Events()
	errs := r.channel.Errors()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err, ok := <-errs:
			if !ok {
				return nil
			}
			log.Printf("[the mind] channel disturbance: %v", err)
		case evt, ok := <-events:
			if !ok {
				return nil
			}
			if evt.Type == channel.EventNewChatItems {
				for _, item := range evt.ChatItems {
					r.dispatchItem(ctx, item)
				}
			}
		}
	}
}

// handleMessage classifies an incoming message and routes to the appropriate handler.
func (r *Runtime) handleMessage(ctx context.Context, chatInfo channel.ChatInfo, content string) {
	if content == "" {
		return
	}

	classification := Classify(ctx, r.provider, content)

	conv, err := r.findOrCreateConversation(chatInfo)
	if err != nil {
		log.Printf("[the mind] conversation lookup failed: %v", err)
		return
	}

	// Store the user message.
	tokenEst := r.provider.TokenCount(content)
	if _, err := r.memory.StoreMessage(conv.ID, memory.RoleUser, content, 0.5, tokenEst); err != nil {
		log.Printf("[the mind] failed to store user message: %v", err)
	}

	switch classification {
	case ClassChat:
		r.handleChat(ctx, conv.ID, content)
	case ClassOrder:
		r.handleOrder(ctx, conv.ID, content)
	case ClassStop:
		r.handleStop(ctx, conv.ID)
	}
}

// handleOrder builds context, runs the tool loop, and sends the final reply.
func (r *Runtime) handleOrder(ctx context.Context, conversationID, userMessage string) {
	log.Printf("[the mind] order received, awakening the breath")

	messages, err := r.ctxBuilder.Build(ctx, conversationID, userMessage)
	if err != nil {
		log.Printf("[the mind] context build failed: %v", err)
		r.sendReply(conversationID, "The currents of time are disrupted. I could not gather my memories.")
		return
	}

	apiTools, err := r.ctxBuilder.ToolDeclarations(ctx)
	if err != nil {
		log.Printf("[the mind] tool discovery failed: %v", err)
	}

	var records []toolCallRecord

	maxIter := r.config.Agent.MaxToolIterations
	for i := 0; i < maxIter; i++ {
		resp, err := r.provider.Chat(ctx, messages, apiTools)
		if err != nil {
			log.Printf("[the mind] ollama call failed: %v", err)
			r.sendReply(conversationID, "The temporal winds resist. The breath falters.")
			return
		}

		if len(resp.ToolCalls) > 0 {
			// Append the assistant message (with tool_calls) to conversation history.
			messages = append(messages, *resp)

			for _, tc := range resp.ToolCalls {
				log.Printf("[the breath] invoking run")

				// Parse arguments to extract command and stdin.
				var args struct {
					Command string `json:"command"`
					Stdin   string `json:"stdin"`
				}
				argsMap := tc.Function.Arguments.ToMap()
				argsJSON, _ := json.Marshal(argsMap)
				_ = json.Unmarshal(argsJSON, &args)

				result, execErr := r.commands.Exec(ctx, args.Command, args.Stdin)

				var resultContent string
				if execErr != nil {
					resultContent = fmt.Sprintf("[error] run failed: %v", execErr)
					log.Printf("[the breath] run failed: %v", execErr)
				} else {
					resultContent = result
					log.Printf("[the breath] run completed")
				}

				records = append(records, toolCallRecord{
					Tool:    "run",
					Input:   args.Command,
					Output:  truncateString(resultContent, 200),
					Success: !strings.HasPrefix(resultContent, "[error]"),
				})

				messages = append(messages, api.Message{
					Role:       "tool",
					Content:    resultContent,
					ToolName:   "run",
					ToolCallID: tc.ID,
				})
			}

			continue
		}

		// Final text response — store, reply, commit.
		if resp.Content != "" {
			log.Printf("[the mind] order complete, response formed")
			r.storeAndReply(conversationID, resp.Content)

			// Extract a learning and blueprint from the completed task.
			go func() {
				learnCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := ExtractLearning(learnCtx, r.provider, conversationID, r.memory); err != nil {
					log.Printf("[learn] failed to extract learning: %v", err)
				}
			}()
			go func() {
				bpCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := ExtractBlueprint(bpCtx, r.provider, conversationID, r.memory, records); err != nil {
					log.Printf("[blueprint] failed to extract blueprint: %v", err)
				}
			}()
			return
		}

		// Empty response with no tool calls — bail out.
		log.Printf("[the mind] empty response at iteration %d", i)
		r.sendReply(conversationID, "The vision clears but reveals nothing.")
		return
	}

	log.Printf("[the mind] tool loop exhausted after %d iterations", maxIter)
	r.sendReply(conversationID, fmt.Sprintf("The breath has been exhausted after %d cycles. The task remains incomplete.", maxIter))
}

// handleChat sends a simple LLM reply without tool use.
func (r *Runtime) handleChat(ctx context.Context, conversationID, userMessage string) {
	log.Printf("[the mind] casual exchange")

	messages, err := r.ctxBuilder.Build(ctx, conversationID, userMessage)
	if err != nil {
		log.Printf("[the mind] context build failed: %v", err)
		r.sendReply(conversationID, "The memories blur. I cannot find my thoughts.")
		return
	}

	resp, err := r.provider.Chat(ctx, messages, nil)
	if err != nil {
		log.Printf("[the mind] ollama chat failed: %v", err)
		r.sendReply(conversationID, "The temporal winds resist. I cannot speak.")
		return
	}

	r.storeAndReply(conversationID, resp.Content)
}

// handleStop acknowledges a stop request.
func (r *Runtime) handleStop(_ context.Context, conversationID string) {
	log.Printf("[the mind] stop command acknowledged")
	r.sendReply(conversationID, "The crystal dims. I shall rest.")
}

// storeAndReply stores the assistant message, sends the reply, and commits.
func (r *Runtime) storeAndReply(conversationID, content string) {
	tokenEst := r.provider.TokenCount(content)
	if _, err := r.memory.StoreMessage(conversationID, memory.RoleAssistant, content, 0.7, tokenEst); err != nil {
		log.Printf("[the mind] failed to store assistant message: %v", err)
	}

	if err := r.memory.UpdateConversationTimestamp(conversationID); err != nil {
		log.Printf("[the mind] failed to update conversation timestamp: %v", err)
	}

	r.memory.AutoCommit(fmt.Sprintf("agent response in conversation %s", conversationID))
	r.sendReply(conversationID, content)
}

// sendReply looks up the conversation's contact and sends the text via the channel.
func (r *Runtime) sendReply(conversationID, text string) {
	conv, err := r.memory.GetConversation(conversationID)
	if err != nil || conv == nil {
		log.Printf("[the mind] cannot find conversation %s for reply: %v", conversationID, err)
		return
	}

	chatRef := channel.ChatRef{ContactID: conv.ContactID}
	if err := r.channel.Send(chatRef, text); err != nil {
		log.Printf("[the mind] failed to send reply: %v", err)
	}
}

// findOrCreateConversation returns an existing conversation for the contact or creates one.
func (r *Runtime) findOrCreateConversation(chatInfo channel.ChatInfo) (*memory.Conversation, error) {
	conv, err := r.memory.GetConversationByContact(chatInfo.ContactID)
	if err != nil {
		return nil, fmt.Errorf("looking up conversation for contact %s: %w", chatInfo.ContactID, err)
	}
	if conv != nil {
		return conv, nil
	}

	conv, err = r.memory.CreateConversation(chatInfo.ContactID, chatInfo.DisplayName)
	if err != nil {
		return nil, fmt.Errorf("creating conversation for contact %s: %w", chatInfo.ContactID, err)
	}
	return conv, nil
}

// Shutdown performs graceful cleanup: closes all contact channels
// and waits for in-flight processing to complete.
func (r *Runtime) Shutdown() {
	log.Printf("[the mind] the crystal dims, shutting down")

	r.convMu.Lock()
	for id, ch := range r.convCh {
		close(ch)
		delete(r.convCh, id)
	}
	r.convMu.Unlock()

	r.wg.Wait()
}


// truncateString truncates s to at most n bytes, appending "..." if truncated.
func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}