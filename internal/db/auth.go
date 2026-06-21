package db

import (
	"database/sql"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type UserRecord struct {
	Username  string
	Password  string
	UpdatedAt string
}

func (d *DB) GetUser(username string) (string, error) {
	var password string
	err := d.QueryRow("SELECT password FROM users WHERE username = ?", username).Scan(&password)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return password, err
}

func (d *DB) GetAllUsers() ([]UserRecord, error) {
	rows, err := d.Query("SELECT username, password, updated_at FROM users ORDER BY username")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []UserRecord
	for rows.Next() {
		var r UserRecord
		if err := rows.Scan(&r.Username, &r.Password, &r.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}

func (d *DB) DeleteUser(username string) error {
	_, err := d.Exec("DELETE FROM users WHERE username = ?", username)
	return err
}

func (d *DB) GetFirstUser() (string, string, error) {
	var username, password string
	err := d.QueryRow("SELECT username, password FROM users LIMIT 1").Scan(&username, &password)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	return username, password, err
}

func (d *DB) UpsertUser(username, password string) error {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	_, err = d.Exec(`
		INSERT INTO users (username, password, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(username) DO UPDATE SET
			password = excluded.password,
			updated_at = excluded.updated_at
	`, username, string(hashed))
	return err
}

func (d *DB) AuthenticateUser(username, password string) (bool, error) {
	var hashed string
	err := d.QueryRow("SELECT password FROM users WHERE username = ?", username).Scan(&hashed)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	err = bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password))
	if err == bcrypt.ErrMismatchedHashAndPassword {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

func (d *DB) CreateSession(token, username string, expiresAt time.Time) error {
	_, err := d.Exec("INSERT INTO sessions (token, username, expires_at) VALUES (?, ?, ?)", token, username, expiresAt)
	return err
}

func (d *DB) GetSession(token string) (string, error) {
	var username string
	var expiresAt time.Time
	err := d.QueryRow("SELECT username, expires_at FROM sessions WHERE token = ?", token).Scan(&username, &expiresAt)
	if err != nil {
		return "", err
	}
	if time.Now().After(expiresAt) {
		_ = d.DeleteSession(token)
		return "", fmt.Errorf("session expired")
	}
	return username, nil
}

func (d *DB) DeleteSession(token string) error {
	_, err := d.Exec("DELETE FROM sessions WHERE token = ?", token)
	return err
}
