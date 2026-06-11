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
			season_number INTEGER,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (arr_instance, file_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_media_files_inode ON media_files(inode);`,
		`CREATE INDEX IF NOT EXISTS idx_media_files_path ON media_files(path);`,
		`CREATE TABLE IF NOT EXISTS candidates (
			arr_instance TEXT,
			file_id INTEGER,
			reason TEXT,
			is_ignored BOOLEAN DEFAULT 0,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (arr_instance, file_id),
			FOREIGN KEY (arr_instance, file_id) REFERENCES media_files (arr_instance, file_id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS reports (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action_type TEXT,
			arr_instance TEXT,
			arr_type TEXT,
			item_title TEXT,
			main_file_id INTEGER,
			main_file_path TEXT,
			total_size_before INTEGER,
			total_size_after INTEGER,
			deleted_files TEXT,
			deleted_torrents TEXT,
			new_release_title TEXT,
			new_indexer TEXT,
			status TEXT,
			error_message TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
	}

	for _, q := range queries {
		if _, err := d.Exec(q); err != nil {
			return fmt.Errorf("exec query %q: %w", q, err)
		}
	}

	// Dynamic migration for columns added later
	_, _ = d.Exec("ALTER TABLE torrents ADD COLUMN added_at INTEGER")
	_, _ = d.Exec("ALTER TABLE media_files ADD COLUMN season_number INTEGER")
	_, _ = d.Exec("ALTER TABLE candidates ADD COLUMN is_ignored BOOLEAN DEFAULT 0")

	return nil
}

type MediaFileRecord struct {
	ArrInstance  string
	ArrType      string
	ItemID       int32 // MovieID or SeriesID
	FileID       int32
	Path         string
	Title        string
	Inode        uint64
	Size         int64
	Duration     int64
	Quality      string
	SeasonNumber int32
}

func (d *DB) UpsertMediaFile(r MediaFileRecord) error {
	_, err := d.Exec(`
		INSERT INTO media_files (arr_instance, arr_type, item_id, file_id, path, title, inode, size, duration, quality, season_number, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(arr_instance, file_id) DO UPDATE SET
			item_id = excluded.item_id,
			path = excluded.path,
			title = excluded.title,
			inode = excluded.inode,
			size = excluded.size,
			duration = excluded.duration,
			quality = excluded.quality,
			season_number = excluded.season_number,
			updated_at = excluded.updated_at
	`, r.ArrInstance, r.ArrType, r.ItemID, r.FileID, r.Path, r.Title, r.Inode, r.Size, r.Duration, r.Quality, r.SeasonNumber)
	return err
}

func (d *DB) UpsertCandidate(arrInstance string, fileID int32, reason string) error {
	_, err := d.Exec(`
		INSERT INTO candidates (arr_instance, file_id, reason, is_ignored, updated_at)
		VALUES (?, ?, ?, 0, CURRENT_TIMESTAMP)
		ON CONFLICT(arr_instance, file_id) DO UPDATE SET
			reason = excluded.reason,
			updated_at = excluded.updated_at
	`, arrInstance, fileID, reason)
	return err
}

func (d *DB) SetIgnoreCandidate(arrInstance string, fileID int32, ignore bool) error {
	val := 0
	if ignore {
		val = 1
	}
	_, err := d.Exec("UPDATE candidates SET is_ignored = ? WHERE arr_instance = ? AND file_id = ?", val, arrInstance, fileID)
	return err
}

func (d *DB) IsCandidateIgnored(arrInstance string, fileID int32) bool {
	var ignored bool
	err := d.QueryRow("SELECT is_ignored FROM candidates WHERE arr_instance = ? AND file_id = ?", arrInstance, fileID).Scan(&ignored)
	if err != nil {
		return false
	}
	return ignored
}

type CandidateRecord struct {
	MediaFileRecord
	Reason    string
	IsIgnored bool
}

func (d *DB) GetCandidatesWithMedia() ([]CandidateRecord, error) {
	rows, err := d.Query(`
		SELECT m.arr_instance, m.arr_type, m.item_id, m.file_id, m.path, m.title, m.inode, m.size, m.duration, m.quality, m.season_number, c.reason, c.is_ignored
		FROM candidates c
		JOIN media_files m ON c.arr_instance = m.arr_instance AND c.file_id = m.file_id
		WHERE c.is_ignored = 0
		ORDER BY m.size DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []CandidateRecord
	for rows.Next() {
		var r CandidateRecord
		if err := rows.Scan(&r.ArrInstance, &r.ArrType, &r.ItemID, &r.FileID, &r.Path, &r.Title, &r.Inode, &r.Size, &r.Duration, &r.Quality, &r.SeasonNumber, &r.Reason, &r.IsIgnored); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}

func (d *DB) GetIgnoredCandidates() ([]CandidateRecord, error) {
	rows, err := d.Query(`
		SELECT m.arr_instance, m.arr_type, m.item_id, m.file_id, m.path, m.title, m.inode, m.size, m.duration, m.quality, m.season_number, c.reason, c.is_ignored
		FROM candidates c
		JOIN media_files m ON c.arr_instance = m.arr_instance AND c.file_id = m.file_id
		WHERE c.is_ignored = 1
		ORDER BY m.updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []CandidateRecord
	for rows.Next() {
		var r CandidateRecord
		if err := rows.Scan(&r.ArrInstance, &r.ArrType, &r.ItemID, &r.FileID, &r.Path, &r.Title, &r.Inode, &r.Size, &r.Duration, &r.Quality, &r.SeasonNumber, &r.Reason, &r.IsIgnored); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}

func (d *DB) RemoveIgnore(arrInstance string, fileID int32) error {
	_, err := d.Exec("UPDATE candidates SET is_ignored = 0 WHERE arr_instance = ? AND file_id = ?", arrInstance, fileID)
	return err
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
	Inode      uint64
	IsSeeding  bool
	AddedAt    int64
}

func (d *DB) GetTorrentsByInode(inode uint64) ([]TorrentRecord, error) {
	if inode == 0 {
		return nil, nil
	}
	rows, err := d.Query("SELECT client_name, info_hash, file_path, inode, is_seeding, added_at FROM torrents WHERE inode = ?", inode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []TorrentRecord
	for rows.Next() {
		var r TorrentRecord
		if err := rows.Scan(&r.ClientName, &r.InfoHash, &r.FilePath, &r.Inode, &r.IsSeeding, &r.AddedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}

func (d *DB) GetTorrentsByPath(path string) ([]TorrentRecord, error) {
	rows, err := d.Query("SELECT client_name, info_hash, file_path, inode, is_seeding, added_at FROM torrents WHERE file_path = ?", path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []TorrentRecord
	for rows.Next() {
		var r TorrentRecord
		if err := rows.Scan(&r.ClientName, &r.InfoHash, &r.FilePath, &r.Inode, &r.IsSeeding, &r.AddedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}

func (d *DB) GetTorrentsByHash(hash string) ([]TorrentRecord, error) {
	rows, err := d.Query("SELECT client_name, info_hash, file_path, inode, is_seeding, added_at FROM torrents WHERE info_hash = ?", hash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []TorrentRecord
	for rows.Next() {
		var r TorrentRecord
		if err := rows.Scan(&r.ClientName, &r.InfoHash, &r.FilePath, &r.Inode, &r.IsSeeding, &r.AddedAt); err != nil {
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
		SELECT arr_instance, arr_type, item_id, file_id, path, title, inode, size, duration, quality, season_number
		FROM media_files WHERE inode = ?`, inode).
		Scan(&r.ArrInstance, &r.ArrType, &r.ItemID, &r.FileID, &r.Path, &r.Title, &r.Inode, &r.Size, &r.Duration, &r.Quality, &r.SeasonNumber)
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
		SELECT arr_instance, arr_type, item_id, file_id, path, title, inode, size, duration, quality, season_number
		FROM media_files WHERE path = ?`, path).
		Scan(&r.ArrInstance, &r.ArrType, &r.ItemID, &r.FileID, &r.Path, &r.Title, &r.Inode, &r.Size, &r.Duration, &r.Quality, &r.SeasonNumber)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (d *DB) GetAllTorrents() ([]TorrentRecord, error) {
	rows, err := d.Query("SELECT client_name, info_hash, file_path, inode, is_seeding, added_at FROM torrents ORDER BY client_name, file_path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []TorrentRecord
	for rows.Next() {
		var r TorrentRecord
		if err := rows.Scan(&r.ClientName, &r.InfoHash, &r.FilePath, &r.Inode, &r.IsSeeding, &r.AddedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}

func (d *DB) DeleteMediaFile(arrInstance string, fileID int32) error {
	_, err := d.Exec("DELETE FROM media_files WHERE arr_instance = ? AND file_id = ?", arrInstance, fileID)
	return err
}

func (d *DB) DeleteTorrentByHash(clientName, hash string) error {
	_, err := d.Exec("DELETE FROM torrents WHERE client_name = ? AND info_hash = ?", clientName, hash)
	return err
}

type ReportRecord struct {
	ID              int
	ActionType      string
	ArrInstance     string
	ArrType         string
	ItemTitle       string
	MainFileID      int32
	MainFilePath    string
	TotalSizeBefore int64
	TotalSizeAfter  int64
	DeletedFiles    string // JSON
	DeletedTorrents string // JSON
	NewReleaseTitle string
	NewIndexer      string
	Status          string
	ErrorMessage    string
	CreatedAt       string
}

func (d *DB) InsertReport(r ReportRecord) error {
	_, err := d.Exec(`
		INSERT INTO reports (
			action_type, arr_instance, arr_type, item_title, main_file_id, main_file_path,
			total_size_before, total_size_after, deleted_files, deleted_torrents,
			new_release_title, new_indexer, status, error_message
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, r.ActionType, r.ArrInstance, r.ArrType, r.ItemTitle, r.MainFileID, r.MainFilePath,
		r.TotalSizeBefore, r.TotalSizeAfter, r.DeletedFiles, r.DeletedTorrents,
		r.NewReleaseTitle, r.NewIndexer, r.Status, r.ErrorMessage)
	return err
}

func (d *DB) GetReports(limit, offset int) ([]ReportRecord, error) {
	rows, err := d.Query(`
		SELECT id, action_type, arr_instance, arr_type, item_title, main_file_id, main_file_path,
		       total_size_before, total_size_after, deleted_files, deleted_torrents,
		       new_release_title, new_indexer, status, error_message, created_at
		FROM reports
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []ReportRecord
	for rows.Next() {
		var r ReportRecord
		if err := rows.Scan(
			&r.ID, &r.ActionType, &r.ArrInstance, &r.ArrType, &r.ItemTitle, &r.MainFileID, &r.MainFilePath,
			&r.TotalSizeBefore, &r.TotalSizeAfter, &r.DeletedFiles, &r.DeletedTorrents,
			&r.NewReleaseTitle, &r.NewIndexer, &r.Status, &r.ErrorMessage, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}

func (d *DB) GetReportByID(id int) (*ReportRecord, error) {
	var r ReportRecord
	err := d.QueryRow(`
		SELECT id, action_type, arr_instance, arr_type, item_title, main_file_id, main_file_path,
		       total_size_before, total_size_after, deleted_files, deleted_torrents,
		       new_release_title, new_indexer, status, error_message, created_at
		FROM reports WHERE id = ?
	`, id).Scan(
		&r.ID, &r.ActionType, &r.ArrInstance, &r.ArrType, &r.ItemTitle, &r.MainFileID, &r.MainFilePath,
		&r.TotalSizeBefore, &r.TotalSizeAfter, &r.DeletedFiles, &r.DeletedTorrents,
		&r.NewReleaseTitle, &r.NewIndexer, &r.Status, &r.ErrorMessage, &r.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (d *DB) DeleteReport(id int) error {
	_, err := d.Exec("DELETE FROM reports WHERE id = ?", id)
	return err
}

func (d *DB) ClearReports() error {
	_, err := d.Exec("DELETE FROM reports")
	return err
}
