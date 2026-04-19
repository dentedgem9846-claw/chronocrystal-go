package memory

import (
	"testing"
)

func TestStoreAndGetBlueprint(t *testing.T) {
	store := openTestStore(t)

	bp := Blueprint{
		Name:         "deploy web app",
		Description:  "Deploy a web application to the server",
		Steps:        []BlueprintStep{{Tool: "shell", Input: "run deploy.sh", Output: "deploy ok", Success: true}},
		FitnessScore: 0.5,
		UseCount:     0,
	}

	id, err := store.StoreBlueprint(bp)
	if err != nil {
		t.Fatalf("StoreBlueprint: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero blueprint ID")
	}

	// Retrieve by keyword match.
	matches, err := store.GetMatchingBlueprints("deploy", 10)
	if err != nil {
		t.Fatalf("GetMatchingBlueprints: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Name != bp.Name {
		t.Errorf("Name = %q, want %q", matches[0].Name, bp.Name)
	}
	if matches[0].Description != bp.Description {
		t.Errorf("Description = %q, want %q", matches[0].Description, bp.Description)
	}
	if len(matches[0].Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(matches[0].Steps))
	}
	if matches[0].Steps[0].Tool != "shell" {
		t.Errorf("Step.Tool = %q, want %q", matches[0].Steps[0].Tool, "shell")
	}
	if matches[0].Steps[0].Success != true {
		t.Errorf("Step.Success = %v, want true", matches[0].Steps[0].Success)
	}
}

