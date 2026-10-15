package store

import (
	"database/sql"
	"errors"
	"time"
)

// John's editable harness: presets (persona + reply style), uploaded sales
// playbook documents, and a few global settings (FAQ, referral link, which
// preset is active). Demo resets never touch these tables.
const harnessSchema = `
CREATE TABLE IF NOT EXISTS presets (
  id INTEGER PRIMARY KEY,
  key TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  harness TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS documents (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  kind TEXT NOT NULL,
  text TEXT NOT NULL,
  bytes INTEGER NOT NULL DEFAULT 0,
  active INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);`

// Preset is a named version of John's persona and reply style. Key is set
// for the built-in presets so they can be reset to their defaults.
type Preset struct {
	ID          int64  `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Harness     string `json:"-"` // JSON, owned by the harness package
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// Document is an uploaded sales playbook file, stored as extracted text.
type Document struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // md | txt | pdf
	Text      string `json:"-"`
	Bytes     int64  `json:"bytes"`
	Active    bool   `json:"active"`
	CreatedAt int64  `json:"created_at"`
}

func (s *Store) Presets() ([]Preset, error) {
	rows, err := s.db.Query(`SELECT id, key, name, description, harness, created_at, updated_at FROM presets ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Preset
	for rows.Next() {
		var p Preset
		if err := rows.Scan(&p.ID, &p.Key, &p.Name, &p.Description, &p.Harness, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) Preset(id int64) (Preset, error) {
	var p Preset
	err := s.db.QueryRow(`SELECT id, key, name, description, harness, created_at, updated_at FROM presets WHERE id = ?`, id).
		Scan(&p.ID, &p.Key, &p.Name, &p.Description, &p.Harness, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

func (s *Store) AddPreset(p Preset) (int64, error) {
	now := time.Now().UnixMilli()
	res, err := s.db.Exec(`INSERT INTO presets (key, name, description, harness, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		p.Key, p.Name, p.Description, p.Harness, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdatePreset(p Preset) error {
	res, err := s.db.Exec(`UPDATE presets SET name = ?, description = ?, harness = ?, updated_at = ? WHERE id = ?`,
		p.Name, p.Description, p.Harness, time.Now().UnixMilli(), p.ID)
	return rowsOrNotFound(res, err)
}

func (s *Store) DeletePreset(id int64) error {
	return rowsOrNotFound(s.db.Exec(`DELETE FROM presets WHERE id = ?`, id))
}

func (s *Store) Documents() ([]Document, error) {
	rows, err := s.db.Query(`SELECT id, name, kind, text, bytes, active, created_at FROM documents ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Document
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.ID, &d.Name, &d.Kind, &d.Text, &d.Bytes, &d.Active, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) AddDocument(d Document) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO documents (name, kind, text, bytes, active, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		d.Name, d.Kind, d.Text, d.Bytes, d.Active, time.Now().UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) SetDocumentActive(id int64, active bool) error {
	return rowsOrNotFound(s.db.Exec(`UPDATE documents SET active = ? WHERE id = ?`, active, id))
}

func (s *Store) DeleteDocument(id int64) error {
	return rowsOrNotFound(s.db.Exec(`DELETE FROM documents WHERE id = ?`, id))
}

// Setting returns a saved setting and whether it exists.
func (s *Store) Setting(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return v, err == nil, err
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// ErrNotFound is returned when a row to read, update or delete doesn't exist.
var ErrNotFound = errors.New("not found")

func rowsOrNotFound(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
