package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
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
	IsRead          bool
	CreatedAt       string
}

// ReportFilter defines query filters and sorting for reports.
type ReportFilter struct {
	Status    string
	Action    string
	SortBy    string // "date", "item", "saved"
	SortOrder string // "asc", "desc"
	Limit     int
	Offset    int
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
	isReadVal := 0
	if r.IsRead {
		isReadVal = 1
	}
	_, err := d.Exec(`
		INSERT INTO reports (
			action_type, arr_instance, arr_type, item_title, main_file_id, main_file_path,
			total_size_before, total_size_after, deleted_files, deleted_torrents,
			new_release_title, new_indexer, status, error_message, warning_messages, is_read
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, r.ActionType, r.ArrInstance, r.ArrType, r.ItemTitle, r.MainFileID, r.MainFilePath,
		r.TotalSizeBefore, r.TotalSizeAfter, r.DeletedFiles, r.DeletedTorrents,
		r.NewReleaseTitle, r.NewIndexer, r.Status, r.ErrorMessage, warningJSON, isReadVal)
	return err
}

func (d *DB) GetReports(limit, offset int) ([]ReportRecord, error) {
	return d.GetReportsFiltered("", limit, offset)
}

func (d *DB) GetReportsFiltered(status string, limit, offset int) ([]ReportRecord, error) {
	return d.GetReportsAdvanced(ReportFilter{
		Status: status,
		Limit:  limit,
		Offset: offset,
	})
}

func (d *DB) GetReportsAdvanced(filter ReportFilter) ([]ReportRecord, error) {
	query := `
		SELECT id, action_type, arr_instance, arr_type, item_title, main_file_id, main_file_path,
		       total_size_before, total_size_after, deleted_files, deleted_torrents,
		       new_release_title, new_indexer, status, error_message, warning_messages, is_read, created_at
		FROM reports
		WHERE 1=1
	`
	var args []any
	if filter.Status != "" && filter.Status != "ALL" {
		query += " AND status = ?"
		args = append(args, filter.Status)
	}
	if filter.Action != "" && filter.Action != "ALL" {
		query += " AND action_type = ?"
		args = append(args, filter.Action)
	}

	var orderClause string
	switch strings.ToLower(filter.SortBy) {
	case "date", "created_at":
		if strings.ToLower(filter.SortOrder) == "asc" {
			orderClause = "ORDER BY created_at ASC, id ASC"
		} else {
			orderClause = "ORDER BY created_at DESC, id DESC"
		}
	case "item", "title", "item_title":
		if strings.ToLower(filter.SortOrder) == "desc" {
			orderClause = "ORDER BY item_title DESC, id DESC"
		} else {
			orderClause = "ORDER BY item_title ASC, id ASC"
		}
	case "saved", "net_saved":
		netSavedExpr := `(
			CASE
				WHEN status IN ('SUCCESS', 'WARNING') THEN
					CASE
						WHEN action_type = 'UPGRADE' AND total_size_after > 0 THEN (total_size_before - total_size_after)
						WHEN action_type = 'DELETE' THEN total_size_before
						ELSE 0
					END
				ELSE 0
			END
		)`
		if strings.ToLower(filter.SortOrder) == "asc" {
			orderClause = fmt.Sprintf("ORDER BY %s ASC, id ASC", netSavedExpr)
		} else {
			orderClause = fmt.Sprintf("ORDER BY %s DESC, id DESC", netSavedExpr)
		}
	default:
		if strings.ToLower(filter.SortOrder) == "asc" {
			orderClause = "ORDER BY created_at ASC, id ASC"
		} else {
			orderClause = "ORDER BY created_at DESC, id DESC"
		}
	}
	query += " " + orderClause

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
		if filter.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, filter.Offset)
		}
	}

	rows, err := d.Query(query, args...)
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
		var isReadInt int
		if err := rows.Scan(
			&r.ID, &r.ActionType, &r.ArrInstance, &r.ArrType, &r.ItemTitle, &r.MainFileID, &r.MainFilePath,
			&r.TotalSizeBefore, &r.TotalSizeAfter, &r.DeletedFiles, &r.DeletedTorrents,
			&r.NewReleaseTitle, &r.NewIndexer, &r.Status, &r.ErrorMessage, &warningJSON, &isReadInt, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		r.IsRead = isReadInt == 1
		if warningJSON.Valid {
			_ = json.Unmarshal([]byte(warningJSON.String), &r.WarningMessages)
		}
		records = append(records, r)
	}
	return records, nil
}

func (d *DB) GetDistinctReportActions() ([]string, error) {
	rows, err := d.Query("SELECT DISTINCT action_type FROM reports WHERE action_type IS NOT NULL AND action_type != '' ORDER BY action_type ASC")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	var actions []string
	actionSet := make(map[string]bool)
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		if a != "" && !actionSet[a] {
			actions = append(actions, a)
			actionSet[a] = true
		}
	}
	for _, def := range []string{"UPGRADE", "DELETE"} {
		if !actionSet[def] {
			actions = append(actions, def)
			actionSet[def] = true
		}
	}
	sort.Strings(actions)
	return actions, nil
}

func (d *DB) GetReportByID(id int) (*ReportRecord, error) {
	var r ReportRecord
	var warningJSON sql.NullString
	var isReadInt int
	err := d.QueryRow(`
		SELECT id, action_type, arr_instance, arr_type, item_title, main_file_id, main_file_path,
		       total_size_before, total_size_after, deleted_files, deleted_torrents,
		       new_release_title, new_indexer, status, error_message, warning_messages, is_read, created_at
		FROM reports WHERE id = ?
	`, id).Scan(
		&r.ID, &r.ActionType, &r.ArrInstance, &r.ArrType, &r.ItemTitle, &r.MainFileID, &r.MainFilePath,
		&r.TotalSizeBefore, &r.TotalSizeAfter, &r.DeletedFiles, &r.DeletedTorrents,
		&r.NewReleaseTitle, &r.NewIndexer, &r.Status, &r.ErrorMessage, &warningJSON, &isReadInt, &r.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.IsRead = isReadInt == 1
	if warningJSON.Valid {
		_ = json.Unmarshal([]byte(warningJSON.String), &r.WarningMessages)
	}
	return &r, nil
}

func (d *DB) MarkReportAsRead(id int) error {
	_, err := d.Exec("UPDATE reports SET is_read = 1 WHERE id = ?", id)
	return err
}

func (d *DB) GetUnreadErrorsCount() (int, error) {
	var count int
	err := d.QueryRow("SELECT COUNT(*) FROM reports WHERE status = 'FAILED' AND (is_read = 0 OR is_read IS NULL)").Scan(&count)
	return count, err
}

func (d *DB) DeleteReport(id int) error {
	_, err := d.Exec("DELETE FROM reports WHERE id = ?", id)
	return err
}

func (d *DB) ClearReports() error {
	_, err := d.Exec("DELETE FROM reports")
	return err
}
