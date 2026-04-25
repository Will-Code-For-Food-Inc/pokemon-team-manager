// Package db manages the SQLite connection and schema migrations for ptm.
//
// It uses modernc.org/sqlite (pure Go, no CGO) and supports FTS5 for
// full-text search over the knowledge base. Vector similarity is computed
// in-process over stored BLOB embeddings.
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite" // register "sqlite" driver
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open opens (or creates) the SQLite database at the given path, enables WAL
// mode and foreign keys, then runs any pending migrations. The parent directory
// is created automatically if it does not exist.
func Open(path string) (*sql.DB, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("db.Open mkdir: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, fmt.Errorf("db.Open: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer
	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("db.Open migrations: %w", err)
	}
	return db, nil
}

// OpenMemory opens a fresh in-memory SQLite database and runs all migrations.
// Useful for tests.
func OpenMemory() (*sql.DB, error) {
	return Open(":memory:")
}

func runMigrations(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return err
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return err
	}

	// Sort by filename so migrations apply in order.
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		version := strings.TrimSuffix(name, ".sql")
		var exists int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		data, err := fs.ReadFile(migrationsFS, filepath.Join("migrations", name))
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(data)); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations(version) VALUES(?)`, version); err != nil {
			return err
		}
	}
	return nil
}
