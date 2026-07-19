package db

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
	defer func() { _ = rows.Close() }()

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
	defer func() { _ = rows.Close() }()

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
	defer func() { _ = rows.Close() }()

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

func (d *DB) GetAllTorrents() ([]TorrentRecord, error) {
	rows, err := d.Query("SELECT client_name, info_hash, file_path, inode, is_seeding, added_at FROM torrents ORDER BY client_name, file_path")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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

func (d *DB) DeleteTorrentByHash(clientName, hash string) error {
	_, err := d.Exec("DELETE FROM torrents WHERE client_name = ? AND info_hash = ?", clientName, hash)
	return err
}
