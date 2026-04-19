package memory

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Conversation represents a single conversation with a contact.
type Conversation struct {
	ID          string
	ContactID   string
	DisplayName string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CreateConversation creates a new conversation with a generated UUID.
func (s *Store) CreateConversation(contactID, displayName string) (*Conversation, error) {
	id := uuid.New().String()
	now := time.Now()

	_, err := s.db.Exec(
		`INSERT INTO conversations (id, contact_id, display_name, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		id, contactID, displayName, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting conversation: %w", err)
	}

	s.AutoCommit(fmt.Sprintf("created conversation %s", id))

	return &Conversation{
		ID:          id,
		ContactID:   contactID,
		DisplayName: displayName,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// GetConversation retrieves a conversation by ID.
// Returns nil, nil when the conversation does not exist.
func (s *Store) GetConversation(id string) (*Conversation, error) {
	var c Conversation
	err := s.db.QueryRow(
		`SELECT id, contact_id, display_name, created_at, updated_at
		 FROM conversations WHERE id = ?`,
		id,
	).Scan(&c.ID, &c.ContactID, &c.DisplayName, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying conversation %s: %w", id, err)
	}
	return &c, nil
}

// GetConversationByContact retrieves a conversation by contact ID.
// Returns nil, nil when no conversation exists for the contact.
func (s *Store) GetConversationByContact(contactID string) (*Conversation, error) {
	var c Conversation
	err := s.db.QueryRow(
		`SELECT id, contact_id, display_name, created_at, updated_at
		 FROM conversations WHERE contact_id = ?`,
		contactID,
	).Scan(&c.ID, &c.ContactID, &c.DisplayName, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying conversation by contact %s: %w", contactID, err)
	}
	return &c, nil
}

// ListConversations returns all conversations ordered by most recently updated.
func (s *Store) ListConversations() ([]Conversation, error) {
	rows, err := s.db.Query(
		`SELECT id, contact_id, display_name, created_at, updated_at
		 FROM conversations ORDER BY updated_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing conversations: %w", err)
	}
	defer rows.Close()

	var conversations []Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.ContactID, &c.DisplayName, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning conversation: %w", err)
		}
		conversations = append(conversations, c)
	}
	return conversations, rows.Err()
}

// UpdateConversationTimestamp sets updated_at to the current timestamp.
func (s *Store) UpdateConversationTimestamp(id string) error {
	_, err := s.db.Exec(
		`UPDATE conversations SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		id,
	)
	if err != nil {
		return fmt.Errorf("updating conversation timestamp %s: %w", id, err)
	}

	s.AutoCommit(fmt.Sprintf("updated conversation %s", id))
	return nil
}
