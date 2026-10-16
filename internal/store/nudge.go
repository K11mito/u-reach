package store

import (
	"encoding/json"
	"strings"
)

// Follow-up nudges. When a lead goes quiet after John's last message, the AI
// drafts a short personal follow-up and John approves it before it's sent.
// At most one nudge per silence: once a lead has been nudged (or John skipped
// them), they aren't offered again until they write back.
const nudgeSchema = `
CREATE TABLE IF NOT EXISTS nudges (
  id INTEGER PRIMARY KEY,
  contact_id INTEGER NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
  script TEXT NOT NULL DEFAULT '',
  followup TEXT NOT NULL DEFAULT '',
  link INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'draft',
  error TEXT NOT NULL DEFAULT '',
  audio_path TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0,
  quiet_since INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  sent_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS nudges_contact ON nudges(contact_id, created_at);`

// Nudge statuses.
const (
	NudgeDraft   = "draft"   // written, waiting for John
	NudgeSending = "sending" // approved and going out
	NudgeSent    = "sent"
	NudgeSkipped = "skipped" // John said not now
	NudgeFailed  = "failed"  // drafting or sending went wrong; redraft or retry
	NudgeStale   = "stale"   // the lead wrote back first, so it's not needed
)

// Nudge is one follow-up for a quiet lead.
type Nudge struct {
	ID         int64  `json:"id"`
	ContactID  int64  `json:"contact_id"`
	Script     string `json:"script"`   // the voice note's words
	Followup   string `json:"followup"` // optional text after it
	Link       bool   `json:"link"`     // add the referral link, if they still don't have it
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	AudioPath  string `json:"audio_path,omitempty"` // recorded on listen or send, relative to data/audio
	DurationMS int64  `json:"duration_ms,omitempty"`
	QuietSince int64  `json:"quiet_since"` // John's last message, when the silence began
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
	SentAt     int64  `json:"sent_at,omitempty"`
}

const nudgeCols = `id, contact_id, script, followup, link, status, error, audio_path, duration_ms, quiet_since, created_at, updated_at, sent_at`

func scanNudge(row interface{ Scan(...any) error }) (Nudge, error) {
	var n Nudge
	err := row.Scan(&n.ID, &n.ContactID, &n.Script, &n.Followup, &n.Link, &n.Status, &n.Error, &n.AudioPath, &n.DurationMS, &n.QuietSince, &n.CreatedAt, &n.UpdatedAt, &n.SentAt)
	return n, err
}

