package commands

import (
	"database/sql"
	"fmt"
	"strconv"

	"github.com/chronocrystal/chronocrystal-go/internal/memory"
)

// StoreAdapter wraps *memory.Store to implement the MemoryStore interface.
// It maps learnings → facts and summary-fidelity messages → recent summaries.
type StoreAdapter struct {
	store *memory.Store
}

// NewStoreAdapter creates a MemoryStore backed by the given memory store.
func NewStoreAdapter(store *memory.Store) *StoreAdapter {
	return &StoreAdapter{store: store}
}

// SearchFacts searches learnings whose lesson or description matches the query.
func (a *StoreAdapter) SearchFacts(query string, limit int) ([]Fact, error) {
	if limit < 1 {
		limit = 10
	}
	rows, err := a.store.DB().Query(
		`SELECT id, COALESCE(lesson, description)
		 FROM learnings
		 WHERE lesson LIKE ? OR description LIKE ?
		 ORDER BY relevance_score DESC
		 LIMIT ?`,
		"%"+query+"%", "%"+query+"%", limit,
	)
	if err != nil {
		return nil, fmt.Errorf("searching facts: %w", err)
	}
	defer rows.Close()

	return scanFacts(rows)
}

// RecentSummaries returns the most recent summary-fidelity messages.
func (a *StoreAdapter) RecentSummaries(n int) ([]Summary, error) {
	if n < 1 {
		n = 5
	}
	rows, err := a.store.DB().Query(
		`SELECT content FROM messages
		 WHERE fidelity_level = 'summary'
		 ORDER BY created_at DESC
		 LIMIT ?`,
		n,
	)
	if err != nil {
		return nil, fmt.Errorf("querying recent summaries: %w", err)
	}
	defer rows.Close()

	var summaries []Summary
	for rows.Next() {
		var s Summary
		if err := rows.Scan(&s.Content); err != nil {
			return nil, fmt.Errorf("scanning summary: %w", err)
		}
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}

// StoreFact persists a new fact as a learning record.
func (a *StoreAdapter) StoreFact(note string) (string, error) {
	result, err := a.store.DB().Exec(
		`INSERT INTO learnings (task_type, description, lesson, relevance_score) VALUES ('fact', ?, ?, 1.0)`,
		note, note,
	)
	if err != nil {
		return "", fmt.Errorf("storing fact: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return "", fmt.Errorf("getting fact id: %w", err)
	}
	a.store.AutoCommit(fmt.Sprintf("stored fact via memory command"))
	return strconv.FormatInt(id, 10), nil
}

// ListFacts returns all learnings as facts.
func (a *StoreAdapter) ListFacts() ([]Fact, error) {
	rows, err := a.store.DB().Query(
		`SELECT id, COALESCE(lesson, description) FROM learnings ORDER BY id`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing facts: %w", err)
	}
	defer rows.Close()

	return scanFacts(rows)
}

// ForgetFact deletes a learning by ID.
func (a *StoreAdapter) ForgetFact(id string) error {
	result, err := a.store.DB().Exec(`DELETE FROM learnings WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("forgetting fact %s: %w", id, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("fact %s not found", id)
	}
	a.store.AutoCommit(fmt.Sprintf("forgot fact %s via memory command", id))
	return nil
}

func scanFacts(rows *sql.Rows) ([]Fact, error) {
	var facts []Fact
	for rows.Next() {
		var f Fact
		var id int64
		if err := rows.Scan(&id, &f.Content); err != nil {
			return nil, fmt.Errorf("scanning fact: %w", err)
		}
		f.ID = strconv.FormatInt(id, 10)
		facts = append(facts, f)
	}
	return facts, rows.Err()
}