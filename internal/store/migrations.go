package store

import (
	"database/sql"
	"fmt"
)

type migration struct {
	version int
	sql     string
}

var migrations = []migration{
	{
		version: 1,
		sql: `
CREATE TABLE IF NOT EXISTS process_events (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT    NOT NULL,
    event         TEXT    NOT NULL,
    pid           INTEGER,
    exit_code     INTEGER,
    started_at    DATETIME,
    ended_at      DATETIME,
    restart_count INTEGER DEFAULT 0
);
`,
	},
}

func Migrate(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS meta (
    schema_version INTEGER NOT NULL
);
`)
	if err != nil {
		return fmt.Errorf("failed to create meta table: %w", err)
	}

	var currentVersion int
	err = db.QueryRow("SELECT schema_version FROM meta").Scan(&currentVersion)
	if err != nil {
		if err == sql.ErrNoRows {
			_, err = db.Exec("INSERT INTO meta (schema_version) VALUES (0)")
			if err != nil {
				return fmt.Errorf("failed to initialize schema_version: %w", err)
			}
			currentVersion = 0
		} else {
			return fmt.Errorf("failed to query schema_version: %w", err)
		}
	}

	for _, m := range migrations {
		if m.version > currentVersion {
			tx, err := db.Begin()
			if err != nil {
				return fmt.Errorf("failed to begin transaction for migration %d: %w", m.version, err)
			}

			if _, err := tx.Exec(m.sql); err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to execute migration %d: %w", m.version, err)
			}

			if _, err := tx.Exec("UPDATE meta SET schema_version = ?", m.version); err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to update schema_version to %d: %w", m.version, err)
			}

			if err := tx.Commit(); err != nil {
				return fmt.Errorf("failed to commit migration %d: %w", m.version, err)
			}
			currentVersion = m.version
		}
	}

	return nil
}
