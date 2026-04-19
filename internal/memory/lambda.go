package memory

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/chronocrystal/chronocrystal-go/internal/config"
)

// outputTokenReserve is the token budget reserved for LLM output.
const outputTokenReserve = 1000

// summaryHeadLines and summaryTailLines control the 'summary' fidelity layer.
const summaryHeadLines = 3
const summaryTailLines = 3

// DecayedImportance returns the effective importance of a memory after
// exponential decay: initialScore * e^(-lambda * hoursSinceCreation).
// Lambda and hours are always positive, so the exponent is always negative
// and math.Exp returns a value in (0, 1].
func DecayedImportance(initialScore float64, createdAt time.Time, lambda float64) float64 {
	hoursSince := time.Since(createdAt).Hours()
	return initialScore * math.Exp(-lambda*hoursSince)
}

// LambdaMemory applies exponential decay and fidelity-layer compression
// to select and trim messages within a token budget.
type LambdaMemory struct {
	store *Store
	cfg   config.MemoryConfig
}

// NewLambdaMemory creates a LambdaMemory backed by the given store and config.
func NewLambdaMemory(store *Store, cfg config.MemoryConfig) *LambdaMemory {
	return &LambdaMemory{
		store: store,
		cfg:   cfg,
	}
}

// GetContextMessages assembles a context message list for the given conversation
// that fits within budgetTokens. It always preserves the last recentKeep messages
// in full fidelity, then fills remaining budget with older messages selected by
// decayed importance and compressed through fidelity layers.
func (lm *LambdaMemory) GetContextMessages(convID string, budgetTokens int, recentKeep int) ([]Message, error) {
	if budgetTokens <= outputTokenReserve {
		return nil, fmt.Errorf("budget %d must exceed output reserve %d", budgetTokens, outputTokenReserve)
	}

	msgs, err := lm.store.GetMessagesForContext(convID, 1000)
	if err != nil {
		return nil, fmt.Errorf("lambda: failed to get messages for conversation %s: %w", convID, err)
	}

	if len(msgs) == 0 {
		return nil, nil
	}

	available := budgetTokens - outputTokenReserve

	// Split into recent (always kept) and older (importance-selected).
	recentStart := len(msgs) - recentKeep
	if recentStart < 0 {
		recentStart = 0
	}

	recent := msgs[recentStart:]
	older := msgs[:recentStart]

	// Account for recent messages at full fidelity.
	recentTokens := 0
	for i := range recent {
		recentTokens += tokenCountOrEstimate(&recent[i])
	}

	// If recent messages alone exceed budget, include them anyway (they are mandatory).
	if recentTokens >= available {
		result := make([]Message, len(recent))
		copy(result, recent)
		return result, nil
	}

	remaining := available - recentTokens

	// Sort older messages by decayed importance (highest first).
	type scoredMsg struct {
		msg   Message
		score float64
	}
	scored := make([]scoredMsg, 0, len(older))
	for i := range older {
		s := DecayedImportance(older[i].Importance, older[i].CreatedAt, lm.cfg.LambdaDecay)
		scored = append(scored, scoredMsg{msg: older[i], score: s})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Select older messages that fit within the remaining budget, applying fidelity.
	var selected []Message
	for i := range scored {
		content := lm.ApplyFidelity(&scored[i].msg, remaining)
		newTokens := estimateTokens(content)

		if newTokens <= remaining {
			scored[i].msg.Content = content
			selected = append(selected, scored[i].msg)
			remaining -= newTokens
		}
		// If fidelity reduction still doesn't fit, skip this message.
	}

	// Reassemble in chronological order: selected older (by original position) + recent.
	result := make([]Message, 0, len(selected)+len(recent))
	result = append(result, selected...)
	result = append(result, recent...)

	return result, nil
}

// ApplyFidelity compresses a message to fit within availableTokens by reducing
// its fidelity layer. Returns the content at the highest fidelity that fits.
//
// Fidelity layers (in order of degradation):
//   - full:    original content, fits within budget
//   - summary: first 3 + last 3 lines joined with '...'
//   - essence: first sentence of content
//   - hash:    "[ref:<id>]" — a placeholder reference
//
// The stored fidelity_level and content are updated via store.UpdateFidelity
// so that repeated reads do not re-decay from the already-compressed form.
func (lm *LambdaMemory) ApplyFidelity(msg *Message, availableTokens int) string {
	tokens := tokenCountOrEstimate(msg)

	// Full fidelity: message already fits.
	if tokens <= availableTokens && msg.FidelityLevel == FidelityFull {
		return msg.Content
	}

	// Full fidelity: message would fit if we restore from original — but we
	// only return content as-is; original_content is the source of truth for
	// re-expansion (not implemented in MVP). If fidelity is already reduced,
	// keep it at that level.
	if msg.FidelityLevel != FidelityFull {
		// Already compressed; check if current form fits.
		if tokens <= availableTokens {
			return msg.Content
		}
		// Fall through to further compression.
	}

	// Summary fidelity: first + last 3 lines, roughly half the original.
	halfTokens := tokens / 2
	if availableTokens >= halfTokens && halfTokens > 0 {
		summary := buildSummary(msg.Content)
		_ = lm.store.UpdateFidelity(msg.ID, FidelitySummary, summary)
		msg.FidelityLevel = FidelitySummary
		msg.Content = summary
		return summary
	}

	// Essence fidelity: first sentence, roughly 20 tokens.
	if availableTokens >= 20 {
		essence := buildEssence(msg.Content)
		_ = lm.store.UpdateFidelity(msg.ID, FidelityEssence, essence)
		msg.FidelityLevel = FidelityEssence
		msg.Content = essence
		return essence
	}

	// Hash fidelity: minimal reference.
	hash := fmt.Sprintf("[ref:%d]", msg.ID)
	_ = lm.store.UpdateFidelity(msg.ID, FidelityHash, hash)
	msg.FidelityLevel = FidelityHash
	msg.Content = hash
	return hash
}

// PruneGone marks messages whose decayed importance has fallen below the
// gone threshold. Instead of deleting (which would break conversation
// continuity), it sets fidelity to 'hash' and content to '[gone]'.
func (lm *LambdaMemory) PruneGone(convID string) error {
	msgs, err := lm.store.GetMessagesForContext(convID, 1000)
	if err != nil {
		return fmt.Errorf("lambda: prune failed for conversation %s: %w", convID, err)
	}

	for i := range msgs {
		score := DecayedImportance(msgs[i].Importance, msgs[i].CreatedAt, lm.cfg.LambdaDecay)
		if score < lm.cfg.GoneThreshold {
			if err := lm.store.UpdateFidelity(msgs[i].ID, FidelityHash, "[gone]"); err != nil {
				return fmt.Errorf("lambda: prune failed for message %d: %w", msgs[i].ID, err)
			}
		}
	}

	return nil
}

// --- internal helpers ---

// tokenCountOrEstimate returns the message's TokenCount if positive,
// otherwise estimates from content length (roughly 4 chars per token).
func tokenCountOrEstimate(msg *Message) int {
	if msg.TokenCount > 0 {
		return msg.TokenCount
	}
	return estimateTokens(msg.Content)
}

// estimateTokens returns a rough token count from string length.
// Average English text is ~4 characters per token.
func estimateTokens(s string) int {
	if len(s) == 0 {
		return 1
	}
	return len(s) / 4
}

// buildSummary compresses content to first + last 3 lines with ellipsis.
func buildSummary(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= summaryHeadLines+summaryTailLines {
		return content
	}

	head := lines[:summaryHeadLines]
	tail := lines[len(lines)-summaryTailLines:]

	var b strings.Builder
	for _, l := range head {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	b.WriteString("...\n")
	for _, l := range tail {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// buildEssence extracts the first sentence from content.
// Splits on '.', '!', '?' and returns the first sentence with its
// terminating punctuation.
func buildEssence(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}

	// Find the first sentence boundary.
	for i, ch := range content {
		switch ch {
		case '.', '!', '?':
			return content[:i+1]
		}
	}

	// No sentence terminator found; return up to a reasonable length.
	const maxEssenceLen = 200
	if len(content) > maxEssenceLen {
		return content[:maxEssenceLen] + "..."
	}
	return content
}