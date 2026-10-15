// Package harness is John's editable "harness". Who John is (his profile:
// identity, voice, phrases, guardrails) is global; how he sells (his
// approach and reply style) comes from the active preset. The FAQ, referral
// link and uploaded sales playbook are shared by every preset. The package
// persists all of it in the store and pushes the active combination into
// the brain whenever anything changes.
package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"ureach/internal/brain"
	"ureach/internal/store"
)

// Persona is the full persona card the brain reads, the same shape as
// data/persona/john.json. It is the profile and a preset's approach merged.
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

// Profile is who John is, whatever preset he sells with.
type Profile struct {
	Name             string   `json:"name"`
	ShortName        string   `json:"short_name"`
	Role             string   `json:"role"`
	Background       []string `json:"background"`
	Values           []string `json:"values"`
	Voice            Voice    `json:"voice"`
	SignaturePhrases []string `json:"signature_phrases"`
	SampleLines      []string `json:"sample_lines"`
	Metaphors        []string `json:"metaphors"`
	Never            []string `json:"never"`
	Openers          []string `json:"openers"`
	Closers          []string `json:"closers"`
}

// Harness is what a preset holds: how John sells with it, in plain words,
// and his reply style.
type Harness struct {
	Approach string      `json:"approach"`
	Style    brain.Style `json:"style"`
}

// card is the persona card the brain reads: the profile plus a preset's approach.
type card struct {
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
	HowHeSells       string   `json:"how_he_sells"`
}

// Preset is a named sales approach John can switch to.
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
	Profile       Profile       `json:"profile"`
	ActiveID      int64         `json:"active_id"`
	Presets       []Preset      `json:"presets"`
	FAQ           string        `json:"faq"`
	ReferralURL   string        `json:"referral_url"`
	Documents     []DocInfo     `json:"documents"`
	PlaybookMax   int           `json:"playbook_max_chars"`
	AlwaysHandoff []string      `json:"always_handoff"` // at the current autonomy
	Autonomy      string        `json:"autonomy"`
	Levels        []brain.Level `json:"levels"`
}

const (
	settingActive   = "active_preset"
	settingProfile  = "profile"
	settingFAQ      = "faq"
	settingReferral = "referral_url"
	settingAutonomy = "autonomy"
	// SHA-256 of the data/faq.md the saved FAQ was copied from. A saved FAQ
	// that still matches it was never edited, so it follows the file.
	settingFAQShipped = "faq_shipped"

	// MaxDocChars caps one document (~30k tokens); MaxPlaybookChars caps all
	// active documents together so every reply stays fast and affordable.
	MaxDocChars      = 120_000
	MaxPlaybookChars = 200_000
)

// shippedFAQs are earlier versions of data/faq.md (SHA-256), for databases
// saved before settingFAQShipped existed: a saved FAQ matching one of them is
// an untouched copy, not John's own edit.
var shippedFAQs = map[string]bool{
	"339a4ae74508990207869cbcf3629403bc1ecc9dd10431e78a14357d0d7c3ece": true, // first FAQ, before the full knowledge base
}

// ErrInvalid wraps problems the user can fix (shown as-is in the dashboard).
var ErrInvalid = errors.New("invalid")

// Service owns the harness. All methods are safe for concurrent use.
type Service struct {
	st   *store.Store
	br   *brain.Brain
	base Persona // data/persona/john.json; the profile and built-ins start from it
	mu   sync.Mutex
}

