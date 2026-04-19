package memory

import (
	"testing"

	"github.com/chronocrystal/chronocrystal-go/internal/config"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	cfg := config.MemoryConfig{
		DBPath:      ":memory:",
		AutoCommit:  false,
		LambdaDecay: 0.01,
	}
	store, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestOpenClose(t *testing.T) {
	store := openTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}
}

func TestMigrations(t *testing.T) {
	store := openTestStore(t)
	db := store.DB()

	tables := []string{
		"conversations",
		"messages",
		"learnings",
		"skills",
		"user_profiles",
		"blueprints",
		"schema_version",
	}

	for _, table := range tables {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found in schema: %v", table, err)
		}
	}
}

func TestSchemaVersion(t *testing.T) {
	store := openTestStore(t)
	db := store.DB()

	var version int
	err := db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&version)
	if err != nil {
		t.Fatalf("querying schema_version: %v", err)
	}
	if version != 2 {
		t.Errorf("expected schema version 2, got %d", version)
	}
}

func TestAutoCommit(t *testing.T) {
	cfg := config.MemoryConfig{
		DBPath:      ":memory:",
		AutoCommit:  true,
		LambdaDecay: 0.01,
	}
	store, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	defer store.Close()

	// AutoCommit should not panic. DoltLite-specific dolt_commit will fail
	// silently on plain SQLite, which is the expected behavior.
	store.AutoCommit("test commit")
}

func TestDoltLog(t *testing.T) {
	store := openTestStore(t)

	// dolt_log does not exist on plain SQLite; should return nil, nil.
	logs, err := store.DoltLog()
	if err != nil {
		t.Errorf("DoltLog() returned unexpected error: %v", err)
	}
	if logs != nil {
		t.Errorf("DoltLog() on plain SQLite: expected nil logs, got %v", logs)
	}
}