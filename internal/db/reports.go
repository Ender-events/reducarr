package db

import (
	"database/sql"
	"encoding/json"
)

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
	WarningMessages []string
	CreatedAt       string
}

func (d *DB) InsertReport(r ReportRecord) error {
	warningJSON := ""
	if len(r.WarningMessages) > 0 {
		var err error
		warningBytes, err := json.Marshal(r.WarningMessages)
		if err != nil {
			return err
		}
		warningJSON = string(warningBytes)
	}
	_, err := d.Exec(`
		INSERT INTO reports (
			action_type, arr_instance, arr_type, item_title, main_file_id, main_file_path,
			total_size_before, total_size_after, deleted_files, deleted_torrents,
			new_release_title, new_indexer, status, error_message, warning_messages
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, r.ActionType, r.ArrInstance, r.ArrType, r.ItemTitle, r.MainFileID, r.MainFilePath,
		r.TotalSizeBefore, r.TotalSizeAfter, r.DeletedFiles, r.DeletedTorrents,
		r.NewReleaseTitle, r.NewIndexer, r.Status, r.ErrorMessage, warningJSON)
	return err
}

func (d *DB) GetReports(limit, offset int) ([]ReportRecord, error) {
	rows, err := d.Query(`
		SELECT id, action_type, arr_instance, arr_type, item_title, main_file_id, main_file_path,
		       total_size_before, total_size_after, deleted_files, deleted_torrents,
		       new_release_title, new_indexer, status, error_message, warning_messages, created_at
		FROM reports
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	var records []ReportRecord
	for rows.Next() {
		var r ReportRecord
		var warningJSON sql.NullString
		if err := rows.Scan(
			&r.ID, &r.ActionType, &r.ArrInstance, &r.ArrType, &r.ItemTitle, &r.MainFileID, &r.MainFilePath,
			&r.TotalSizeBefore, &r.TotalSizeAfter, &r.DeletedFiles, &r.DeletedTorrents,
			&r.NewReleaseTitle, &r.NewIndexer, &r.Status, &r.ErrorMessage, &warningJSON, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		if warningJSON.Valid {
			_ = json.Unmarshal([]byte(warningJSON.String), &r.WarningMessages)
		}
		records = append(records, r)
	}
	return records, nil
}

func (d *DB) GetReportByID(id int) (*ReportRecord, error) {
	var r ReportRecord
	var warningJSON sql.NullString
	err := d.QueryRow(`
		SELECT id, action_type, arr_instance, arr_type, item_title, main_file_id, main_file_path,
		       total_size_before, total_size_after, deleted_files, deleted_torrents,
		       new_release_title, new_indexer, status, error_message, warning_messages, created_at
		FROM reports WHERE id = ?
	`, id).Scan(
		&r.ID, &r.ActionType, &r.ArrInstance, &r.ArrType, &r.ItemTitle, &r.MainFileID, &r.MainFilePath,
		&r.TotalSizeBefore, &r.TotalSizeAfter, &r.DeletedFiles, &r.DeletedTorrents,
		&r.NewReleaseTitle, &r.NewIndexer, &r.Status, &r.ErrorMessage, &warningJSON, &r.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if warningJSON.Valid {
		_ = json.Unmarshal([]byte(warningJSON.String), &r.WarningMessages)
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
