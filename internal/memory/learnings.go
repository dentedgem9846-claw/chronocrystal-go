package memory

import (
	"database/sql"
	"fmt"
	"time"
)

// Learning captures a lesson extracted from a completed task.
type Learning struct {
	ID             int64
	TaskType       string
	Approach       string
	Outcome        string
	Lesson         string
	RelevanceScore float64
	CreatedAt      time.Time
}

// StoreLearning inserts a new learning record.
func (s *Store) StoreLearning(l Learning) error {
	if l.RelevanceScore <= 0 {
		l.RelevanceScore = 1.0
	}

	_, err := s.db.Exec(
		`INSERT INTO learnings (task_type, description, approach, outcome, lesson, relevance_score, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		l.TaskType, l.Lesson, l.Approach, l.Outcome, l.Lesson, l.RelevanceScore, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("inserting learning for task_type %s: %w", l.TaskType, err)
	}

	s.AutoCommit(fmt.Sprintf("learning extracted for task_type: %s", l.TaskType))
	return nil
}

// GetRelevantLearnings returns learnings matching taskType, ordered by
// relevance_score descending, limited to n results.
func (s *Store) GetRelevantLearnings(taskType string, limit int) ([]Learning, error) {
	if limit < 1 {
		limit = 5
	}

	rows, err := s.db.Query(
		`SELECT id, task_type, COALESCE(approach, ''), COALESCE(outcome, ''),
		        COALESCE(lesson, ''), COALESCE(relevance_score, 1.0), created_at
		 FROM learnings
		 WHERE task_type = ?
		 ORDER BY relevance_score DESC
		 LIMIT ?`,
		taskType, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("querying learnings for task_type %s: %w", taskType, err)
	}
	defer rows.Close()

	return scanLearnings(rows)
}

// DecayLearningScores multiplies all learning relevance scores by the configured
// decay factor, reducing their influence over time.
func (s *Store) DecayLearningScores() error {
	factor := s.cfg.LearningDecayFactor
	if factor <= 0 || factor >= 1 {
		factor = 0.95
	}

	_, err := s.db.Exec(
		`UPDATE learnings SET relevance_score = relevance_score * ?`,
		factor,
	)
	if err != nil {
		return fmt.Errorf("decaying learning scores: %w", err)
	}

	// Purge learnings that have decayed below threshold.
	_, err = s.db.Exec(`DELETE FROM learnings WHERE relevance_score < 0.1`)
	if err != nil {
		return fmt.Errorf("purging decayed learnings: %w", err)
	}

	s.AutoCommit("decayed learning scores")
	return nil
}

func scanLearnings(rows *sql.Rows) ([]Learning, error) {
	var learnings []Learning
	for rows.Next() {
		var l Learning
		var createdAtStr string
		err := rows.Scan(
			&l.ID, &l.TaskType, &l.Approach, &l.Outcome,
			&l.Lesson, &l.RelevanceScore, &createdAtStr,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning learning row: %w", err)
		}

		l.CreatedAt, err = parseTime(createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("parsing created_at for learning id %d: %w", l.ID, err)
		}

		learnings = append(learnings, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating learning rows: %w", err)
	}
	return learnings, nil
}