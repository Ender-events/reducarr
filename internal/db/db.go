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
		`CREATE TABLE IF NOT EXISTS torrents (
			client_name TEXT,
			info_hash TEXT,
			file_path TEXT,
			inode INTEGER,
			is_seeding BOOLEAN,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (client_name, info_hash, file_path)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_torrents_inode ON torrents(inode);`,
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

func (d *DB) Exec(query string, args ...any) (sql.Result, error) {
	return d.DB.Exec(query, args...)
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

type TorrentRecord struct {
	ClientName string
	InfoHash   string
	FilePath   string
	IsSeeding  bool
}

func (d *DB) GetTorrentsByInode(inode uint64) ([]TorrentRecord, error) {
	if inode == 0 {
		return nil, nil
	}
	rows, err := d.Query("SELECT client_name, info_hash, file_path, is_seeding FROM torrents WHERE inode = ?", inode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []TorrentRecord
	for rows.Next() {
		var r TorrentRecord
		if err := rows.Scan(&r.ClientName, &r.InfoHash, &r.FilePath, &r.IsSeeding); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}

func (d *DB) GetAllTorrents() ([]TorrentRecord, error) {
	rows, err := d.Query("SELECT client_name, info_hash, file_path, is_seeding FROM torrents ORDER BY client_name, file_path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []TorrentRecord
	for rows.Next() {
		var r TorrentRecord
		if err := rows.Scan(&r.ClientName, &r.InfoHash, &r.FilePath, &r.IsSeeding); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}
