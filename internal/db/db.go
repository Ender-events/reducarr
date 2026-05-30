package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	d := &DB{db}
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	return d, nil
}

func (d *DB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS scan_state (
			instance_id TEXT PRIMARY KEY,
			last_item_id TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
	}

	for _, q := range queries {
		if _, err := d.Exec(q); err != nil {
			return fmt.Errorf("exec query %q: %w", q, err)
		}
	}

	return nil
}

func (d *DB) GetLastItemID(instanceID string) (string, error) {
	var lastID string
	err := d.QueryRow("SELECT last_item_id FROM scan_state WHERE instance_id = ?", instanceID).Scan(&lastID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return lastID, err
}

func (d *DB) SetLastItemID(instanceID, lastID string) error {
	_, err := d.Exec(`
		INSERT INTO scan_state (instance_id, last_item_id, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(instance_id) DO UPDATE SET
			last_item_id = excluded.last_item_id,
			updated_at = excluded.updated_at
	`, instanceID, lastID)
	return err
}
