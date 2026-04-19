package memory

import (
	"math"
	"testing"
	"time"

	"github.com/chronocrystal/chronocrystal-go/internal/config"
)

func TestDecayCalculation(t *testing.T) {
	// Fresh message (0 hours old): importance * e^0 = importance * 1 = importance.
	score := DecayedImportance(0.8, time.Now(), 0.01)
	if math.Abs(score-0.8) > 1e-9 {
		t.Errorf("fresh decay: got %f, want 0.8", score)
	}

	// 100 hours old: importance * e^(-0.01 * 100) = 0.8 * e^(-1) ≈ 0.8 * 0.36788.
	createdAt := time.Now().Add(-100 * time.Hour)
	score = DecayedImportance(0.8, createdAt, 0.01)
	expected := 0.8 * math.Exp(-1)
	if math.Abs(score-expected) > 1e-6 {
		t.Errorf("100h decay: got %f, want %f", score, expected)
	}

	// Very old message: score should be near zero.
	createdAt = time.Now().Add(-10000 * time.Hour)
	score = DecayedImportance(1.0, createdAt, 0.01)
	if score > 0.001 {
		t.Errorf("very old decay: got %f, want near 0", score)
	}

	// Higher lambda decays faster.
	highLambda := DecayedImportance(1.0, time.Now().Add(-24*time.Hour), 0.1)
	lowLambda := DecayedImportance(1.0, time.Now().Add(-24*time.Hour), 0.01)
	if highLambda >= lowLambda {
		t.Errorf("higher lambda should produce lower score: high=%f, low=%f", highLambda, lowLambda)
	}
}

func TestFidelityLayerFullFits(t *testing.T) {
	store := openTestStore(t)
	convID := createTestConversation(t, store)
	lm := NewLambdaMemory(store, config.MemoryConfig{
		DBPath:         ":memory:",
		LambdaDecay:    0.01,
		GoneThreshold:  0.01,
		LambdaBudgetPct: 0.15,
	})

	msg, err := store.StoreMessage(convID, RoleUser, "short", 0.9, 1)
	if err != nil {
		t.Fatalf("StoreMessage: %v", err)
	}

	// Large budget: should keep full fidelity.
	result := lm.ApplyFidelity(msg, 10000)
	if result != "short" {
		t.Errorf("expected full content %q, got %q", "short", result)
	}
}

func TestFidelityLayerSummary(t *testing.T) {
	store := openTestStore(t)
	convID := createTestConversation(t, store)
	lm := NewLambdaMemory(store, config.MemoryConfig{
		DBPath:         ":memory:",
		LambdaDecay:    0.01,
		GoneThreshold:  0.01,
		LambdaBudgetPct: 0.15,
	})

	// Create a message with many lines — content ~80 chars = ~20 tokens.
	// With budget=5, full content won't fit, so fidelity degrades to summary.
	content := ""
	for i := 0; i < 20; i++ {
		if i > 0 {
			content += "\n"
		}
		content += "this is a longer line of content that makes it bigger"
	}
	msg, err := store.StoreMessage(convID, RoleAssistant, content, 0.5, 0)
	if err != nil {
		t.Fatalf("StoreMessage: %v", err)
	}

	// Budget too small for full but enough for summary.
	result := lm.ApplyFidelity(msg, 50)
	if result == content {
		t.Error("expected compressed content, got full content")
	}
}

func TestFidelityLayerEssence(t *testing.T) {
	store := openTestStore(t)
	convID := createTestConversation(t, store)
	lm := NewLambdaMemory(store, config.MemoryConfig{
		DBPath:         ":memory:",
		LambdaDecay:    0.01,
		GoneThreshold:  0.01,
		LambdaBudgetPct: 0.15,
	})

	// Content ~52 chars = ~13 tokens. Budget=5 forces degradation past summary to essence.
	msg, err := store.StoreMessage(convID, RoleAssistant, "This is the first sentence. Second sentence here. Third.", 0.5, 100)
	if err != nil {
		t.Fatalf("StoreMessage: %v", err)
	}

	// With token_count=100 and budget=5, full won't fit, summary (50 tokens) won't fit, essence will.
	result := lm.ApplyFidelity(msg, 20)
	if result != "This is the first sentence." {
		t.Errorf("essence: got %q, want %q", result, "This is the first sentence.")
	}
}

func TestFidelityLayerHash(t *testing.T) {
	store := openTestStore(t)
	convID := createTestConversation(t, store)
	lm := NewLambdaMemory(store, config.MemoryConfig{
		DBPath:         ":memory:",
		LambdaDecay:    0.01,
		GoneThreshold:  0.01,
		LambdaBudgetPct: 0.15,
	})

	msg, err := store.StoreMessage(convID, RoleAssistant, "Some content", 0.5, 0)
	if err != nil {
		t.Fatalf("StoreMessage: %v", err)
	}

	// Essentially zero budget: should fall to hash.
	result := lm.ApplyFidelity(msg, 0)
	if len(result) < 6 || result[:5] != "[ref:" {
		t.Errorf("hash fidelity: got %q, want [ref:...] format", result)
	}
}

