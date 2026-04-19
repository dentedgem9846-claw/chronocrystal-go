package memory

import (
	"testing"
	"time"
)

func createTestConversation(t *testing.T, store *Store) string {
	t.Helper()
	conv, err := store.CreateConversation("test-contact", "TestUser")
	if err != nil {
		t.Fatalf("createTestConversation: %v", err)
	}
	return conv.ID
}

func TestStoreAndGetMessage(t *testing.T) {
	store := openTestStore(t)
	convID := createTestConversation(t, store)

	msg, err := store.StoreMessage(convID, RoleUser, "Hello, world!", 0.8, 5)
	if err != nil {
		t.Fatalf("StoreMessage: %v", err)
	}

	if msg.ID == 0 {
		t.Error("expected non-zero message ID")
	}
	if msg.ConversationID != convID {
		t.Errorf("ConversationID = %q, want %q", msg.ConversationID, convID)
	}
	if msg.Role != RoleUser {
		t.Errorf("Role = %q, want %q", msg.Role, RoleUser)
	}
	if msg.Content != "Hello, world!" {
		t.Errorf("Content = %q, want %q", msg.Content, "Hello, world!")
	}
	if msg.Importance != 0.8 {
		t.Errorf("Importance = %f, want 0.8", msg.Importance)
	}
	if msg.FidelityLevel != FidelityFull {
		t.Errorf("FidelityLevel = %q, want %q", msg.FidelityLevel, FidelityFull)
	}
	if msg.TokenCount != 5 {
		t.Errorf("TokenCount = %d, want 5", msg.TokenCount)
	}

	// Retrieve by ID.
	got, err := store.GetMessage(msg.ID)
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if got == nil {
		t.Fatal("GetMessage returned nil")
	}
	if got.Content != "Hello, world!" {
		t.Errorf("Content = %q, want %q", got.Content, "Hello, world!")
	}
}

func TestGetNonexistentMessage(t *testing.T) {
	store := openTestStore(t)

	got, err := store.GetMessage(99999)
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent message, got %+v", got)
	}
}

func TestStoreMessageImportanceClamp(t *testing.T) {
	store := openTestStore(t)
	convID := createTestConversation(t, store)

	// Below 0 should clamp to 0.
	msg, err := store.StoreMessage(convID, RoleUser, "negative importance", -0.5, 3)
	if err != nil {
		t.Fatalf("StoreMessage: %v", err)
	}
	if msg.Importance != 0.0 {
		t.Errorf("Importance = %f, want 0.0 (clamped from -0.5)", msg.Importance)
	}

	// Above 1 should clamp to 1.
	msg, err = store.StoreMessage(convID, RoleUser, "over importance", 1.5, 3)
	if err != nil {
		t.Fatalf("StoreMessage: %v", err)
	}
	if msg.Importance != 1.0 {
		t.Errorf("Importance = %f, want 1.0 (clamped from 1.5)", msg.Importance)
	}
}

func TestGetRecentMessages(t *testing.T) {
	store := openTestStore(t)
	convID := createTestConversation(t, store)

	for i := 0; i < 5; i++ {
		_, err := store.StoreMessage(convID, RoleUser, "msg"+string(rune('A'+i)), 0.5, 4)
		if err != nil {
			t.Fatalf("StoreMessage %d: %v", i, err)
		}
	}

	// Request only 3 most recent.
	msgs, err := store.ListMessages(convID, 3)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("expected 3 messages, got %d", len(msgs))
	}
}

func TestMessageOrdering(t *testing.T) {
	store := openTestStore(t)
	convID := createTestConversation(t, store)

	// Store messages with small delays to ensure ordering.
	content := []string{"first", "second", "third"}
	for _, c := range content {
		_, err := store.StoreMessage(convID, RoleUser, c, 0.5, 4)
		if err != nil {
			t.Fatalf("StoreMessage: %v", err)
		}
		time.Sleep(1 * time.Second)
	}

	// GetMessagesForContext returns messages in chronological order (oldest first).
	msgs, err := store.GetMessagesForContext(convID, 10)
	if err != nil {
		t.Fatalf("GetMessagesForContext: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	for i, want := range content {
		if msgs[i].Content != want {
			t.Errorf("message %d: Content = %q, want %q", i, msgs[i].Content, want)
		}
	}
}

func TestUpdateFidelity(t *testing.T) {
	store := openTestStore(t)
	convID := createTestConversation(t, store)

	msg, err := store.StoreMessage(convID, RoleAssistant, "Original long content here", 0.5, 10)
	if err != nil {
		t.Fatalf("StoreMessage: %v", err)
	}

	err = store.UpdateFidelity(msg.ID, FidelitySummary, "Short summary")
	if err != nil {
		t.Fatalf("UpdateFidelity: %v", err)
	}

	updated, err := store.GetMessage(msg.ID)
	if err != nil {
		t.Fatalf("GetMessage after UpdateFidelity: %v", err)
	}
	if updated.FidelityLevel != FidelitySummary {
		t.Errorf("FidelityLevel = %q, want %q", updated.FidelityLevel, FidelitySummary)
	}
	if updated.Content != "Short summary" {
		t.Errorf("Content = %q, want %q", updated.Content, "Short summary")
	}
	if updated.OriginalContent != "Original long content here" {
		t.Errorf("OriginalContent = %q, want %q", updated.OriginalContent, "Original long content here")
	}
}

func TestEstimateTokens(t *testing.T) {
	store := openTestStore(t)

	// "abcdefghijklmnop" = 16 chars -> 16/4 = 4 tokens.
	result := store.EstimateTokens("abcdefghijklmnop")
	if result != 4 {
		t.Errorf("EstimateTokens = %d, want 4", result)
	}
}