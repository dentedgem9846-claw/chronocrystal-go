package memory

import (
	"testing"

	"github.com/chronocrystal/chronocrystal-go/internal/config"
)

func TestCreateAndGetConversation(t *testing.T) {
	store := openTestStore(t)

	conv, err := store.CreateConversation("contact-1", "Alice")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	if conv.ID == "" {
		t.Error("expected non-empty conversation ID")
	}
	if conv.ContactID != "contact-1" {
		t.Errorf("ContactID = %q, want %q", conv.ContactID, "contact-1")
	}
	if conv.DisplayName != "Alice" {
		t.Errorf("DisplayName = %q, want %q", conv.DisplayName, "Alice")
	}

	// Retrieve by ID.
	got, err := store.GetConversation(conv.ID)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if got == nil {
		t.Fatal("GetConversation returned nil")
	}
	if got.ID != conv.ID {
		t.Errorf("ID = %q, want %q", got.ID, conv.ID)
	}
	if got.ContactID != "contact-1" {
		t.Errorf("ContactID = %q, want %q", got.ContactID, "contact-1")
	}

	// Retrieve by contact ID.
	gotByContact, err := store.GetConversationByContact("contact-1")
	if err != nil {
		t.Fatalf("GetConversationByContact: %v", err)
	}
	if gotByContact == nil {
		t.Fatal("GetConversationByContact returned nil")
	}
	if gotByContact.ID != conv.ID {
		t.Errorf("ID = %q, want %q", gotByContact.ID, conv.ID)
	}
}

func TestGetNonexistentConversation(t *testing.T) {
	store := openTestStore(t)

	got, err := store.GetConversation("nonexistent-id")
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent conversation, got %+v", got)
	}

	gotByContact, err := store.GetConversationByContact("nonexistent-contact")
	if err != nil {
		t.Fatalf("GetConversationByContact: %v", err)
	}
	if gotByContact != nil {
		t.Errorf("expected nil for nonexistent contact, got %+v", gotByContact)
	}
}

func TestListConversations(t *testing.T) {
	store := openTestStore(t)

	_, err := store.CreateConversation("c1", "Alice")
	if err != nil {
		t.Fatalf("CreateConversation c1: %v", err)
	}
	_, err = store.CreateConversation("c2", "Bob")
	if err != nil {
		t.Fatalf("CreateConversation c2: %v", err)
	}

	convs, err := store.ListConversations()
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(convs) != 2 {
		t.Errorf("expected 2 conversations, got %d", len(convs))
	}
}

func TestUpdateConversationTimestamp(t *testing.T) {
	store := openTestStore(t)

	conv, err := store.CreateConversation("c1", "Alice")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	err = store.UpdateConversationTimestamp(conv.ID)
	if err != nil {
		t.Fatalf("UpdateConversationTimestamp: %v", err)
	}
}

func TestConversationAutoCommitOff(t *testing.T) {
	// Verify that with AutoCommit=false, CreateConversation still works.
	cfg := config.MemoryConfig{
		DBPath:      ":memory:",
		AutoCommit:  false,
		LambdaDecay: 0.01,
	}
	store, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	defer store.Close()

	_, err = store.CreateConversation("c1", "Test")
	if err != nil {
		t.Fatalf("CreateConversation with AutoCommit=false: %v", err)
	}
}