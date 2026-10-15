// Package harness is John's editable "harness": the persona card and reply
// style (per preset), the FAQ, the referral link, and uploaded sales
// playbook documents. It persists them in the store and pushes the active
// combination into the brain whenever anything changes.
package harness

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"ureach/internal/brain"
	"ureach/internal/store"
)

// Persona is John's persona card, the same shape as data/persona/john.json.
type Persona struct {
	Name             string   `json:"name"`
	ShortName        string   `json:"short_name"`
	Role             string   `json:"role"`
	Background       []string `json:"background"`
	Voice            Voice    `json:"voice"`
	Openers          []string `json:"openers"`
	SignaturePhrases []string `json:"signature_phrases"`
	Metaphors        []string `json:"metaphors"`
	Values           []string `json:"values"`
	Closers          []string `json:"closers"`
	Never            []string `json:"never"`
	SampleLines      []string `json:"sample_lines"`
	HowHeSells       []string `json:"how_he_sells"`
}

type Voice struct {
	Tone       string `json:"tone"`
	Rhythm     string `json:"rhythm"`
	English    string `json:"english"`
	BrandWords string `json:"brand_words"`
}

// Harness is what a preset holds.
type Harness struct {
	Persona Persona     `json:"persona"`
	Style   brain.Style `json:"style"`
}

// Preset is a named harness John can switch to.
type Preset struct {
	ID          int64   `json:"id"`
	Key         string  `json:"key,omitempty"` // built-in presets only
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Harness     Harness `json:"harness"`
	UpdatedAt   int64   `json:"updated_at"`
}

// DocInfo describes an uploaded playbook document without its full text.
type DocInfo struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Bytes     int64  `json:"bytes"`
	Words     int    `json:"words"`
	Chars     int    `json:"chars"`
	Active    bool   `json:"active"`
	Preview   string `json:"preview"`
	CreatedAt int64  `json:"created_at"`
}

// Snapshot is everything the dashboard's John page shows.
type Snapshot struct {
	ActiveID      int64     `json:"active_id"`
	Presets       []Preset  `json:"presets"`
	FAQ           string    `json:"faq"`
	ReferralURL   string    `json:"referral_url"`
	Documents     []DocInfo `json:"documents"`
	PlaybookMax   int       `json:"playbook_max_chars"`
	AlwaysHandoff []string  `json:"always_handoff"`
}

const (
	settingActive   = "active_preset"
	settingFAQ      = "faq"
	settingReferral = "referral_url"

	// MaxDocChars caps one document (~30k tokens); MaxPlaybookChars caps all
	// active documents together so every reply stays fast and affordable.
	MaxDocChars      = 120_000
	MaxPlaybookChars = 200_000
)

// ErrInvalid wraps problems the user can fix (shown as-is in the dashboard).
var ErrInvalid = errors.New("invalid")

// Service owns the harness. All methods are safe for concurrent use.
type Service struct {
	st          *store.Store
	br          *brain.Brain
	personaPath string
	base        Persona // data/persona/john.json, the built-ins start from it
	mu          sync.Mutex
}

// New loads John's base persona, seeds the built-in presets, FAQ and
// referral link on first run, and configures the brain from the active preset.
func New(st *store.Store, br *brain.Brain, personaPath, faqPath string) (*Service, error) {
	raw, err := os.ReadFile(personaPath)
	if err != nil {
		return nil, err
	}
	var base Persona
	if err := json.Unmarshal(raw, &base); err != nil {
		return nil, fmt.Errorf("%s: %w", personaPath, err)
	}
	s := &Service{st: st, br: br, personaPath: personaPath, base: base}

	presets, err := st.Presets()
	if err != nil {
		return nil, err
	}
	if len(presets) == 0 {
		var first int64
		for _, b := range builtins(base) {
			id, err := s.addPreset(b.Key, b.Name, b.Description, b.Harness)
			if err != nil {
				return nil, err
			}
			if first == 0 {
				first = id
			}
		}
		if err := st.SetSetting(settingActive, strconv.FormatInt(first, 10)); err != nil {
			return nil, err
		}
	}
	if _, ok, err := st.Setting(settingFAQ); err != nil {
		return nil, err
	} else if !ok {
		faq, err := os.ReadFile(faqPath)
		if err != nil {
			return nil, err
		}
		if err := st.SetSetting(settingFAQ, string(faq)); err != nil {
			return nil, err
		}
	}
	// An empty link falls back to REFERRAL_URL from .env, so adding it there later works.
	if v, _, err := st.Setting(settingReferral); err != nil {
		return nil, err
	} else if v == "" && br.Referral() != "" {
		if err := st.SetSetting(settingReferral, br.Referral()); err != nil {
			return nil, err
		}
	}
	return s, s.apply()
}

