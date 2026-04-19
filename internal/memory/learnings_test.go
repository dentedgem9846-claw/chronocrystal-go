package memory

import (
	"path/filepath"
	"testing"

	"github.com/chronocrystal/chronocrystal-go/internal/config"
)

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cfg := config.MemoryConfig{
		DBPath:             dbPath,
		AutoCommit:         false,
		LearningDecayFactor: 0.95,
	}
	store, err := Open(cfg)
	if err != nil {
		t.Fatalf("opening test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestStoreAndGetLearning(t *testing.T) {
	store := setupTestStore(t)

	l := Learning{
		TaskType:       "code",
		Approach:       "direct implementation",
		Outcome:        "success",
		Lesson:         "always write tests first",
		RelevanceScore: 1.0,
	}

	if err := store.StoreLearning(l); err != nil {
		t.Fatalf("StoreLearning: %v", err)
	}

	results, err := store.GetRelevantLearnings("code", 5)
	if err != nil {
		t.Fatalf("GetRelevantLearnings: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 learning, got %d", len(results))
	}

	got := results[0]
	if got.TaskType != "code" {
		t.Errorf("TaskType = %q, want %q", got.TaskType, "code")
	}
	if got.Approach != "direct implementation" {
		t.Errorf("Approach = %q, want %q", got.Approach, "direct implementation")
	}
	if got.Outcome != "success" {
		t.Errorf("Outcome = %q, want %q", got.Outcome, "success")
	}
	if got.Lesson != "always write tests first" {
		t.Errorf("Lesson = %q, want %q", got.Lesson, "always write tests first")
	}
	if got.RelevanceScore != 1.0 {
		t.Errorf("RelevanceScore = %f, want 1.0", got.RelevanceScore)
	}
}

func TestRelevanceOrdering(t *testing.T) {
	store := setupTestStore(t)

	learnings := []Learning{
		{TaskType: "debug", Approach: "log analysis", Outcome: "success", Lesson: "check logs first", RelevanceScore: 0.5},
		{TaskType: "debug", Approach: "print debugging", Outcome: "partial", Lesson: "print statements help", RelevanceScore: 0.9},
		{TaskType: "debug", Approach: "debugger", Outcome: "failure", Lesson: "debugger didnt attach", RelevanceScore: 0.3},
	}

	for _, l := range learnings {
		if err := store.StoreLearning(l); err != nil {
			t.Fatalf("StoreLearning: %v", err)
		}
	}

	results, err := store.GetRelevantLearnings("debug", 10)
	if err != nil {
		t.Fatalf("GetRelevantLearnings: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 learnings, got %d", len(results))
	}

	// Should be ordered by relevance_score DESC.
	if results[0].RelevanceScore < results[1].RelevanceScore {
		t.Errorf("results not ordered by relevance: [%f, %f, %f]",
			results[0].RelevanceScore, results[1].RelevanceScore, results[2].RelevanceScore)
	}
	if results[0].Lesson != "print statements help" {
		t.Errorf("expected highest relevance lesson first, got %q", results[0].Lesson)
	}
}

func TestDecayScores(t *testing.T) {
	store := setupTestStore(t)

	l := Learning{
		TaskType:       "deploy",
		Approach:       "manual deploy",
		Outcome:        "success",
		Lesson:         "verify env vars",
		RelevanceScore: 1.0,
	}
	if err := store.StoreLearning(l); err != nil {
		t.Fatalf("StoreLearning: %v", err)
	}

	if err := store.DecayLearningScores(); err != nil {
		t.Fatalf("DecayLearningScores: %v", err)
	}

	results, err := store.GetRelevantLearnings("deploy", 5)
	if err != nil {
		t.Fatalf("GetRelevantLearnings: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 learning, got %d", len(results))
	}

	// 1.0 * 0.95 = 0.95
	if results[0].RelevanceScore < 0.94 || results[0].RelevanceScore > 0.96 {
		t.Errorf("expected relevance ~0.95 after decay, got %f", results[0].RelevanceScore)
	}
}

func TestGetRelevantLearningsLimit(t *testing.T) {
	store := setupTestStore(t)

	for i := 0; i < 10; i++ {
		l := Learning{
			TaskType:       "test",
			Approach:       "unit test",
			Outcome:        "success",
			Lesson:         "testing is good",
			RelevanceScore: float64(i) / 10.0,
		}
		if err := store.StoreLearning(l); err != nil {
			t.Fatalf("StoreLearning %d: %v", i, err)
		}
	}

	results, err := store.GetRelevantLearnings("test", 3)
	if err != nil {
		t.Fatalf("GetRelevantLearnings: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 learnings (limit), got %d", len(results))
	}
}

func TestGetRelevantLearningsNoMatch(t *testing.T) {
	store := setupTestStore(t)

	l := Learning{
		TaskType:       "code",
		Approach:       "TDD",
		Outcome:        "success",
		Lesson:         "write tests first",
		RelevanceScore: 1.0,
	}
	if err := store.StoreLearning(l); err != nil {
		t.Fatalf("StoreLearning: %v", err)
	}

	results, err := store.GetRelevantLearnings("deploy", 5)
	if err != nil {
		t.Fatalf("GetRelevantLearnings: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 learnings for non-matching task_type, got %d", len(results))
	}
}

func TestDecayPurgesLowScores(t *testing.T) {
	store := setupTestStore(t)

	l := Learning{
		TaskType:       "rare",
		Approach:       "niche",
		Outcome:        "success",
		Lesson:         "rare lesson",
		RelevanceScore: 0.15,
	}
	if err := store.StoreLearning(l); err != nil {
		t.Fatalf("StoreLearning: %v", err)
	}

	// 0.15 * 0.95 = 0.1425 — still above 0.1 threshold.
	if err := store.DecayLearningScores(); err != nil {
		t.Fatalf("DecayLearningScores: %v", err)
	}
	results, err := store.GetRelevantLearnings("rare", 5)
	if err != nil {
		t.Fatalf("GetRelevantLearnings: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected learning to survive one decay, got %d", len(results))
	}

	// 0.1425 * 0.95 ≈ 0.1354 — still above threshold.
	if err := store.DecayLearningScores(); err != nil {
		t.Fatalf("DecayLearningScores: %v", err)
	}

	// Manually push it below threshold for the test.
	_, err = store.db.Exec("UPDATE learnings SET relevance_score = 0.09 WHERE task_type = ?", "rare")
	if err != nil {
		t.Fatalf("manual score update: %v", err)
	}

	// Next decay should purge it.
	if err := store.DecayLearningScores(); err != nil {
		t.Fatalf("DecayLearningScores: %v", err)
	}
	results, err = store.GetRelevantLearnings("rare", 5)
	if err != nil {
		t.Fatalf("GetRelevantLearnings: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected learning to be purged, got %d", len(results))
	}
}