// New loads John's base persona, seeds the profile, built-in presets, FAQ and
// referral link on first run, and configures the brain.
func New(st *store.Store, br *brain.Brain, personaPath, faqPath string) (*Service, error) {
	raw, err := os.ReadFile(personaPath)
	if err != nil {
		return nil, err
	}
	var base Persona
	if err := json.Unmarshal(raw, &base); err != nil {
		return nil, fmt.Errorf("%s: %w", personaPath, err)
	}
	s := &Service{st: st, br: br, base: base}

	if _, ok, err := st.Setting(settingProfile); err != nil {
		return nil, err
	} else if !ok {
		// Presets saved before the profile was split out carry the whole
		// persona; keep the active one's identity edits.
		p := profileOf(base)
		if id, err := s.activeID(); err == nil {
			if row, err := st.Preset(id); err == nil {
				if legacy, ok := legacyPersona(row.Harness); ok {
					p = profileOf(legacy)
				}
			}
		}
		if err := s.putProfile(p); err != nil {
			return nil, err
		}
	} else if p, err := s.profile(); err != nil {
		return nil, err
	} else if len(p.Openers) == 0 && len(p.Closers) == 0 {
		// Openers and closers used to live in each preset; they're John's own words.
		p.Openers, p.Closers = base.Openers, base.Closers
		if err := s.putProfile(p); err != nil {
			return nil, err
		}
	}

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
	if err := s.syncFAQ(faqPath); err != nil {
		return nil, err
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
	snap := Snapshot{PlaybookMax: MaxPlaybookChars, Autonomy: s.autonomy(), Levels: brain.Levels}
	snap.AlwaysHandoff = brain.LevelOf(snap.Autonomy).HandsOff
	var err error
	if snap.Profile, err = s.profile(); err != nil {
		return snap, err
	}
	rows, err := s.st.Presets()
	if err != nil {
		return snap, err
	}
	for _, r := range rows {
		p, err := s.toPreset(r)
		if err != nil {
			return snap, err
		}
		snap.Presets = append(snap.Presets, p)
	}
	if active, err := s.active(); err == nil {
		snap.ActiveID = active.ID
	} else {
		return snap, err
	}
	if snap.FAQ, _, err = s.st.Setting(settingFAQ); err != nil {
		return snap, err
	}
	if snap.ReferralURL, _, err = s.st.Setting(settingReferral); err != nil {
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

// ActivePersonaJSON is the persona card the brain is using, for the dashboard state.
func (s *Service) ActivePersonaJSON() json.RawMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, err := s.config(nil, nil)
	if err != nil {
		return json.RawMessage("null")
	}
	return json.RawMessage(cfg.Persona)
}

// FAQ is the current FAQ text.
func (s *Service) FAQ() string {
	v, _, _ := s.st.Setting(settingFAQ)
	return v
}

// syncFAQ keeps the saved FAQ in step with data/faq.md. On first run it
// copies the file in. Later, when the file changes, a saved copy John never
// edited is replaced with the new one; one he edited is kept.
func (s *Service) syncFAQ(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	file, fileSum := string(raw), sha(string(raw))
	saved, ok, err := s.st.Setting(settingFAQ)
	if err != nil {
		return err
	}
	from, _, err := s.st.Setting(settingFAQShipped)
	if err != nil {
		return err
	}
	switch {
	case !ok || saved == file:
	case sha(saved) == from || (from == "" && shippedFAQs[sha(saved)]):
		log.Printf("harness: facts updated to the new %s", path)
	default:
		if from != fileSum {
			log.Printf("harness: keeping John's edited facts; %s has changed since they were saved", path)
		}
		return nil
	}
	if err := s.st.SetSetting(settingFAQ, file); err != nil {
		return err
	}
	return s.st.SetSetting(settingFAQShipped, fileSum)
}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// SetProfile replaces who John is.
func (s *Service) SetProfile(p Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := p.validate(); err != nil {
		return err
	}
	if err := s.putProfile(p); err != nil {
		return err
	}
	return s.apply()
}

// SavePreset replaces a preset's approach and reply style.
func (s *Service) SavePreset(id int64, h Harness) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := h.validate(); err != nil {
		return err
	}
	row, err := s.st.Preset(id)
	if err != nil {
		return err
	}
	if err := s.updatePreset(id, row.Name, row.Description, h); err != nil {
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

// SaveAs adds a new preset with the given approach and makes it active.
func (s *Service) SaveAs(name, description string, h Harness) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, fmt.Errorf("%w: give the preset a name", ErrInvalid)
	}
	if err := h.validate(); err != nil {
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
	p, err := s.toPreset(row)
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

// SetAutonomy sets how much the AI handles before handing a chat to John.
func (s *Service) SetAutonomy(level string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if brain.NormalizeAutonomy(level) != level {
		return fmt.Errorf("%w: autonomy is low, medium or high", ErrInvalid)
	}
	if err := s.st.SetSetting(settingAutonomy, level); err != nil {
		return err
	}
	return s.apply()
}

func (s *Service) autonomy() string {
	v, _, _ := s.st.Setting(settingAutonomy)
	return brain.NormalizeAutonomy(v)
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

// PreviewConfig is the brain config with unsaved edits swapped in (nil
// keeps what's saved), for trying a draft without touching live replies.
func (s *Service) PreviewConfig(profile *Profile, preset *Harness) (brain.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if profile != nil {
		if err := profile.validate(); err != nil {
			return brain.Config{}, err
		}
	}
	if preset != nil {
		if err := preset.validate(); err != nil {
			return brain.Config{}, err
		}
	}
	return s.config(profile, preset)
}

// apply pushes the saved profile, active preset, FAQ, playbook and link into the brain.
func (s *Service) apply() error {
	cfg, err := s.config(nil, nil)
	if err != nil {
		return err
	}
	s.br.Configure(cfg)
	return nil
}

func (s *Service) config(profile *Profile, preset *Harness) (brain.Config, error) {
	if profile == nil {
		p, err := s.profile()
		if err != nil {
			return brain.Config{}, err
		}
		profile = &p
	}
	if preset == nil {
		a, err := s.active()
		if err != nil {
			return brain.Config{}, err
		}
		preset = &a.Harness
	}
	persona, err := json.MarshalIndent(merge(*profile, preset.Approach), "", "  ")
	if err != nil {
		return brain.Config{}, err
	}
	faq, _, err := s.st.Setting(settingFAQ)
	if err != nil {
		return brain.Config{}, err
	}
	referral, _, err := s.st.Setting(settingReferral)
	if err != nil {
		return brain.Config{}, err
	}
	docs, err := s.st.Documents()
	if err != nil {
		return brain.Config{}, err
	}
	var playbook []brain.Doc
	for _, d := range docs {
		if d.Active {
			playbook = append(playbook, brain.Doc{Name: d.Name, Text: d.Text})
		}
	}
	return brain.Config{Persona: string(persona), FAQ: faq, Playbook: playbook, Style: preset.Style, ReferralURL: referral, Autonomy: s.autonomy()}, nil
}

func (s *Service) profile() (Profile, error) {
	v, ok, err := s.st.Setting(settingProfile)
	if err != nil || !ok {
		return profileOf(s.base), err
	}
	var p Profile
	if err := json.Unmarshal([]byte(v), &p); err != nil {
		return p, fmt.Errorf("profile: %w", err)
	}
	return p, nil
}

func (s *Service) putProfile(p Profile) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.st.SetSetting(settingProfile, string(raw))
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
	return s.toPreset(row)
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

func (s *Service) toPreset(r store.Preset) (Preset, error) {
	p := Preset{ID: r.ID, Key: r.Key, Name: r.Name, Description: r.Description, UpdatedAt: r.UpdatedAt}
	var raw struct {
		Persona  *Persona        `json:"persona"`  // before the profile was split out
		Approach json.RawMessage `json:"approach"` // a string now; lists before
		Style    brain.Style     `json:"style"`
	}
	if err := json.Unmarshal([]byte(r.Harness), &raw); err != nil {
		return p, fmt.Errorf("preset %d: %w", r.ID, err)
	}
	p.Harness.Style = raw.Style
	switch {
	case raw.Persona != nil:
		p.Harness.Approach = s.approachText(raw.Persona.HowHeSells, raw.Persona.Openers, raw.Persona.Closers)
	case len(raw.Approach) > 0 && raw.Approach[0] == '{':
		var old struct {
			HowHeSells []string `json:"how_he_sells"`
			Openers    []string `json:"openers"`
			Closers    []string `json:"closers"`
		}
		json.Unmarshal(raw.Approach, &old)
		p.Harness.Approach = s.approachText(old.HowHeSells, old.Openers, old.Closers)
	default:
		json.Unmarshal(raw.Approach, &p.Harness.Approach)
	}
	p.Harness.normalize()
	return p, nil
}

// approachText turns an older list-shaped approach into the free text a
// preset holds now. Openers and closers that differ from John's own are kept
// in the text, since they now live in his profile.
func (s *Service) approachText(how, openers, closers []string) string {
	text := strings.Join(clean(how), "\n\n")
	if o := clean(openers); len(o) > 0 && !slices.Equal(o, clean(s.base.Openers)) {
		text += "\n\nWays he likes to open:\n- " + strings.Join(o, "\n- ")
	}
	if c := clean(closers); len(c) > 0 && !slices.Equal(c, clean(s.base.Closers)) {
		text += "\n\nWays he likes to close:\n- " + strings.Join(c, "\n- ")
	}
	return strings.TrimSpace(text)
}

// legacyPersona reads a preset saved before the profile was split out.
func legacyPersona(raw string) (Persona, bool) {
	var old struct {
		Persona *Persona `json:"persona"`
	}
	if json.Unmarshal([]byte(raw), &old) != nil || old.Persona == nil {
		return Persona{}, false
	}
	return *old.Persona, true
}

func profileOf(p Persona) Profile {
	return Profile{Name: p.Name, ShortName: p.ShortName, Role: p.Role, Background: p.Background, Values: p.Values, Voice: p.Voice,
		SignaturePhrases: p.SignaturePhrases, SampleLines: p.SampleLines, Metaphors: p.Metaphors, Never: p.Never,
		Openers: p.Openers, Closers: p.Closers}
}

func merge(p Profile, approach string) card {
	return card{Name: p.Name, ShortName: p.ShortName, Role: p.Role, Background: p.Background, Voice: p.Voice,
		Openers: p.Openers, SignaturePhrases: p.SignaturePhrases, Metaphors: p.Metaphors, Values: p.Values,
		Closers: p.Closers, Never: p.Never, SampleLines: p.SampleLines, HowHeSells: approach}
}

// validate trims the profile and checks what the brain relies on.
func (p *Profile) validate() error {
	p.Name, p.ShortName, p.Role = strings.TrimSpace(p.Name), strings.TrimSpace(p.ShortName), strings.TrimSpace(p.Role)
	if p.Name == "" {
		return fmt.Errorf("%w: John needs a name", ErrInvalid)
	}
	for _, l := range []*[]string{&p.Background, &p.Values, &p.SignaturePhrases, &p.SampleLines, &p.Metaphors, &p.Never, &p.Openers, &p.Closers} {
		*l = clean(*l)
	}
	return nil
}

// validate checks a preset before it's saved or tried.
func (h *Harness) validate() error {
	h.normalize()
	if h.Approach == "" {
		return fmt.Errorf("%w: describe how John sells with this preset", ErrInvalid)
	}
	return nil
}

func (h *Harness) normalize() {
	h.Approach = strings.TrimSpace(h.Approach)
	h.Style = h.Style.Normalized()
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
