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
		`CREATE INDEX IF NOT EXISTS idx_media_files_title ON media_files(title);`,
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
			warning_messages TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS users (
			username TEXT PRIMARY KEY,
			password TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS sessions (
			token TEXT PRIMARY KEY,
			username TEXT,
			expires_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (username) REFERENCES users (username) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS scan_locks (
			lock_name TEXT PRIMARY KEY,
			holder_pid INTEGER,
			holder_type TEXT,
			acquired_at DATETIME DEFAULT CURRENT_TIMESTAMP
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
	_, _ = d.Exec("ALTER TABLE reports ADD COLUMN warning_messages TEXT")

	return nil
}

type DashboardStats struct {
	TotalSpaceSaved   int64
	PendingCandidates int
	IgnoredFiles      int
	FailedActions     int
	LastScanTime      string
}

func (d *DB) GetDashboardStats() (DashboardStats, error) {
	var s DashboardStats

	// Total saved: sum(size_before - size_after) for UPGRADE, plus size_before for DELETE
	err := d.QueryRow(`
		SELECT COALESCE(SUM(
			CASE
				WHEN action_type = 'UPGRADE' THEN total_size_before - total_size_after
				WHEN action_type = 'DELETE' THEN total_size_before
				ELSE 0
			END), 0)
		FROM reports WHERE status = 'SUCCESS'
	`).Scan(&s.TotalSpaceSaved)
	if err != nil {
		return s, err
	}

	// Pending candidates
	err = d.QueryRow("SELECT COUNT(*) FROM candidates WHERE is_ignored = 0").Scan(&s.PendingCandidates)
	if err != nil {
		return s, err
	}

	// Ignored files
	err = d.QueryRow("SELECT COUNT(*) FROM candidates WHERE is_ignored = 1").Scan(&s.IgnoredFiles)
	if err != nil {
		return s, err
	}

	// Failed actions
	err = d.QueryRow("SELECT COUNT(*) FROM reports WHERE status = 'FAILED'").Scan(&s.FailedActions)
	if err != nil {
		return s, err
	}

	// Last scan time (max updated_at from scan_state)
	err = d.QueryRow("SELECT COALESCE(MAX(updated_at), 'Never') FROM scan_state").Scan(&s.LastScanTime)
	if err != nil {
		s.LastScanTime = "Never"
	}

	return s, nil
}

func (d *DB) GetLastItemID(instanceID string) (string, error) {
	var lastID string
	err := d.QueryRow("SELECT last_item_id FROM scan_state WHERE instance_id = ?", instanceID).Scan(&lastID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return lastID, err
}

type ScanState struct {
	InstanceID string
	LastItemID string
	UpdatedAt  string
}

func (d *DB) GetAllScanStates() ([]ScanState, error) {
	rows, err := d.Query("SELECT instance_id, last_item_id, updated_at FROM scan_state ORDER BY instance_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []ScanState
	for rows.Next() {
		var s ScanState
		if err := rows.Scan(&s.InstanceID, &s.LastItemID, &s.UpdatedAt); err != nil {
			return nil, err
		}
		states = append(states, s)
	}
	return states, nil
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

func (d *DB) AcquireScanLock(lockName string, pid int, holderType string, maxDurationSeconds int) (bool, error) {
	// Attempt to insert the lock
	_, err := d.Exec(`
		INSERT INTO scan_locks (lock_name, holder_pid, holder_type)
		VALUES (?, ?, ?)
	`, lockName, pid, holderType)
	if err == nil {
		return true, nil
	}

	// Lock insertion failed, let's see if the existing lock is too old (zombie lock)
	var ageSeconds int
	errRow := d.QueryRow(`
		SELECT (strftime('%s', 'now') - strftime('%s', acquired_at)) FROM scan_locks WHERE lock_name = ?
	`, lockName).Scan(&ageSeconds)
	if errRow == sql.ErrNoRows {
		// Try again just in case it was deleted in the millisecond between insert and query
		_, errRetry := d.Exec(`
			INSERT INTO scan_locks (lock_name, holder_pid, holder_type)
			VALUES (?, ?, ?)
		`, lockName, pid, holderType)
		if errRetry == nil {
			return true, nil
		}
		return false, fmt.Errorf("failed to acquire lock after retry: %w", errRetry)
	} else if errRow != nil {
		return false, fmt.Errorf("query scan lock: %w", errRow)
	}

	// If the lock is held longer than maxDurationSeconds, break it
	if ageSeconds > maxDurationSeconds {
		_, _ = d.Exec("DELETE FROM scan_locks WHERE lock_name = ?", lockName)
		return d.AcquireScanLock(lockName, pid, holderType, maxDurationSeconds)
	}

	return false, nil
}

func (d *DB) ReleaseScanLock(lockName string) error {
	_, err := d.Exec("DELETE FROM scan_locks WHERE lock_name = ?", lockName)
	return err
}
