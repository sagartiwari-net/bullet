package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"authchecker/internal/runner"
)

type DB struct {
	conn *sql.DB
}

func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	_, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS hits (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT NOT NULL,
			password TEXT NOT NULL,
			capture TEXT,
			config_name TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			config_name TEXT,
			wordlist TEXT,
			total INTEGER,
			hits INTEGER,
			started_at DATETIME,
			finished_at DATETIME
		);
	`)
	return err
}

func (db *DB) SaveHit(configName string, r runner.CheckResult) error {
	capture, _ := json.Marshal(r.Capture)
	_, err := db.conn.Exec(
		`INSERT INTO hits (email, password, capture, config_name) VALUES (?, ?, ?, ?)`,
		r.Email, r.Password, string(capture), configName,
	)
	return err
}

func (db *DB) ListHits(limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.conn.Query(
		`SELECT id, email, password, capture, config_name, created_at FROM hits ORDER BY id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var id int
		var email, password, capture, configName, createdAt string
		if err := rows.Scan(&id, &email, &password, &capture, &configName, &createdAt); err != nil {
			return nil, err
		}
		var cap map[string]string
		_ = json.Unmarshal([]byte(capture), &cap)
		out = append(out, map[string]interface{}{
			"id":          id,
			"email":       email,
			"password":    password,
			"capture":     cap,
			"config_name": configName,
			"created_at":  createdAt,
		})
	}
	return out, nil
}

func (db *DB) StartRun(configName, wordlist string, total int) (int64, error) {
	res, err := db.conn.Exec(
		`INSERT INTO runs (config_name, wordlist, total, hits, started_at) VALUES (?, ?, ?, 0, ?)`,
		configName, wordlist, total, time.Now().Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (db *DB) FinishRun(runID int64, hits int) error {
	_, err := db.conn.Exec(
		`UPDATE runs SET hits = ?, finished_at = ? WHERE id = ?`,
		hits, time.Now().Format(time.RFC3339), runID,
	)
	return err
}

func (db *DB) ExportHits(path string) error {
	rows, err := db.conn.Query(`SELECT email, password, capture FROM hits ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var lines []string
	for rows.Next() {
		var email, password, capture string
		if err := rows.Scan(&email, &password, &capture); err != nil {
			return err
		}
		lines = append(lines, fmt.Sprintf("%s:%s %s", email, password, capture))
	}

	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	return os.WriteFile(path, []byte(content), 0644)
}
