// Package store is the app's SQLite store (data/ureach.db): contacts,
// messages, reply runs (per-stage timings for the Stage view) and events.
// It is separate from the whatsmeow session DB so re-seeding never unlinks.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Contact statuses, shown as chips in the Inbox.
const (
	StatusNew       = "new"        // no reply yet
	StatusAuto      = "auto"       // the AI is handling it
	StatusNeedsJohn = "needs_john" // the AI handed off; it stays quiet
	StatusHuman     = "human"      // John took over; the AI stays quiet
)

type Contact struct {
	ID            int64    `json:"id"`
	JID           string   `json:"jid"`  // phone JID when known, else LID; the identity key
	Chat          string   `json:"chat"` // where to reply
	Phone         string   `json:"phone"`
	Name          string   `json:"name"` // what John calls them; wins over push name
	PushName      string   `json:"push_name"`
	Context       string   `json:"context"`
	Memory        []string `json:"memory"`
	Status        string   `json:"status"`
	HandoffReason string   `json:"handoff_reason"`
	LinkSentAt    int64    `json:"link_sent_at"`
	CreatedAt     int64    `json:"created_at"`
	LastMessageAt int64    `json:"last_message_at"`
}

// DisplayName is the name to show and say.
func (c Contact) DisplayName() string {
	if c.Name != "" {
		return c.Name
	}
	return c.PushName
}

type Message struct {
	ID         int64  `json:"id"`
	ContactID  int64  `json:"contact_id"`
	WAID       string `json:"wa_id,omitempty"`
	Direction  string `json:"direction"` // in | out
	Kind       string `json:"kind"`      // text | voice
	Source     string `json:"source"`    // lead | ai | john | seed
	Text       string `json:"text"`
	AudioPath  string `json:"audio_path,omitempty"` // relative to data/audio
	DurationMS int64  `json:"duration_ms,omitempty"`
	RunID      int64  `json:"run_id,omitempty"`
	Status     string `json:"status,omitempty"` // outbound: sent | delivered | read | played
	CreatedAt  int64  `json:"created_at"`
}

// Run is one reply pipeline pass. Stage times are unix ms (0 = not reached).
type Run struct {
	ID         int64  `json:"id"`
	ContactID  int64  `json:"contact_id"`
	ReceivedAt int64  `json:"received_at"`
	HeardAt    int64  `json:"heard_at"`
	ThoughtAt  int64  `json:"thought_at"`
	SpokenAt   int64  `json:"spoken_at"`
	SentAt     int64  `json:"sent_at"`
	Status     string `json:"status"` // running | sent | handoff | failed
	Error      string `json:"error,omitempty"`
	Model      string `json:"model,omitempty"`
	Brain      string `json:"brain,omitempty"` // raw brain JSON
}

var ErrDuplicate = errors.New("duplicate message")

