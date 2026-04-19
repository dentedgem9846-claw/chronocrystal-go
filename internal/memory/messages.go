package memory

import (
	"database/sql"
	"fmt"
	"time"
)

type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleSystem    MessageRole = "system"
	RoleTool      MessageRole = "tool"
)

type FidelityLevel string

const (
	FidelityFull    FidelityLevel = "full"
	FidelitySummary FidelityLevel = "summary"
	FidelityEssence FidelityLevel = "essence"
	FidelityHash    FidelityLevel = "hash"
)

type Message struct {
	ID              int64
	ConversationID  string
	Role            MessageRole
	Content         string
	Importance      float64
	FidelityLevel   FidelityLevel
	OriginalContent string
	TokenCount      int
	CreatedAt       time.Time
}

// StoreMessage inserts a new message with the given fields and sets fidelity to full.
func (s *Store) StoreMessage(convID string, role MessageRole, content string, importance float64, tokenCount int) (*Message, error) {
	if importance < 0.0 {
		importance = 0.0
	}
	if importance > 1.0 {
		importance = 1.0
	}

	result, err := s.db.Exec(
		`INSERT INTO messages (conversation_id, role, content, importance, fidelity_level, token_count)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		convID, string(role), content, importance, string(FidelityFull), tokenCount,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting message into conversation %s: %w", convID, err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("getting last insert id: %w", err)
	}

	msg, err := s.GetMessage(id)
	if err != nil {
		return nil, fmt.Errorf("reading back stored message: %w", err)
	}

	s.AutoCommit(fmt.Sprintf("stored message in %s", convID))

	return msg, nil
}

// GetMessage returns a message by ID. Returns nil, nil if not found.
func (s *Store) GetMessage(id int64) (*Message, error) {
	row := s.db.QueryRow(
		`SELECT id, conversation_id, role, content, importance, fidelity_level,
		        COALESCE(original_content, ''), token_count, created_at
		 FROM messages WHERE id = ?`,
		id,
	)

	var m Message
	var roleStr string
	var fidelityStr string
	var createdAtStr string

	err := row.Scan(
		&m.ID, &m.ConversationID, &roleStr, &m.Content,
		&m.Importance, &fidelityStr, &m.OriginalContent,
		&m.TokenCount, &createdAtStr,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scanning message id %d: %w", id, err)
	}

	m.Role = MessageRole(roleStr)
	m.FidelityLevel = FidelityLevel(fidelityStr)
	m.CreatedAt, err = parseTime(createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing created_at for message id %d: %w", id, err)
	}

	return &m, nil
}

// ListMessages returns the most recent messages for a conversation, ordered by created_at descending.
func (s *Store) ListMessages(convID string, limit int) ([]Message, error) {
	rows, err := s.db.Query(
		`SELECT id, conversation_id, role, content, importance, fidelity_level,
		        COALESCE(original_content, ''), token_count, created_at
		 FROM messages WHERE conversation_id = ?
		 ORDER BY created_at DESC LIMIT ?`,
		convID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("listing messages for conversation %s: %w", convID, err)
	}
	defer rows.Close()

	return scanMessages(rows)
}

// UpdateFidelity changes a message's fidelity level and replaces its content with the summary.
// If the original content hasn't been saved yet, it preserves the current content first.
func (s *Store) UpdateFidelity(id int64, level FidelityLevel, summaryContent string) error {
	// Preserve original content before overwriting.
	var originalContent sql.NullString
	err := s.db.QueryRow(
		`SELECT original_content FROM messages WHERE id = ?`, id,
	).Scan(&originalContent)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("message id %d not found", id)
		}
		return fmt.Errorf("checking original_content for message id %d: %w", id, err)
	}

	if !originalContent.Valid || originalContent.String == "" {
		_, err = s.db.Exec(
			`UPDATE messages SET original_content = content, content = ?, fidelity_level = ? WHERE id = ?`,
			summaryContent, string(level), id,
		)
	} else {
		_, err = s.db.Exec(
			`UPDATE messages SET content = ?, fidelity_level = ? WHERE id = ?`,
			summaryContent, string(level), id,
		)
	}
	if err != nil {
		return fmt.Errorf("updating fidelity for message id %d: %w", id, err)
	}

	s.AutoCommit(fmt.Sprintf("updated message %d fidelity to %s", id, level))

	return nil
}

// GetMessagesForContext returns messages for a conversation in chronological order (oldest first),
// used by the context builder.
func (s *Store) GetMessagesForContext(convID string, limit int) ([]Message, error) {
	rows, err := s.db.Query(
		`SELECT id, conversation_id, role, content, importance, fidelity_level,
		        COALESCE(original_content, ''), token_count, created_at
		 FROM messages WHERE conversation_id = ?
		 ORDER BY created_at ASC LIMIT ?`,
		convID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("getting context messages for conversation %s: %w", convID, err)
	}
	defer rows.Close()

	return scanMessages(rows)
}

// GetMessages returns messages for a conversation in chronological order.
// This is the primary method used by the lambda memory system.
func (s *Store) GetMessages(convID string, limit int) ([]Message, error) {
	return s.GetMessagesForContext(convID, limit)
}

// EstimateTokens provides a rough token count approximation.
func (s *Store) EstimateTokens(text string) int {
	return len(text) / 4
}

// scanMessages reads all rows from a query result into a slice of Message.
func scanMessages(rows *sql.Rows) ([]Message, error) {
	var messages []Message
	for rows.Next() {
		var m Message
		var roleStr string
		var fidelityStr string
		var createdAtStr string

		err := rows.Scan(
			&m.ID, &m.ConversationID, &roleStr, &m.Content,
			&m.Importance, &fidelityStr, &m.OriginalContent,
			&m.TokenCount, &createdAtStr,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning message row: %w", err)
		}

		m.Role = MessageRole(roleStr)
		m.FidelityLevel = FidelityLevel(fidelityStr)
		m.CreatedAt, err = parseTime(createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("parsing created_at: %w", err)
		}

		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating message rows: %w", err)
	}
	return messages, nil
}

// parseTime tries multiple time formats to handle differences between
// SQLite CURRENT_TIMESTAMP output and the go-sqlite3 driver.
func parseTime(s string) (time.Time, error) {
	formats := []string{
		time.DateTime,
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time format: %q", s)
}