// Snapshot returns the full harness for the dashboard.
func (s *Service) Snapshot() (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot()
}

func (s *Service) snapshot() (Snapshot, error) {
	snap := Snapshot{PlaybookMax: MaxPlaybookChars, AlwaysHandoff: alwaysHandoff}
	rows, err := s.st.Presets()
	if err != nil {
		return snap, err
	}
	for _, r := range rows {
		p, err := toPreset(r)
		if err != nil {
			return snap, err
		}
		snap.Presets = append(snap.Presets, p)
	}
	snap.ActiveID, err = s.activeID()
	if err != nil {
		return snap, err
	}
	snap.FAQ, _, err = s.st.Setting(settingFAQ)
	if err != nil {
		return snap, err
	}
	snap.ReferralURL, _, err = s.st.Setting(settingReferral)
	if err != nil {
		return snap, err
	}
	docs, err := s.st.Documents()
	if err != nil {
		return snap, err
	}
	snap.Documents = []DocInfo{}
	for _, d := range docs {
		snap.Documents = append(snap.Documents, info(d))
	}
	return snap, nil
}

// ActivePersonaJSON is the active persona card, for the dashboard state.
func (s *Service) ActivePersonaJSON() json.RawMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.active()
	if err != nil {
		return json.RawMessage("null")
	}
	b, _ := json.Marshal(p.Harness.Persona)
	return b
}

// FAQ is the current FAQ text.
func (s *Service) FAQ() string {
	v, _, _ := s.st.Setting(settingFAQ)
	return v
}

// SaveActive replaces the active preset's persona and style.
func (s *Service) SaveActive(h Harness) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validate(&h); err != nil {
		return err
	}
	p, err := s.active()
	if err != nil {
		return err
	}
	if err := s.updatePreset(p.ID, p.Name, p.Description, h); err != nil {
		return err
	}
	return s.apply()
}

// Activate switches John to another preset.
func (s *Service) Activate(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.st.Preset(id); err != nil {
		return err
	}
	if err := s.st.SetSetting(settingActive, strconv.FormatInt(id, 10)); err != nil {
		return err
	}
	return s.apply()
}

// SaveAs copies the given harness into a new preset and makes it active.
func (s *Service) SaveAs(name, description string, h Harness) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, fmt.Errorf("%w: give the preset a name", ErrInvalid)
	}
	if err := validate(&h); err != nil {
		return 0, err
	}
	id, err := s.addPreset("", name, strings.TrimSpace(description), h)
	if err != nil {
		return 0, err
	}
	if err := s.st.SetSetting(settingActive, strconv.FormatInt(id, 10)); err != nil {
		return 0, err
	}
	return id, s.apply()
}

// Rename changes a preset's name and description.
func (s *Service) Rename(id int64, name, description string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: give the preset a name", ErrInvalid)
	}
	row, err := s.st.Preset(id)
	if err != nil {
		return err
	}
	p, err := toPreset(row)
	if err != nil {
		return err
	}
	return s.updatePreset(id, name, strings.TrimSpace(description), p.Harness)
}

// Delete removes a custom preset. Built-ins can only be reset.
func (s *Service) Delete(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.st.Preset(id)
	if err != nil {
		return err
	}
	if row.Key != "" {
		return fmt.Errorf("%w: built-in presets can be reset but not deleted", ErrInvalid)
	}
	if err := s.st.DeletePreset(id); err != nil {
		return err
	}
	if active, _ := s.activeID(); active == id {
		rows, err := s.st.Presets()
		if err != nil || len(rows) == 0 {
			return err
		}
		if err := s.st.SetSetting(settingActive, strconv.FormatInt(rows[0].ID, 10)); err != nil {
			return err
		}
	}
	return s.apply()
}

