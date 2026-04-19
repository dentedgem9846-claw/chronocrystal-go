package memory

import (
	"database/sql"
	"fmt"
	"strings"
)

const schemaVersion = 2

// Migrate runs database schema migrations.
func Migrate(db *sql.DB) error {
	if err := ensureSchemaVersionTable(db); err != nil {
		return fmt.Errorf("ensuring schema_version table: %w", err)
	}

	current, err := currentVersion(db)
	if err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}

	if current >= schemaVersion {
		return nil
	}

	if current < 1 {
		if err := applyV1(db); err != nil {
			return fmt.Errorf("applying v1 migration: %w", err)
		}
	}

	if current < 2 {
		if err := applyV2(db); err != nil {
			return fmt.Errorf("applying v2 migration: %w", err)
		}
	}

	// DoltLite-specific: commit the schema migration.
	// This will fail on plain SQLite, which is expected.
	_, _ = db.Exec("SELECT dolt_commit('-m', 'schema migration')")

	if err := setVersion(db, schemaVersion); err != nil {
		return fmt.Errorf("setting schema version: %w", err)
	}

	return nil
}

func ensureSchemaVersionTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_version (
			version   INTEGER PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("creating schema_version table: %w", err)
	}
	return nil
}

func currentVersion(db *sql.DB) (int, error) {
	var version int
	err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("querying max version: %w", err)
	}
	return version, nil
}

func setVersion(db *sql.DB, version int) error {
	_, err := db.Exec(
		"INSERT INTO schema_version (version, applied_at) VALUES (?, CURRENT_TIMESTAMP)",
		version,
	)
	if err != nil {
		return fmt.Errorf("inserting version %d: %w", version, err)
	}
	return nil
}

func applyV1(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS conversations (
			id           TEXT PRIMARY KEY,
			contact_id   TEXT NOT NULL,
			display_name TEXT NOT NULL,
			created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id TEXT NOT NULL REFERENCES conversations(id),
			role            TEXT NOT NULL,
			content         TEXT NOT NULL,
			importance      REAL NOT NULL DEFAULT 0.5,
			fidelity_level  TEXT NOT NULL DEFAULT 'full',
			original_content TEXT,
			token_count     INTEGER NOT NULL DEFAULT 0,
			created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS learnings (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			task_type   TEXT NOT NULL,
			description TEXT NOT NULL,
			created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS skills (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			name             TEXT NOT NULL UNIQUE,
			description      TEXT NOT NULL,
			trigger_keywords TEXT,
			content          TEXT NOT NULL,
			created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS user_profiles (
			contact_id   TEXT PRIMARY KEY,
			display_name TEXT NOT NULL,
			preferences  TEXT,
			updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_conv_time ON messages(conversation_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_learnings_task ON learnings(task_type)`,
	}

	for _, s := range statements {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("executing %q: %w", s, err)
		}
	}
	return nil
}

func applyV2(db *sql.DB) error {
	// Add new columns to learnings table. SQLite ALTER TABLE ADD COLUMN
	// fails if the column already exists, so we tolerate that error.
	alterStatements := []string{
		`ALTER TABLE learnings ADD COLUMN approach TEXT`,
		`ALTER TABLE learnings ADD COLUMN outcome TEXT`,
		`ALTER TABLE learnings ADD COLUMN lesson TEXT`,
		`ALTER TABLE learnings ADD COLUMN relevance_score REAL NOT NULL DEFAULT 1.0`,
	}

	for _, s := range alterStatements {
		if _, err := db.Exec(s); err != nil {
			if !isColumnExistsErr(err) {
				return fmt.Errorf("executing %q: %w", s, err)
			}
		}
	}

	// Backfill: migrate existing descriptions into the new lesson column,
	// set reasonable defaults for outcome and relevance.
	db.Exec(`UPDATE learnings SET lesson = description WHERE lesson IS NULL OR lesson = ''`)
	db.Exec(`UPDATE learnings SET outcome = 'success' WHERE outcome IS NULL OR outcome = ''`)

	// Create blueprints table for procedural memory.
	otherStatements := []string{
		`CREATE TABLE IF NOT EXISTS blueprints (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			name          TEXT NOT NULL,
			description   TEXT NOT NULL,
			steps         TEXT NOT NULL,
			fitness_score REAL NOT NULL DEFAULT 0.5,
			use_count     INTEGER NOT NULL DEFAULT 0,
			created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_blueprints_fitness ON blueprints(fitness_score)`,
	}

	for _, s := range otherStatements {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("executing %q: %w", s, err)
		}
	}

	// DoltLite commit for the schema change.
	_, _ = db.Exec("SELECT dolt_commit('-m', 'v2 migration: learnings expansion + blueprints table')")

	return nil
}

func isColumnExistsErr(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "duplicate column name") || strings.Contains(msg, "already exists")
}