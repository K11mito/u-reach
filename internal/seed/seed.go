// Package seed loads the fake leads and past conversations from
// data/seed-leads.json, so the dashboard looks lived-in. Used by cmd/seed and
// by the dashboard's "Reset demo" button. Lead photos live in
// data/seed-avatars and are copied next to the audio the dashboard serves.
package seed

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ureach/internal/store"
	"ureach/internal/voice"
)

type message struct {
	Ago  string `json:"ago"`  // Go duration before now, e.g. "26h"
	From string `json:"from"` // lead | ai | john
	Kind string `json:"kind"` // text (default) | voice
	Text string `json:"text"`
}

type lead struct {
	Name          string    `json:"name"`
	Phone         string    `json:"phone"`
	Context       string    `json:"context"`
	Status        string    `json:"status"`
	HandoffReason string    `json:"handoff_reason"`
	Avatar        string    `json:"avatar"` // file in data/seed-avatars
	Memory        []string  `json:"memory"`
	Messages      []message `json:"messages"`
}

// VoiceNote is a seeded AI voice note that can be rendered with TTS.
type VoiceNote struct {
	MessageID int64
	Text      string
}

// Result summarizes what was loaded.
type Result struct {
	Leads      int
	Messages   int
	VoiceNotes []VoiceNote
}

// Load wipes app data (never the WhatsApp session) and loads the seed file.
func Load(st *store.Store, path, referral string) (Result, error) {
	leads, err := read(path)
	if err != nil {
		return Result{}, err
	}
	if err := st.Reset(); err != nil {
		return Result{}, err
	}
	return insert(st, path, referral, leads)
}

// Add loads only the seeded leads that aren't in the database yet, keeping
// live chats. Existing seeded leads just get their photo if they lack one.
func Add(st *store.Store, path, referral string) (Result, error) {
	leads, err := read(path)
	if err != nil {
		return Result{}, err
	}
	var missing []lead
	for _, l := range leads {
		c, err := st.ContactByJID(l.Phone + "@s.whatsapp.net")
		switch {
		case errors.Is(err, sql.ErrNoRows):
			missing = append(missing, l)
		case err != nil:
			return Result{}, err
		case c.Avatar == "" && l.Avatar != "":
			if err := setAvatar(st, path, c.ID, l.Avatar); err != nil {
				return Result{}, err
			}
		}
	}
	return insert(st, path, referral, missing)
}

func read(path string) ([]lead, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var leads []lead
	if err := json.Unmarshal(raw, &leads); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return leads, nil
}

// setAvatar copies a lead's photo into data/audio/avatars/seed, which the
// dashboard serves, and points the contact at it.
func setAvatar(st *store.Store, path string, id int64, file string) error {
	dir := filepath.Dir(path)
	img, err := os.ReadFile(filepath.Join(dir, "seed-avatars", file))
	if err != nil {
		return err
	}
	dst := filepath.Join(dir, "audio", "avatars", "seed", file)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dst, img, 0o644); err != nil {
		return err
	}
	return st.SetAvatar(id, "avatars/seed/"+file)
}

func insert(st *store.Store, path, referral string, leads []lead) (Result, error) {
	now := time.Now()
	var res Result
	for _, l := range leads {
		jid := l.Phone + "@s.whatsapp.net"
		c := store.Contact{JID: jid, Chat: jid, Phone: l.Phone, Name: l.Name, Context: l.Context,
			Memory: l.Memory, Status: l.Status, HandoffReason: l.HandoffReason, CreatedAt: now.Add(-7 * 24 * time.Hour).UnixMilli()}
		for _, m := range l.Messages {
			if strings.Contains(m.Text, "{REFERRAL_URL}") {
				c.LinkSentAt = at(now, m.Ago)
			}
		}
		created, err := st.CreateContact(c)
		if err != nil {
			return res, err
		}
		if l.Avatar != "" {
			if err := setAvatar(st, path, created.ID, l.Avatar); err != nil {
				return res, err
			}
		}
		for _, m := range l.Messages {
			dir, source := "out", "seed"
			switch m.From {
			case "lead":
				dir, source = "in", "lead"
			case "john":
				source = "john"
			}
			kind := m.Kind
			if kind == "" {
				kind = "text"
			}
			status := ""
			if dir == "out" {
				status = map[string]string{"voice": "played", "text": "read"}[kind] // a lived-in history has been listened to
			}
			msg, err := st.AddMessage(store.Message{
				ContactID: created.ID, Direction: dir, Kind: kind, Source: source, Status: status,
				Text: strings.ReplaceAll(m.Text, "{REFERRAL_URL}", referral), CreatedAt: at(now, m.Ago),
			})
			if err != nil {
				return res, err
			}
			res.Messages++
			if kind == "voice" && source == "seed" {
				res.VoiceNotes = append(res.VoiceNotes, VoiceNote{msg.ID, msg.Text})
			}
		}
		res.Leads++
	}
	return res, nil
}

// Characters is what rendering these notes would cost in TTS characters.
func Characters(notes []VoiceNote) int {
	n := 0
	for _, v := range notes {
		n += len(voice.Speakable(v.Text))
	}
	return n
}

// RenderAudio voices the seeded notes. TTS is cached, so after the first
// render a reset costs nothing (unless the voice or model changed).
func RenderAudio(ctx context.Context, st *store.Store, el *voice.Eleven, voiceID, model string, notes []VoiceNote, progress func(VoiceNote, float64, bool)) error {
	for _, v := range notes {
		mp3, cached, err := el.TTS(ctx, voiceID, model, v.Text, voice.DefaultSettings)
		if err != nil {
			return err
		}
		dur, _ := voice.Duration(ctx, mp3)
		if err := st.SetMessageAudio(v.MessageID, strings.TrimPrefix(mp3, "data/audio/"), int64(dur*1000)); err != nil {
			return err
		}
		if progress != nil {
			progress(v, dur, cached)
		}
	}
	return nil
}

// LiveMessages counts real (non-seed) messages that a reset would delete.
func LiveMessages(st *store.Store) int {
	contacts, err := st.Contacts()
	if err != nil {
		return 0
	}
	n := 0
	for _, c := range contacts {
		msgs, _ := st.Messages(c.ID, 1000)
		for _, m := range msgs {
			if m.Source == "ai" || (m.Source == "lead" && m.WAID != "") {
				n++
			}
		}
	}
	return n
}

func at(now time.Time, ago string) int64 {
	d, err := time.ParseDuration(ago)
	if err != nil {
		d = 0
	}
	return now.Add(-d).UnixMilli()
}