// Reset restores a built-in preset to its shipped version.
func (s *Service) Reset(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.st.Preset(id)
	if err != nil {
		return err
	}
	for _, b := range builtins(s.base) {
		if b.Key == row.Key && row.Key != "" {
			if err := s.updatePreset(id, b.Name, b.Description, b.Harness); err != nil {
				return err
			}
			return s.apply()
		}
	}
	return fmt.Errorf("%w: only built-in presets can be reset", ErrInvalid)
}

// SetFAQ replaces the facts John may state.
func (s *Service) SetFAQ(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("%w: the FAQ can't be empty; John would have no facts to share", ErrInvalid)
	}
	if err := s.st.SetSetting(settingFAQ, text); err != nil {
		return err
	}
	return s.apply()
}

// SetReferral replaces John's referral link ("" turns links off).
func (s *Service) SetReferral(url string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	url = strings.TrimSpace(url)
	if url != "" && !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return fmt.Errorf("%w: the link must start with https://", ErrInvalid)
	}
	if err := s.st.SetSetting(settingReferral, url); err != nil {
		return err
	}
	return s.apply()
}

// AddDocument extracts the text of an uploaded .md, .txt or .pdf file and
// adds it to John's playbook (active if it fits the playbook budget).
func (s *Service) AddDocument(name string, data []byte) (DocInfo, error) {
	text, kind, err := extract(name, data)
	if err != nil {
		return DocInfo{}, err
	}
	if n := utf8.RuneCountInString(text); n > MaxDocChars {
		return DocInfo{}, fmt.Errorf("%w: %s has %d characters of text; the limit is %d. Split it or keep the sales parts", ErrInvalid, name, n, MaxDocChars)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	used, err := s.activeChars()
	if err != nil {
		return DocInfo{}, err
	}
	d := store.Document{Name: name, Kind: kind, Text: text, Bytes: int64(len(data)), Active: used+utf8.RuneCountInString(text) <= MaxPlaybookChars}
	d.ID, err = s.st.AddDocument(d)
	if err != nil {
		return DocInfo{}, err
	}
	return info(d), s.apply()
}

// SetDocumentActive turns a playbook document on or off.
func (s *Service) SetDocumentActive(id int64, active bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if active {
		docs, err := s.st.Documents()
		if err != nil {
			return err
		}
		used, add := 0, 0
		for _, d := range docs {
			n := utf8.RuneCountInString(d.Text)
			if d.ID == id {
				add = n
			} else if d.Active {
				used += n
			}
		}
		if used+add > MaxPlaybookChars {
			return fmt.Errorf("%w: turning this on would make the playbook too long for John to read on every reply; switch another document off first", ErrInvalid)
		}
	}
	if err := s.st.SetDocumentActive(id, active); err != nil {
		return err
	}
	return s.apply()
}

// DeleteDocument removes a playbook document.
func (s *Service) DeleteDocument(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.st.DeleteDocument(id); err != nil {
		return err
	}
	return s.apply()
}

// DocumentText returns a document's full extracted text.
func (s *Service) DocumentText(id int64) (string, error) {
	docs, err := s.st.Documents()
	if err != nil {
		return "", err
	}
	for _, d := range docs {
		if d.ID == id {
			return d.Text, nil
		}
	}
	return "", store.ErrNotFound
}

// apply pushes the active preset, FAQ, playbook and link into the brain.
func (s *Service) apply() error {
	p, err := s.active()
	if err != nil {
		return err
	}
	persona, err := json.MarshalIndent(p.Harness.Persona, "", "  ")
	if err != nil {
		return err
	}
	faq, _, err := s.st.Setting(settingFAQ)
	if err != nil {
		return err
	}
	referral, _, err := s.st.Setting(settingReferral)
	if err != nil {
		return err
	}
	docs, err := s.st.Documents()
	if err != nil {
		return err
	}
	var playbook []brain.Doc
	for _, d := range docs {
		if d.Active {
			playbook = append(playbook, brain.Doc{Name: d.Name, Text: d.Text})
		}
	}
	s.br.Configure(brain.Config{Persona: string(persona), FAQ: faq, Playbook: playbook, Style: p.Harness.Style, ReferralURL: referral})
	return nil
}

func (s *Service) activeID() (int64, error) {
	v, _, err := s.st.Setting(settingActive)
	if err != nil {
		return 0, err
	}
	id, _ := strconv.ParseInt(v, 10, 64)
	if id == 0 {
		rows, err := s.st.Presets()
		if err != nil || len(rows) == 0 {
			return 0, errors.New("no presets")
		}
		id = rows[0].ID
	}
	return id, nil
}

func (s *Service) active() (Preset, error) {
	id, err := s.activeID()
	if err != nil {
		return Preset{}, err
	}
	row, err := s.st.Preset(id)
	if errors.Is(err, store.ErrNotFound) {
		// The active preset was deleted out from under us: fall back to the first.
		rows, err := s.st.Presets()
		if err != nil || len(rows) == 0 {
			return Preset{}, errors.New("no presets")
		}
		row = rows[0]
	} else if err != nil {
		return Preset{}, err
	}
	return toPreset(row)
}

func (s *Service) activeChars() (int, error) {
	docs, err := s.st.Documents()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, d := range docs {
		if d.Active {
			n += utf8.RuneCountInString(d.Text)
		}
	}
	return n, nil
}

func (s *Service) addPreset(key, name, description string, h Harness) (int64, error) {
	raw, err := json.Marshal(h)
	if err != nil {
		return 0, err
	}
	return s.st.AddPreset(store.Preset{Key: key, Name: name, Description: description, Harness: string(raw)})
}

func (s *Service) updatePreset(id int64, name, description string, h Harness) error {
	raw, err := json.Marshal(h)
	if err != nil {
		return err
	}
	return s.st.UpdatePreset(store.Preset{ID: id, Name: name, Description: description, Harness: string(raw)})
}

func toPreset(r store.Preset) (Preset, error) {
	p := Preset{ID: r.ID, Key: r.Key, Name: r.Name, Description: r.Description, UpdatedAt: r.UpdatedAt}
	if err := json.Unmarshal([]byte(r.Harness), &p.Harness); err != nil {
		return p, fmt.Errorf("preset %d: %w", r.ID, err)
	}
	p.Harness.Style = p.Harness.Style.Normalized()
	return p, nil
}

// validate trims empty list items and checks what the brain relies on.
func validate(h *Harness) error {
	p := &h.Persona
	p.Name, p.Role = strings.TrimSpace(p.Name), strings.TrimSpace(p.Role)
	if p.Name == "" {
		return fmt.Errorf("%w: John needs a name", ErrInvalid)
	}
	for _, l := range []*[]string{&p.Background, &p.Openers, &p.SignaturePhrases, &p.Metaphors, &p.Values, &p.Closers, &p.Never, &p.SampleLines, &p.HowHeSells} {
		*l = clean(*l)
	}
	h.Style = h.Style.Normalized()
	return nil
}

func clean(items []string) []string {
	out := []string{}
	for _, it := range items {
		if it = strings.TrimSpace(it); it != "" {
			out = append(out, it)
		}
	}
	return out
}

func info(d store.Document) DocInfo {
	preview := strings.Join(strings.Fields(d.Text), " ")
	if r := []rune(preview); len(r) > 280 {
		preview = string(r[:280]) + "…"
	}
	return DocInfo{ID: d.ID, Name: d.Name, Kind: d.Kind, Bytes: d.Bytes, Words: len(strings.Fields(d.Text)), Chars: utf8.RuneCountInString(d.Text), Active: d.Active, Preview: preview, CreatedAt: d.CreatedAt}
}

// alwaysHandoff mirrors the brain's safety net, shown read-only in the dashboard.
var alwaysHandoff = []string{"Earnings, income and commissions", "Returns, profits and guarantees", "Payments, refunds and withdrawals", "Legal, tax and regulation", "Scam or pyramid questions"}
