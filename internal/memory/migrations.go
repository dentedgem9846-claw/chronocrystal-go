package memory

import (
	"database/sql"
	"fmt"
)

const schemaVersion = 1

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

	if err := applyV1(db); err != nil {
		return fmt.Errorf("applying v1 migration: %w", err)
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