func TestGoneThreshold(t *testing.T) {
	store := openTestStore(t)
	convID := createTestConversation(t, store)
	lm := NewLambdaMemory(store, config.MemoryConfig{
		DBPath:         ":memory:",
		LambdaDecay:    0.01,
		GoneThreshold:  0.01,
		LambdaBudgetPct: 0.15,
	})

	// Create a message with very low importance.
	// With importance=0.001 and no time decay, it's below the gone threshold of 0.01.
	_, err := store.StoreMessage(convID, RoleUser, "I should be pruned", 0.001, 3)
	if err != nil {
		t.Fatalf("StoreMessage: %v", err)
	}

	// Create a message above gone threshold.
	highMsg, err := store.StoreMessage(convID, RoleUser, "I should survive", 0.9, 3)
	if err != nil {
		t.Fatalf("StoreMessage: %v", err)
	}

	err = lm.PruneGone(convID)
	if err != nil {
		t.Fatalf("PruneGone: %v", err)
	}

	// Check the low-importance message is now gone.
	msgs, err := store.GetMessagesForContext(convID, 10)
	if err != nil {
		t.Fatalf("GetMessagesForContext: %v", err)
	}
	for _, m := range msgs {
		if m.Content == "I should be pruned" && m.FidelityLevel != FidelityHash {
			t.Errorf("low-importance message should have hash fidelity, got %q", m.FidelityLevel)
		}
	}

	// High-importance message should still be full.
	highGot, err := store.GetMessage(highMsg.ID)
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if highGot.FidelityLevel != FidelityFull {
		t.Errorf("high-importance message: fidelity = %q, want full", highGot.FidelityLevel)
	}
}

func TestGetMessagesForContext(t *testing.T) {
	store := openTestStore(t)
	convID := createTestConversation(t, store)
	lm := NewLambdaMemory(store, config.MemoryConfig{
		DBPath:         ":memory:",
		LambdaDecay:    0.01,
		GoneThreshold:  0.01,
		LambdaBudgetPct: 0.15,
	})

	// Store several messages.
	for i := 0; i < 5; i++ {
		_, err := store.StoreMessage(convID, RoleUser, "message content here", 0.8, 5)
		if err != nil {
			t.Fatalf("StoreMessage %d: %v", i, err)
		}
	}

	// Large budget should include all messages.
	msgs, err := lm.GetContextMessages(convID, 10000, 2)
	if err != nil {
		t.Fatalf("GetContextMessages: %v", err)
	}
	if len(msgs) < 2 {
		t.Errorf("expected at least 2 messages with large budget, got %d", len(msgs))
	}
}

func TestGetMessagesForContextSmallBudget(t *testing.T) {
	store := openTestStore(t)
	convID := createTestConversation(t, store)
	lm := NewLambdaMemory(store, config.MemoryConfig{
		DBPath:         ":memory:",
		LambdaDecay:    0.01,
		GoneThreshold:  0.01,
		LambdaBudgetPct: 0.15,
	})

	// Store several messages.
	for i := 0; i < 5; i++ {
		_, err := store.StoreMessage(convID, RoleUser, "message content here", 0.8, 5)
		if err != nil {
			t.Fatalf("StoreMessage %d: %v", i, err)
		}
	}

	// Small budget above 1000 (output reserve) should still return at least the recent messages.
	// Budget of 1200: 200 tokens for context after reserving 1000 for output.
	msgs, err := lm.GetContextMessages(convID, 1200, 2)
	if err != nil {
		t.Fatalf("GetContextMessages: %v", err)
	}
	if len(msgs) < 1 {
		t.Error("expected at least 1 message with small budget")
}
}
func TestGetContextMessagesBudgetTooSmall(t *testing.T) {
	store := openTestStore(t)
	convID := createTestConversation(t, store)
	lm := NewLambdaMemory(store, config.MemoryConfig{
		DBPath:         ":memory:",
		LambdaDecay:    0.01,
		GoneThreshold:  0.01,
		LambdaBudgetPct: 0.15,
	})

	// Budget at or below outputTokenReserve (1000) should return error.
	_, err := lm.GetContextMessages(convID, 500, 2)
	if err == nil {
		t.Error("expected error for budget below output reserve")
	}
}

func TestBuildSummary(t *testing.T) {
	// Short content: returns as-is.
	short := "line1\nline2"
	if got := buildSummary(short); got != short {
		t.Errorf("buildSummary short: got %q, want %q", got, short)
	}

	// Long content: head + ... + tail.
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "content line"
	}
	long := ""
	for i, l := range lines {
		if i > 0 {
			long += "\n"
		}
		long += l
	}
	result := buildSummary(long)
	if len(result) >= len(long) {
		t.Error("buildSummary should compress long content")
	}
}

func TestBuildEssence(t *testing.T) {
	// Sentence with period.
	got := buildEssence("First sentence. Second sentence.")
	if got != "First sentence." {
		t.Errorf("buildEssence: got %q, want %q", got, "First sentence.")
	}

	// Empty string.
	got = buildEssence("")
	if got != "" {
		t.Errorf("buildEssence empty: got %q, want empty", got)
	}

	// No sentence terminator — truncates to maxEssenceLen.
	long := "a very long string without any period"
	essence := buildEssence(long)
	if essence != long {
		t.Errorf("buildEssence short no-terminator: got %q, want %q", essence, long)
	}
}