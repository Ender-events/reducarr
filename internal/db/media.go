package db

import (
	"database/sql"
)

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

func (d *DB) GetMediaFile(instance string, fileID int32) (*MediaFileRecord, error) {
	var r MediaFileRecord
	err := d.QueryRow(`
		SELECT arr_instance, arr_type, item_id, file_id, path, title, inode, size, duration, quality, season_number
		FROM media_files WHERE arr_instance = ? AND file_id = ?`, instance, fileID).
		Scan(&r.ArrInstance, &r.ArrType, &r.ItemID, &r.FileID, &r.Path, &r.Title, &r.Inode, &r.Size, &r.Duration, &r.Quality, &r.SeasonNumber)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (d *DB) GetCandidate(instance string, fileID int32) (*CandidateRecord, error) {
	var r CandidateRecord
	err := d.QueryRow(`
		SELECT m.arr_instance, m.arr_type, m.item_id, m.file_id, m.path, m.title, m.inode, m.size, m.duration, m.quality, m.season_number, c.reason, c.is_ignored
		FROM candidates c
		JOIN media_files m ON c.arr_instance = m.arr_instance AND c.file_id = m.file_id
		WHERE c.arr_instance = ? AND c.file_id = ?`, instance, fileID).
		Scan(&r.ArrInstance, &r.ArrType, &r.ItemID, &r.FileID, &r.Path, &r.Title, &r.Inode, &r.Size, &r.Duration, &r.Quality, &r.SeasonNumber, &r.Reason, &r.IsIgnored)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (d *DB) SearchMediaFiles(query string, limit int) ([]MediaFileRecord, error) {
	q := "%" + query + "%"
	rows, err := d.Query(`
		SELECT arr_instance, arr_type, item_id, file_id, path, title, inode, size, duration, quality, season_number
		FROM media_files
		WHERE title LIKE ? OR path LIKE ?
		ORDER BY title ASC
		LIMIT ?`, q, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []MediaFileRecord
	for rows.Next() {
		var r MediaFileRecord
		if err := rows.Scan(&r.ArrInstance, &r.ArrType, &r.ItemID, &r.FileID, &r.Path, &r.Title, &r.Inode, &r.Size, &r.Duration, &r.Quality, &r.SeasonNumber); err != nil {
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
