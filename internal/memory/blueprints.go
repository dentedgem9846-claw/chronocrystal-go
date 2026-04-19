package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Blueprint represents a reusable task procedure captured after successful multi-step orders.
type Blueprint struct {
	ID           int64
	Name         string
	Description  string
	Steps        []BlueprintStep
	FitnessScore float64
	UseCount     int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// BlueprintStep is one step in a blueprint procedure.
type BlueprintStep struct {
	Tool    string `json:"tool"`
	Input   string `json:"input"`
	Output  string `json:"output"`
	Success bool   `json:"success"`
}

// StoreBlueprint inserts a new blueprint into the database.
func (s *Store) StoreBlueprint(bp Blueprint) (int64, error) {
	stepsJSON, err := json.Marshal(bp.Steps)
	if err != nil {
		return 0, fmt.Errorf("marshaling blueprint steps: %w", err)
	}

	result, err := s.db.Exec(
		`INSERT INTO blueprints (name, description, steps, fitness_score, use_count)
		 VALUES (?, ?, ?, ?, ?)`,
		bp.Name, bp.Description, string(stepsJSON), bp.FitnessScore, bp.UseCount,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting blueprint: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting blueprint last insert id: %w", err)
	}

	s.AutoCommit(fmt.Sprintf("blueprint created: %s", bp.Name))
	return id, nil
}

// GetMatchingBlueprints returns blueprints whose name or description contains
// any word from the query, ordered by fitness_score descending.
func (s *Store) GetMatchingBlueprints(query string, limit int) ([]Blueprint, error) {
	words := tokenizeForMatch(query)
	if len(words) == 0 {
		return nil, nil
	}

	// Build WHERE clause: match if any word appears in name or description.
	placeholders := make([]string, 0, len(words)*2)
	args := make([]any, 0, len(words)*2+1)
	for _, w := range words {
		placeholders = append(placeholders, "name LIKE ?", "description LIKE ?")
		args = append(args, "%"+w+"%", "%"+w+"%")
	}
	whereClause := strings.Join(placeholders, " OR ")

	// Cap limit.
	if limit <= 0 {
		limit = 10
	}
	args = append(args, limit)

	rows, err := s.db.Query(
		`SELECT id, name, description, steps, fitness_score, use_count, created_at, updated_at
		 FROM blueprints
		 WHERE `+whereClause+`
		 ORDER BY fitness_score DESC
		 LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("querying matching blueprints: %w", err)
	}
	defer rows.Close()

	return scanBlueprints(rows)
}

// UpdateBlueprintFitness adjusts a blueprint's fitness score by delta.
// The score is clamped to [0.0, 1.0].
func (s *Store) UpdateBlueprintFitness(id int64, delta float64) error {
	result, err := s.db.Exec(
		`UPDATE blueprints
		 SET fitness_score = MAX(0.0, MIN(1.0, fitness_score + ?)),
		     updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		delta, id,
	)
	if err != nil {
		return fmt.Errorf("updating blueprint fitness for id %d: %w", id, err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("blueprint id %d not found", id)
	}

	s.AutoCommit(fmt.Sprintf("blueprint %d fitness updated", id))
	return nil
}

// IncrementBlueprintUse increments the use_count of a blueprint by 1.
func (s *Store) IncrementBlueprintUse(id int64) error {
	result, err := s.db.Exec(
		`UPDATE blueprints
		 SET use_count = use_count + 1,
		     updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		id,
	)
	if err != nil {
		return fmt.Errorf("incrementing blueprint use for id %d: %w", id, err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("blueprint id %d not found", id)
	}

	s.AutoCommit(fmt.Sprintf("blueprint %d used", id))
	return nil
}

// PruneBlueprints removes all blueprints with a fitness score below the threshold.
func (s *Store) PruneBlueprints(threshold float64) error {
	result, err := s.db.Exec(
		`DELETE FROM blueprints WHERE fitness_score < ?`,
		threshold,
	)
	if err != nil {
		return fmt.Errorf("pruning blueprints below threshold %f: %w", threshold, err)
	}

	affected, _ := result.RowsAffected()
	if affected > 0 {
		s.AutoCommit(fmt.Sprintf("pruned %d blueprint(s) below fitness %f", affected, threshold))
	}
	return nil
}

// scanBlueprints reads all rows from a blueprint query result.
func scanBlueprints(rows *sql.Rows) ([]Blueprint, error) {
	var blueprints []Blueprint
	for rows.Next() {
		var bp Blueprint
		var stepsJSON string
		var createdAtStr string
		var updatedAtStr string

		err := rows.Scan(
			&bp.ID, &bp.Name, &bp.Description, &stepsJSON,
			&bp.FitnessScore, &bp.UseCount, &createdAtStr, &updatedAtStr,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning blueprint row: %w", err)
		}

		if err := json.Unmarshal([]byte(stepsJSON), &bp.Steps); err != nil {
			return nil, fmt.Errorf("unmarshaling steps for blueprint %d: %w", bp.ID, err)
		}

		bp.CreatedAt, err = parseTime(createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("parsing created_at for blueprint %d: %w", bp.ID, err)
		}

		bp.UpdatedAt, err = parseTime(updatedAtStr)
		if err != nil {
			return nil, fmt.Errorf("parsing updated_at for blueprint %d: %w", bp.ID, err)
		}

		blueprints = append(blueprints, bp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating blueprint rows: %w", err)
	}
	return blueprints, nil
}

// tokenizeForMatch splits a string into lowercase words for keyword matching.
func tokenizeForMatch(s string) []string {
	s = strings.ToLower(s)
	fields := strings.Fields(s)
	seen := make(map[string]bool, len(fields))
	var result []string
	for _, f := range fields {
		if !seen[f] {
			seen[f] = true
			result = append(result, f)
		}
	}
	return result
}