func TestFitnessUpdate(t *testing.T) {
	store := openTestStore(t)

	bp := Blueprint{
		Name:         "create repo",
		Description:  "Create a new git repository",
		Steps:        []BlueprintStep{{Tool: "shell", Input: "git init", Output: "initialized", Success: true}},
		FitnessScore: 0.5,
	}
	id, err := store.StoreBlueprint(bp)
	if err != nil {
		t.Fatalf("StoreBlueprint: %v", err)
	}

	// Increase fitness.
	if err := store.UpdateBlueprintFitness(id, 0.3); err != nil {
		t.Fatalf("UpdateBlueprintFitness: %v", err)
	}

	matches, err := store.GetMatchingBlueprints("repo", 10)
	if err != nil {
		t.Fatalf("GetMatchingBlueprints: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].FitnessScore != 0.8 {
		t.Errorf("FitnessScore = %f, want 0.8", matches[0].FitnessScore)
	}

	// Decrease fitness — should clamp to 0.
	if err := store.UpdateBlueprintFitness(id, -1.0); err != nil {
		t.Fatalf("UpdateBlueprintFitness (negative): %v", err)
	}

	matches, _ = store.GetMatchingBlueprints("repo", 10)
	if len(matches) == 1 && matches[0].FitnessScore != 0.0 {
		t.Errorf("FitnessScore = %f, want 0.0 (clamped)", matches[0].FitnessScore)
	}

	// Exceed 1.0 — should clamp.
	if err := store.UpdateBlueprintFitness(id, 2.0); err != nil {
		t.Fatalf("UpdateBlueprintFitness (over): %v", err)
	}

	matches, _ = store.GetMatchingBlueprints("repo", 10)
	if len(matches) == 1 && matches[0].FitnessScore != 1.0 {
		t.Errorf("FitnessScore = %f, want 1.0 (clamped)", matches[0].FitnessScore)
	}
}

func TestUseCountIncrement(t *testing.T) {
	store := openTestStore(t)

	bp := Blueprint{
		Name:         "run tests",
		Description:  "Run the project test suite",
		Steps:        []BlueprintStep{{Tool: "shell", Input: "go test ./...", Output: "PASS", Success: true}},
		FitnessScore: 0.6,
	}
	id, err := store.StoreBlueprint(bp)
	if err != nil {
		t.Fatalf("StoreBlueprint: %v", err)
	}

	// Increment twice.
	if err := store.IncrementBlueprintUse(id); err != nil {
		t.Fatalf("IncrementBlueprintUse: %v", err)
	}
	if err := store.IncrementBlueprintUse(id); err != nil {
		t.Fatalf("IncrementBlueprintUse (2nd): %v", err)
	}

	matches, err := store.GetMatchingBlueprints("tests", 10)
	if err != nil {
		t.Fatalf("GetMatchingBlueprints: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].UseCount != 2 {
		t.Errorf("UseCount = %d, want 2", matches[0].UseCount)
	}
}

func TestPruneBlueprints(t *testing.T) {
	store := openTestStore(t)

	lowBP := Blueprint{
		Name:         "low fitness",
		Description:  "A low fitness blueprint",
		Steps:        []BlueprintStep{{Tool: "shell", Input: "echo low", Output: "low", Success: true}},
		FitnessScore: 0.1,
	}
	highBP := Blueprint{
		Name:         "high fitness",
		Description:  "A high fitness blueprint",
		Steps:        []BlueprintStep{{Tool: "shell", Input: "echo high", Output: "high", Success: true}},
		FitnessScore: 0.8,
	}

	_, err := store.StoreBlueprint(lowBP)
	if err != nil {
		t.Fatalf("StoreBlueprint (low): %v", err)
	}
	_, err = store.StoreBlueprint(highBP)
	if err != nil {
		t.Fatalf("StoreBlueprint (high): %v", err)
	}

	// Prune below 0.3 — should remove the low-fitness one.
	if err := store.PruneBlueprints(0.3); err != nil {
		t.Fatalf("PruneBlueprints: %v", err)
	}

	// Only high-fitness should remain.
	matches, err := store.GetMatchingBlueprints("fitness", 10)
	if err != nil {
		t.Fatalf("GetMatchingBlueprints: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match after pruning, got %d", len(matches))
	}
	if matches[0].Name != "high fitness" {
		t.Errorf("remaining blueprint Name = %q, want %q", matches[0].Name, "high fitness")
	}
}

func TestKeywordMatching(t *testing.T) {
	store := openTestStore(t)

	bps := []Blueprint{
		{Name: "deploy web", Description: "Deploy a web application to production", FitnessScore: 0.7},
		{Name: "deploy database", Description: "Deploy a database migration", FitnessScore: 0.9},
		{Name: "run tests", Description: "Execute the test suite", FitnessScore: 0.5},
	}
	for _, bp := range bps {
		bp.Steps = []BlueprintStep{{Tool: "shell", Input: "cmd", Output: "ok", Success: true}}
		if _, err := store.StoreBlueprint(bp); err != nil {
			t.Fatalf("StoreBlueprint %q: %v", bp.Name, err)
		}
	}

	// Match by name word.
	matches, err := store.GetMatchingBlueprints("deploy", 10)
	if err != nil {
		t.Fatalf("GetMatchingBlueprints: %v", err)
	}
	if len(matches) != 2 {
		t.Errorf("expected 2 matches for 'deploy', got %d", len(matches))
	}

	// Ordered by fitness_score DESC, so database (0.9) should come before web (0.7).
	if len(matches) >= 2 {
		if matches[0].FitnessScore < matches[1].FitnessScore {
			t.Errorf("expected results ordered by fitness DESC, got %f then %f", matches[0].FitnessScore, matches[1].FitnessScore)
		}
	}

	// Match by description word.
	matches, err = store.GetMatchingBlueprints("migration", 10)
	if err != nil {
		t.Fatalf("GetMatchingBlueprints: %v", err)
	}
	if len(matches) != 1 || matches[0].Name != "deploy database" {
		t.Errorf("expected 'deploy database' match for 'migration', got %v", matches)
	}

	// No match.
	matches, err = store.GetMatchingBlueprints("nonexistent", 10)
	if err != nil {
		t.Fatalf("GetMatchingBlueprints: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for 'nonexistent', got %d", len(matches))
	}
}

func TestUpdateBlueprintFitnessNotFound(t *testing.T) {
	store := openTestStore(t)

	err := store.UpdateBlueprintFitness(99999, 0.1)
	if err == nil {
		t.Error("expected error for nonexistent blueprint ID")
	}
}

func TestIncrementBlueprintUseNotFound(t *testing.T) {
	store := openTestStore(t)

	err := store.IncrementBlueprintUse(99999)
	if err == nil {
		t.Error("expected error for nonexistent blueprint ID")
	}
}