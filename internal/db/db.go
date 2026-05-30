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
			added_at INTEGER,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (client_name, info_hash, file_path)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_torrents_inode ON torrents(inode);`,
		`CREATE TABLE IF NOT EXISTS media_files (
			arr_instance TEXT,
			arr_type TEXT,
			item_id INTEGER,
			file_id INTEGER,
			path TEXT,
			title TEXT,
			inode INTEGER,
			size INTEGER,
			duration INTEGER,
			quality TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (arr_instance, file_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_media_files_inode ON media_files(inode);`,
		`CREATE INDEX IF NOT EXISTS idx_media_files_path ON media_files(path);`,
		`CREATE TABLE IF NOT EXISTS candidates (
			arr_instance TEXT,
			file_id INTEGER,
			reason TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (arr_instance, file_id),
			FOREIGN KEY (arr_instance, file_id) REFERENCES media_files (arr_instance, file_id) ON DELETE CASCADE
		);`,
	}

	for _, q := range queries {
		if _, err := d.Exec(q); err != nil {
			return fmt.Errorf("exec query %q: %w", q, err)
		}
	}

	// Dynamic migration for added_at if table already exists
	_, _ = d.Exec("ALTER TABLE torrents ADD COLUMN added_at INTEGER")

	return nil
}

type MediaFileRecord struct {
	ArrInstance string
	ArrType     string
	ItemID      int32
	FileID      int32
	Path        string
	Title       string
	Inode       uint64
	Size        int64
	Duration    int64
	Quality     string
}

func (d *DB) UpsertMediaFile(r MediaFileRecord) error {
	_, err := d.Exec(`
		INSERT INTO media_files (arr_instance, arr_type, item_id, file_id, path, title, inode, size, duration, quality, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(arr_instance, file_id) DO UPDATE SET
			item_id = excluded.item_id,
			path = excluded.path,
			title = excluded.title,
			inode = excluded.inode,
			size = excluded.size,
			duration = excluded.duration,
			quality = excluded.quality,
			updated_at = excluded.updated_at
	`, r.ArrInstance, r.ArrType, r.ItemID, r.FileID, r.Path, r.Title, r.Inode, r.Size, r.Duration, r.Quality)
	return err
}

func (d *DB) UpsertCandidate(arrInstance string, fileID int32, reason string) error {
	_, err := d.Exec(`
		INSERT INTO candidates (arr_instance, file_id, reason, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(arr_instance, file_id) DO UPDATE SET
			reason = excluded.reason,
			updated_at = excluded.updated_at
	`, arrInstance, fileID, reason)
	return err
}

type CandidateRecord struct {
	MediaFileRecord
	Reason string
}

func (d *DB) GetCandidatesWithMedia() ([]CandidateRecord, error) {
	rows, err := d.Query(`
		SELECT m.arr_instance, m.arr_type, m.item_id, m.file_id, m.path, m.title, m.inode, m.size, m.duration, m.quality, c.reason
		FROM candidates c
		JOIN media_files m ON c.arr_instance = m.arr_instance AND c.file_id = m.file_id
		ORDER BY m.size DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []CandidateRecord
	for rows.Next() {
		var r CandidateRecord
		if err := rows.Scan(&r.ArrInstance, &r.ArrType, &r.ItemID, &r.FileID, &r.Path, &r.Title, &r.Inode, &r.Size, &r.Duration, &r.Quality, &r.Reason); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
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
	AddedAt    int64
}

func (d *DB) GetTorrentsByInode(inode uint64) ([]TorrentRecord, error) {
	if inode == 0 {
		return nil, nil
	}
	rows, err := d.Query("SELECT client_name, info_hash, file_path, is_seeding, added_at FROM torrents WHERE inode = ?", inode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []TorrentRecord
	for rows.Next() {
		var r TorrentRecord
		if err := rows.Scan(&r.ClientName, &r.InfoHash, &r.FilePath, &r.IsSeeding, &r.AddedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}

func (d *DB) GetMediaFileByInode(inode uint64) (*MediaFileRecord, error) {
	if inode == 0 {
		return nil, nil
	}
	var r MediaFileRecord
	err := d.QueryRow(`
		SELECT arr_instance, arr_type, item_id, file_id, path, title, inode, size, duration, quality
		FROM media_files WHERE inode = ?`, inode).
		Scan(&r.ArrInstance, &r.ArrType, &r.ItemID, &r.FileID, &r.Path, &r.Title, &r.Inode, &r.Size, &r.Duration, &r.Quality)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (d *DB) GetMediaFileByPath(path string) (*MediaFileRecord, error) {
	var r MediaFileRecord
	err := d.QueryRow(`
		SELECT arr_instance, arr_type, item_id, file_id, path, title, inode, size, duration, quality
		FROM media_files WHERE path = ?`, path).
		Scan(&r.ArrInstance, &r.ArrType, &r.ItemID, &r.FileID, &r.Path, &r.Title, &r.Inode, &r.Size, &r.Duration, &r.Quality)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (d *DB) GetAllTorrents() ([]TorrentRecord, error) {
	rows, err := d.Query("SELECT client_name, info_hash, file_path, is_seeding, added_at FROM torrents ORDER BY client_name, file_path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []TorrentRecord
	for rows.Next() {
		var r TorrentRecord
		if err := rows.Scan(&r.ClientName, &r.InfoHash, &r.FilePath, &r.IsSeeding, &r.AddedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}