// AddNudge stores a new nudge.
func (s *Store) AddNudge(n Nudge) (Nudge, error) {
	now := Now()
	if n.CreatedAt == 0 {
		n.CreatedAt = now
	}
	n.UpdatedAt = now
	if n.Status == "" {
		n.Status = NudgeDraft
	}
	res, err := s.db.Exec(`INSERT INTO nudges (contact_id, script, followup, link, status, error, audio_path, duration_ms, quiet_since, created_at, updated_at, sent_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ContactID, n.Script, n.Followup, n.Link, n.Status, n.Error, n.AudioPath, n.DurationMS, n.QuietSince, n.CreatedAt, n.UpdatedAt, n.SentAt)
	if err != nil {
		return n, err
	}
	n.ID, _ = res.LastInsertId()
	return n, nil
}

// Nudge returns a nudge by id.
func (s *Store) Nudge(id int64) (Nudge, error) {
	return scanNudge(s.db.QueryRow(`SELECT `+nudgeCols+` FROM nudges WHERE id = ?`, id))
}

// Nudges lists nudges in the given statuses, oldest silence first.
func (s *Store) Nudges(statuses ...string) ([]Nudge, error) {
	q := `SELECT ` + nudgeCols + ` FROM nudges`
	args := make([]any, len(statuses))
	if len(statuses) > 0 {
		q += ` WHERE status IN (?` + strings.Repeat(",?", len(statuses)-1) + `)`
		for i, st := range statuses {
			args[i] = st
		}
	}
	rows, err := s.db.Query(q+` ORDER BY quiet_since, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Nudge
	for rows.Next() {
		n, err := scanNudge(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// SaveNudge writes back a nudge's words, status, error, audio and send time.
func (s *Store) SaveNudge(n Nudge) error {
	_, err := s.db.Exec(`UPDATE nudges SET script = ?, followup = ?, link = ?, status = ?, error = ?, audio_path = ?, duration_ms = ?, sent_at = ?, updated_at = ? WHERE id = ?`,
		n.Script, n.Followup, n.Link, n.Status, n.Error, n.AudioPath, n.DurationMS, n.SentAt, Now(), n.ID)
	return err
}

// SaveDraft writes back a nudge only while it's still a draft or a failed
// one (an edit, a redraft, its recorded audio). It reports false once it's
// being sent or is done, so a late edit can't change what already went out.
func (s *Store) SaveDraft(n Nudge) (bool, error) {
	res, err := s.db.Exec(`UPDATE nudges SET script = ?, followup = ?, link = ?, status = ?, error = ?, audio_path = ?, duration_ms = ?, updated_at = ?
		WHERE id = ? AND status IN (?, ?)`,
		n.Script, n.Followup, n.Link, n.Status, n.Error, n.AudioPath, n.DurationMS, Now(), n.ID, NudgeDraft, NudgeFailed)
	if err != nil {
		return false, err
	}
	k, err := res.RowsAffected()
	return k == 1, err
}

// SkipNudge sets a draft aside: the lead isn't offered again until they write back.
func (s *Store) SkipNudge(id int64) (bool, error) {
	res, err := s.db.Exec(`UPDATE nudges SET status = ?, updated_at = ? WHERE id = ? AND status IN (?, ?)`,
		NudgeSkipped, Now(), id, NudgeDraft, NudgeFailed)
	if err != nil {
		return false, err
	}
	k, err := res.RowsAffected()
	return k == 1, err
}

// RecoverNudges runs at startup: a nudge left "sending" by a crash may or may
// not have gone out, so it becomes a failed one for John to check, never a
// silent resend.
func (s *Store) RecoverNudges() error {
	_, err := s.db.Exec(`UPDATE nudges SET status = ?, error = ?, updated_at = ? WHERE status = ?`,
		NudgeFailed, "interrupted by a restart; check the chat before sending again", Now(), NudgeSending)
	return err
}

// NudgesSince counts nudges drafted since t (Unix ms), for the daily cap.
func (s *Store) NudgesSince(t int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM nudges WHERE created_at >= ?`, t).Scan(&n)
	return n, err
}

// ClaimNudge marks a draft (or a failed nudge being retried) as sending.
// It reports false if someone else already claimed it, so a double click
// or two open dashboards can never send it twice, and for a held-back
// draft with no words.
func (s *Store) ClaimNudge(id int64) (bool, error) {
	res, err := s.db.Exec(`UPDATE nudges SET status = ?, error = '', updated_at = ? WHERE id = ? AND script != '' AND status IN (?, ?)`,
		NudgeSending, Now(), id, NudgeDraft, NudgeFailed)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// StaleNudges retires a lead's unsent nudges once they're no longer needed
// (the lead wrote back, or John took the chat), saying why.
func (s *Store) StaleNudges(contactID int64, why string) (int64, error) {
	res, err := s.db.Exec(`UPDATE nudges SET status = ?, error = ?, updated_at = ? WHERE contact_id = ? AND status IN (?, ?)`,
		NudgeStale, why, Now(), contactID, NudgeDraft, NudgeFailed)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Quiet is a lead who hasn't answered John's last message.
type Quiet struct {
	Contact  Contact `json:"contact"`
	LastOut  int64   `json:"last_out"`       // John's last message: the silence starts here
	LastIn   int64   `json:"last_in"`        // their last message, 0 if they never wrote
	LastText string  `json:"last_lead_text"` // what they last said, for context
}

// QuietLeads lists leads the AI is handling who haven't answered John's last
// message for at least `after` (and at most `maxAge`, past which a follow-up
// would feel out of the blue), as of `now` (Unix ms). Left out: chats with
// John or waiting for him, leads who asked not to be contacted, and anyone
// who already has a nudge for this silence (drafted, sent, skipped or failed).
func (s *Store) QuietLeads(now, after, maxAge int64) ([]Quiet, error) {
	rows, err := s.db.Query(`SELECT `+prefixed("c.", contactCols)+`, w.last_out, w.last_in,
		COALESCE((SELECT text FROM messages m WHERE m.contact_id = c.id AND m.direction = 'in' ORDER BY created_at DESC, id DESC LIMIT 1), '')
		FROM contacts c JOIN (
			SELECT contact_id,
				MAX(CASE WHEN direction = 'out' THEN created_at ELSE 0 END) AS last_out,
				MAX(CASE WHEN direction = 'in' THEN created_at ELSE 0 END) AS last_in
			FROM messages GROUP BY contact_id
		) w ON w.contact_id = c.id
		WHERE c.status = ?
			AND w.last_out > w.last_in
			AND w.last_out <= ? AND w.last_out >= ?
			AND NOT EXISTS (SELECT 1 FROM events e WHERE e.contact_id = c.id AND e.type = 'opt_out' AND e.created_at >= w.last_in)
			AND NOT EXISTS (SELECT 1 FROM nudges n WHERE n.contact_id = c.id AND n.status != ? AND n.created_at > w.last_in)
		ORDER BY w.last_out, c.id`,
		StatusAuto, now-after, now-maxAge, NudgeStale)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Quiet
	for rows.Next() {
		var q Quiet
		var mem string
		c := &q.Contact
		if err := rows.Scan(&c.ID, &c.JID, &c.Chat, &c.Phone, &c.Name, &c.PushName, &c.Context, &mem,
			&c.Status, &c.HandoffReason, &c.LinkSentAt, &c.Avatar, &c.Interest, &c.InterestReason, &c.InterestAt, &c.CreatedAt, &c.LastMessageAt,
			&q.LastOut, &q.LastIn, &q.LastText); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(mem), &c.Memory)
		out = append(out, q)
	}
	return out, rows.Err()
}

// prefixed qualifies each column in a list with a table alias.
func prefixed(alias, cols string) string {
	parts := strings.Split(cols, ",")
	for i, col := range parts {
		parts[i] = alias + strings.TrimSpace(col)
	}
	return strings.Join(parts, ", ")
}
