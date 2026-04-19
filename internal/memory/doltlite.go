package memory

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"

	"github.com/chronocrystal/chronocrystal-go/internal/config"
)

// Store wraps a database/sql connection with DoltLite-aware operations.
type Store struct {
	db  *sql.DB
	cfg config.MemoryConfig
}

// Open creates a new Store, opens the database, and runs migrations.
func Open(cfg config.MemoryConfig) (*Store, error) {
	db, err := sql.Open("sqlite3", cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("opening database %s: %w", cfg.DBPath, err)
	}

	// SQLite pragmas for reliability.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting journal_mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting foreign_keys: %w", err)
	}

	if err := Migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return &Store{db: db, cfg: cfg}, nil
}

// Close releases the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB for direct queries.
func (s *Store) DB() *sql.DB {
	return s.db
}

// AutoCommit performs a DoltLite commit if auto_commit is enabled.
// The message is used as the commit message.
// Errors are ignored since dolt_commit is DoltLite-specific and
// will fail silently on plain SQLite.
func (s *Store) AutoCommit(message string) {
	if !s.cfg.AutoCommit {
		return
	}
	_, _ = s.db.Exec("SELECT dolt_commit('-m', ?)", message)
}

// DoltLog returns commit history from DoltLite.
// Returns an empty slice on plain SQLite (dolt_log does not exist).
func (s *Store) DoltLog() ([]string, error) {
	rows, err := s.db.Query("SELECT message FROM dolt_log ORDER BY commit_date DESC")
	if err != nil {
		// dolt_log doesn't exist on plain SQLite.
		return nil, nil
	}
	defer rows.Close()

	var logs []string
	for rows.Next() {
		var msg string
		if err := rows.Scan(&msg); err != nil {
			return nil, fmt.Errorf("scanning dolt_log: %w", err)
		}
		logs = append(logs, msg)
	}
	return logs, rows.Err()
}