type Store struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS contacts (
  id INTEGER PRIMARY KEY,
  jid TEXT NOT NULL UNIQUE,
  chat TEXT NOT NULL DEFAULT '',
  phone TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL DEFAULT '',
  push_name TEXT NOT NULL DEFAULT '',
  context TEXT NOT NULL DEFAULT '',
  memory TEXT NOT NULL DEFAULT '[]',
  status TEXT NOT NULL DEFAULT 'new',
  handoff_reason TEXT NOT NULL DEFAULT '',
  link_sent_at INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  last_message_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS messages (
  id INTEGER PRIMARY KEY,
  contact_id INTEGER NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
  wa_id TEXT,
  direction TEXT NOT NULL,
  kind TEXT NOT NULL,
  source TEXT NOT NULL,
  text TEXT NOT NULL DEFAULT '',
  audio_path TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0,
  run_id INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS messages_wa_id ON messages(wa_id) WHERE wa_id IS NOT NULL AND wa_id != '';
CREATE INDEX IF NOT EXISTS messages_contact ON messages(contact_id, created_at);
CREATE TABLE IF NOT EXISTS runs (
  id INTEGER PRIMARY KEY,
  contact_id INTEGER NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
  received_at INTEGER NOT NULL DEFAULT 0,
  heard_at INTEGER NOT NULL DEFAULT 0,
  thought_at INTEGER NOT NULL DEFAULT 0,
  spoken_at INTEGER NOT NULL DEFAULT 0,
  sent_at INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'running',
  error TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  brain TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY,
  contact_id INTEGER NOT NULL DEFAULT 0,
  type TEXT NOT NULL,
  payload TEXT NOT NULL DEFAULT '{}',
  created_at INTEGER NOT NULL
);`

// Open creates or opens the database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one writer; avoids SQLITE_BUSY under concurrent pipelines
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	// Added after the first release; ignore "duplicate column" on existing DBs.
	db.Exec(`ALTER TABLE messages ADD COLUMN status TEXT NOT NULL DEFAULT ''`)
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Reset deletes all app data (used by seed). The WhatsApp session is untouched.
func (s *Store) Reset() error {
	_, err := s.db.Exec(`DELETE FROM events; DELETE FROM runs; DELETE FROM messages; DELETE FROM contacts;`)
	return err
}

func Now() int64 { return time.Now().UnixMilli() }

const contactCols = `id, jid, chat, phone, name, push_name, context, memory, status, handoff_reason, link_sent_at, created_at, last_message_at`

func scanContact(row interface{ Scan(...any) error }) (Contact, error) {
	var c Contact
	var mem string
	err := row.Scan(&c.ID, &c.JID, &c.Chat, &c.Phone, &c.Name, &c.PushName, &c.Context, &mem,
		&c.Status, &c.HandoffReason, &c.LinkSentAt, &c.CreatedAt, &c.LastMessageAt)
	if err == nil {
		json.Unmarshal([]byte(mem), &c.Memory)
	}
	return c, err
}

// Contact returns a contact by id.
func (s *Store) Contact(id int64) (Contact, error) {
	return scanContact(s.db.QueryRow(`SELECT `+contactCols+` FROM contacts WHERE id = ?`, id))
}

// ContactByJID returns a contact by identity JID, or sql.ErrNoRows.
func (s *Store) ContactByJID(jid string) (Contact, error) {
	return scanContact(s.db.QueryRow(`SELECT `+contactCols+` FROM contacts WHERE jid = ?`, jid))
}

// Contacts lists contacts, most recent conversation first.
func (s *Store) Contacts() ([]Contact, error) {
	rows, err := s.db.Query(`SELECT ` + contactCols + ` FROM contacts ORDER BY last_message_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Contact
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpsertInbound finds the contact for a sender or creates it. It refreshes
// the reply chat and push name but never overwrites a name John gave them.
func (s *Store) UpsertInbound(jid, chat, phone, pushName string) (Contact, bool, error) {
	c, err := s.ContactByJID(jid)
	if err == nil {
		_, err = s.db.Exec(`UPDATE contacts SET chat = ?, push_name = CASE WHEN ? != '' THEN ? ELSE push_name END,
			phone = CASE WHEN phone = '' THEN ? ELSE phone END WHERE id = ?`, chat, pushName, pushName, phone, c.ID)
		if err != nil {
			return c, false, err
		}
		c, err = s.Contact(c.ID)
		return c, false, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return c, false, err
	}
	c, err = s.CreateContact(Contact{JID: jid, Chat: chat, Phone: phone, PushName: pushName, Status: StatusNew})
	return c, true, err
}

// CreateContact inserts a contact (seed, campaign, or first inbound).
func (s *Store) CreateContact(c Contact) (Contact, error) {
	if c.Status == "" {
		c.Status = StatusNew
	}
	if c.CreatedAt == 0 {
		c.CreatedAt = Now()
	}
	if c.Chat == "" {
		c.Chat = c.JID
	}
	mem, _ := json.Marshal(nonNil(c.Memory))
	res, err := s.db.Exec(`INSERT INTO contacts (jid, chat, phone, name, push_name, context, memory, status, handoff_reason, link_sent_at, created_at, last_message_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.JID, c.Chat, c.Phone, c.Name, c.PushName, c.Context, string(mem), c.Status, c.HandoffReason, c.LinkSentAt, c.CreatedAt, c.LastMessageAt)
	if err != nil {
		return c, err
	}
	c.ID, _ = res.LastInsertId()
	return s.Contact(c.ID)
}

// UpsertByPhone creates a contact for a phone number, or renames an existing
// one. Used for campaign recipients (teammates' phones under demo names).
func (s *Store) UpsertByPhone(phone, name, context string) (Contact, error) {
	jid := phone + "@s.whatsapp.net"
	c, err := s.ContactByJID(jid)
	if errors.Is(err, sql.ErrNoRows) {
		return s.CreateContact(Contact{JID: jid, Chat: jid, Phone: phone, Name: name, Context: context})
	}
	if err != nil {
		return c, err
	}
	if _, err := s.db.Exec(`UPDATE contacts SET name = ?, context = ? WHERE id = ?`, name, context, c.ID); err != nil {
		return c, err
	}
	return s.Contact(c.ID)
}

// ResetContact wipes a contact's conversation so their next message is a
// fresh first contact. The name and context (e.g. a teammate saved as "Raj")
// are kept.
func (s *Store) ResetContact(id int64) error {
	_, err := s.db.Exec(`DELETE FROM messages WHERE contact_id = ?; DELETE FROM runs WHERE contact_id = ?; DELETE FROM events WHERE contact_id = ?;
		UPDATE contacts SET status = 'new', handoff_reason = '', memory = '[]', link_sent_at = 0, last_message_at = 0 WHERE id = ?`, id, id, id, id)
	return err
}

// SetStatus changes a contact's status chip.
func (s *Store) SetStatus(id int64, status, reason string) error {
	_, err := s.db.Exec(`UPDATE contacts SET status = ?, handoff_reason = ? WHERE id = ?`, status, reason, id)
	return err
}

// Remember appends new facts to a contact's memory, skipping duplicates.
func (s *Store) Remember(id int64, facts []string) error {
	if len(facts) == 0 {
		return nil
	}
	c, err := s.Contact(id)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, f := range c.Memory {
		seen[strings.ToLower(f)] = true
	}
	for _, f := range facts {
		if f = strings.TrimSpace(f); f != "" && !seen[strings.ToLower(f)] {
			c.Memory = append(c.Memory, f)
			seen[strings.ToLower(f)] = true
		}
	}
	mem, _ := json.Marshal(c.Memory)
	_, err = s.db.Exec(`UPDATE contacts SET memory = ? WHERE id = ?`, string(mem), id)
	return err
}

// MarkLinkSent records that the referral link went out.
func (s *Store) MarkLinkSent(id int64) error {
	_, err := s.db.Exec(`UPDATE contacts SET link_sent_at = ? WHERE id = ?`, Now(), id)
	return err
}

// AddMessage stores a message and bumps the contact. A repeated WhatsApp
// message id returns ErrDuplicate (whatsmeow can redeliver).
func (s *Store) AddMessage(m Message) (Message, error) {
	if m.CreatedAt == 0 {
		m.CreatedAt = Now()
	}
	var waID any
	if m.WAID != "" {
		waID = m.WAID
	}
	if m.Direction == "out" && m.Status == "" {
		m.Status = "sent"
	}
	res, err := s.db.Exec(`INSERT INTO messages (contact_id, wa_id, direction, kind, source, text, audio_path, duration_ms, run_id, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ContactID, waID, m.Direction, m.Kind, m.Source, m.Text, m.AudioPath, m.DurationMS, m.RunID, m.Status, m.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return m, ErrDuplicate
		}
		return m, err
	}
	m.ID, _ = res.LastInsertId()
	_, err = s.db.Exec(`UPDATE contacts SET last_message_at = MAX(last_message_at, ?) WHERE id = ?`, m.CreatedAt, m.ContactID)
	return m, err
}

// SetMessageText fills in a voice note's transcript.
func (s *Store) SetMessageText(id int64, text string) error {
	_, err := s.db.Exec(`UPDATE messages SET text = ? WHERE id = ?`, text, id)
	return err
}

// SetMessageAudio records where a message's audio is stored.
func (s *Store) SetMessageAudio(id int64, path string, durationMS int64) error {
	_, err := s.db.Exec(`UPDATE messages SET audio_path = ?, duration_ms = ? WHERE id = ?`, path, durationMS, id)
	return err
}

// Messages returns a contact's last n messages, oldest first.
func (s *Store) Messages(contactID int64, n int) ([]Message, error) {
	rows, err := s.db.Query(`SELECT `+messageCols+`
		FROM (SELECT * FROM messages WHERE contact_id = ? ORDER BY created_at DESC, id DESC LIMIT ?) ORDER BY created_at, id`, contactID, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

const messageCols = `id, contact_id, COALESCE(wa_id, ''), direction, kind, source, text, audio_path, duration_ms, run_id, status, created_at`

func scanMessage(row interface{ Scan(...any) error }) (Message, error) {
	var m Message
	err := row.Scan(&m.ID, &m.ContactID, &m.WAID, &m.Direction, &m.Kind, &m.Source, &m.Text, &m.AudioPath, &m.DurationMS, &m.RunID, &m.Status, &m.CreatedAt)
	return m, err
}

var statusRank = map[string]int{"": 0, "sent": 1, "delivered": 2, "read": 3, "played": 4}

// SetDelivery upgrades an outbound message's receipt status (never
// downgrades: a "read" arriving after "played" is ignored). It returns the
// updated message when something changed.
func (s *Store) SetDelivery(waID, status string) (Message, bool, error) {
	m, err := scanMessage(s.db.QueryRow(`SELECT `+messageCols+` FROM messages WHERE wa_id = ? AND direction = 'out'`, waID))
	if errors.Is(err, sql.ErrNoRows) {
		return m, false, nil
	}
	if err != nil || statusRank[status] <= statusRank[m.Status] {
		return m, false, err
	}
	// Text messages are never "played"; read is as far as they go.
	if status == "played" && m.Kind != "voice" {
		status = "read"
	}
	if _, err := s.db.Exec(`UPDATE messages SET status = ? WHERE id = ?`, status, m.ID); err != nil {
		return m, false, err
	}
	m.Status = status
	return m, true, nil
}

// NewRun starts a pipeline run.
func (s *Store) NewRun(contactID, receivedAt int64) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO runs (contact_id, received_at) VALUES (?, ?)`, contactID, receivedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Stage records the time a run reached a stage: heard, thought, spoken, sent.
func (s *Store) Stage(runID int64, stage string, at int64) error {
	col := map[string]string{"heard": "heard_at", "thought": "thought_at", "spoken": "spoken_at", "sent": "sent_at"}[stage]
	if col == "" {
		return fmt.Errorf("unknown stage %q", stage)
	}
	_, err := s.db.Exec(`UPDATE runs SET `+col+` = ? WHERE id = ?`, at, runID)
	return err
}

// FinishRun records the outcome of a run.
func (s *Store) FinishRun(runID int64, status, errText, model, brainJSON string) error {
	_, err := s.db.Exec(`UPDATE runs SET status = ?, error = ?, model = ?, brain = ? WHERE id = ?`, status, errText, model, brainJSON, runID)
	return err
}

// Runs returns the most recent runs, newest first.
func (s *Store) Runs(n int) ([]Run, error) {
	rows, err := s.db.Query(`SELECT id, contact_id, received_at, heard_at, thought_at, spoken_at, sent_at, status, error, model, brain
		FROM runs ORDER BY id DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.ContactID, &r.ReceivedAt, &r.HeardAt, &r.ThoughtAt, &r.SpokenAt, &r.SentAt, &r.Status, &r.Error, &r.Model, &r.Brain); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddEvent logs a business event: reply_sent, handoff, taken_over, handed_back, link_clicked.
func (s *Store) AddEvent(contactID int64, typ string, payload any) error {
	p, _ := json.Marshal(payload)
	_, err := s.db.Exec(`INSERT INTO events (contact_id, type, payload, created_at) VALUES (?, ?, ?, ?)`, contactID, typ, string(p), Now())
	return err
}

// Stats feed the dashboard header and the hours-saved counter.
type Stats struct {
	Contacts      int `json:"contacts"`
	RepliesWeek   int `json:"replies_week"`   // AI voice notes in the last 7 days
	RepliesToday  int `json:"replies_today"`  // AI voice notes since midnight
	NeedsJohn     int `json:"needs_john"`     // chats waiting for John
	LinksSent     int `json:"links_sent"`     // contacts who got the referral link
	SecondsSpoken int `json:"seconds_spoken"` // total AI voice-note audio this week
}

func (s *Store) Stats() (Stats, error) {
	var st Stats
	now := time.Now()
	week := now.Add(-7 * 24 * time.Hour).UnixMilli()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).UnixMilli()
	err := s.db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM contacts),
		(SELECT COUNT(*) FROM messages WHERE direction = 'out' AND kind = 'voice' AND source IN ('ai', 'seed') AND created_at >= ?),
		(SELECT COUNT(*) FROM messages WHERE direction = 'out' AND kind = 'voice' AND source IN ('ai', 'seed') AND created_at >= ?),
		(SELECT COUNT(*) FROM contacts WHERE status = 'needs_john'),
		(SELECT COUNT(*) FROM contacts WHERE link_sent_at > 0),
		(SELECT COALESCE(SUM(duration_ms), 0) / 1000 FROM messages WHERE direction = 'out' AND kind = 'voice' AND source IN ('ai', 'seed') AND created_at >= ?)`,
		week, midnight, week).Scan(&st.Contacts, &st.RepliesWeek, &st.RepliesToday, &st.NeedsJohn, &st.LinksSent, &st.SecondsSpoken)
	return st, err
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
