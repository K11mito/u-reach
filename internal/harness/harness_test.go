package harness

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ureach/internal/brain"
	"ureach/internal/store"
)

func newService(t *testing.T) (*Service, *brain.Brain, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	br := &brain.Brain{}
	br.Configure(brain.Config{ReferralURL: "https://u.center/?ref=john"})
	s, err := New(st, br, "../../data/persona/john.json", "../../data/faq.md")
	if err != nil {
		t.Fatal(err)
	}
	return s, br, st
}

func TestSeedsProfilePresetsAndConfiguresBrain(t *testing.T) {
	s, br, _ := newService(t)
	snap, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Presets) != 4 || snap.ActiveID != snap.Presets[0].ID || snap.Presets[0].Key != "warm" {
		t.Fatalf("want 4 built-ins with Warm friend active, got %d active=%d", len(snap.Presets), snap.ActiveID)
	}
	if snap.Profile.Name != "John Klensch" || len(snap.Profile.SignaturePhrases) == 0 {
		t.Fatalf("profile should be seeded from john.json: %+v", snap.Profile)
	}
	if snap.ReferralURL != "https://u.center/?ref=john" || !strings.Contains(snap.FAQ, "U-topia") {
		t.Fatalf("referral and FAQ should be seeded: %q", snap.ReferralURL)
	}
	if p := br.SystemPrompt(); !strings.Contains(p, "John Klensch") || !strings.Contains(p, "non-custodial") || strings.Contains(p, "<sales_playbook>") {
		t.Fatal("system prompt should carry the persona and FAQ, and no playbook yet")
	}
}

func TestProfileAndPresetsReachTheBrain(t *testing.T) {
	s, br, _ := newService(t)
	snap, _ := s.Snapshot()

	prof := snap.Profile
	prof.SignaturePhrases = append(prof.SignaturePhrases, "  Let's make it happen.  ", "")
	if err := s.SetProfile(prof); err != nil {
		t.Fatal(err)
	}
	h := snap.Presets[0].Harness
	h.Approach += "\n\nHe always mentions the Bangkok meetup."
	h.Style = brain.Style{MinWords: 30, MaxWords: 45, LinkTiming: brain.LinkWhenAsked}
	if err := s.SavePreset(snap.Presets[0].ID, h); err != nil {
		t.Fatal(err)
	}
	p := br.SystemPrompt()
	for _, want := range []string{"Let's make it happen.", "He always mentions the Bangkok meetup.", "30 to 45 words", "Don't push your link", "Reply in English."} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
	if st := br.Style(); st.LinkTiming != brain.LinkWhenAsked || st.MaxWords != 45 {
		t.Fatalf("brain style not updated: %+v", st)
	}
	saved, _ := s.Snapshot()
	if got := saved.Profile.SignaturePhrases; got[len(got)-1] != "Let's make it happen." {
		t.Fatalf("items should be trimmed and blanks dropped: %q", got[len(got)-2:])
	}

	// Editing a preset that isn't in use doesn't change live replies.
	call := snap.Presets[3]
	call.Harness.Approach = "He always closes with: Ring me tonight?"
	if err := s.SavePreset(call.ID, call.Harness); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(br.SystemPrompt(), "Ring me tonight?") {
		t.Fatal("an inactive preset must not reach live replies")
	}
	if err := s.Activate(call.ID); err != nil {
		t.Fatal(err)
	}
	if p := br.SystemPrompt(); !strings.Contains(p, "Ring me tonight?") || !strings.Contains(p, "Let's make it happen.") {
		t.Fatal("switching preset changes the approach and keeps the profile")
	}
}

func TestPreviewConfigUsesDraftsWithoutSaving(t *testing.T) {
	s, br, _ := newService(t)
	snap, _ := s.Snapshot()
	draft := snap.Presets[1].Harness
	draft.Approach = "He always opens with Yo {name}!"
	prof := snap.Profile
	prof.Role = "Draft role"
	cfg, err := s.PreviewConfig(&prof, &draft)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cfg.Persona, "Yo {name}!") || !strings.Contains(cfg.Persona, "Draft role") || cfg.Style.MaxWords != 60 {
		t.Fatalf("preview should use both drafts: %s", cfg.Persona)
	}
	if strings.Contains(br.SystemPrompt(), "Yo {name}!") || strings.Contains(br.SystemPrompt(), "Draft role") {
		t.Fatal("previews must not touch live replies")
	}
	bad := Profile{}
	if _, err := s.PreviewConfig(&bad, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("a nameless profile is invalid")
	}
}

func TestLegacyPresetsKeepTheirEdits(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	// A preset saved before the profile was split out.
	id, _ := st.AddPreset(store.Preset{Key: "warm", Name: "Warm friend", Harness: `{"persona":{"name":"John K","role":"Edited role","how_he_sells":["Old step"],"closers":["Old closer"]},"style":{"min_words":30,"max_words":45,"link_timing":"when_asked","language":"english"}}`})
	st.SetSetting(settingActive, "1")
	br := &brain.Brain{}
	s, err := New(st, br, "../../data/persona/john.json", "../../data/faq.md")
	if err != nil {
		t.Fatal(err)
	}
	snap, _ := s.Snapshot()
	if snap.Profile.Role != "Edited role" || snap.Presets[0].ID != id || snap.Presets[0].Harness.Approach != "Old step\n\nWays he likes to close:\n- Old closer" || snap.Presets[0].Harness.Style.LinkTiming != brain.LinkWhenAsked {
		t.Fatalf("legacy preset should convert: %+v %+v", snap.Profile.Role, snap.Presets[0].Harness)
	}
}

func TestPlaybookDocuments(t *testing.T) {
	s, br, _ := newService(t)
	doc, err := s.AddDocument("objections.md", []byte("\ufeff# Objections\r\n\r\n\r\n\r\nIf they say it's too good to be true, share your own story first.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Active || doc.Kind != "md" || doc.Words == 0 {
		t.Fatalf("doc should be active markdown: %+v", doc)
	}
	p := br.SystemPrompt()
	if !strings.Contains(p, "<sales_playbook>") || !strings.Contains(p, `<document name="objections.md">`) || !strings.Contains(p, "share your own story first") {
		t.Fatal("active document should be in the prompt")
	}
	if err := s.SetDocumentActive(doc.ID, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(br.SystemPrompt(), "share your own story") {
		t.Fatal("inactive document must leave the prompt")
	}
	if err := s.DeleteDocument(doc.ID); err != nil {
		t.Fatal(err)
	}

	for name, data := range map[string][]byte{
		"deck.docx": []byte("PK\x03\x04"),
		"empty.md":  []byte("  \n "),
		"bad.pdf":   []byte("%PDF-1.4 not really a pdf"),
		"big.md":    []byte(strings.Repeat("word ", MaxDocChars/5+10)),
	} {
		if _, err := s.AddDocument(name, data); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s: want ErrInvalid, got %v", name, err)
		}
	}
}

func TestPresetLifecycle(t *testing.T) {
	s, _, _ := newService(t)
	snap, _ := s.Snapshot()
	warm := snap.Presets[0]
	if err := s.Delete(warm.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("built-ins can't be deleted: %v", err)
	}
	id, err := s.SaveAs("Bangkok push", "For the meetup crowd", warm.Harness)
	if err != nil {
		t.Fatal(err)
	}
	if snap, _ = s.Snapshot(); snap.ActiveID != id {
		t.Fatal("save-as should switch to the new preset")
	}
	if _, err := s.SaveAs("  ", "", warm.Harness); !errors.Is(err, ErrInvalid) {
		t.Fatal("a preset needs a name")
	}
	if err := s.Delete(id); err != nil {
		t.Fatal(err)
	}
	if snap, _ = s.Snapshot(); snap.ActiveID != warm.ID {
		t.Fatalf("deleting the active preset should fall back to the first one, got %d", snap.ActiveID)
	}

	if err := s.Rename(warm.ID, "My style", "changed"); err != nil {
		t.Fatal(err)
	}
	if err := s.Reset(warm.ID); err != nil {
		t.Fatal(err)
	}
	if snap, _ = s.Snapshot(); snap.Presets[0].Name != "Warm friend" {
		t.Fatalf("reset should restore the built-in: %q", snap.Presets[0].Name)
	}

	if err := s.SetReferral("u.center/john"); !errors.Is(err, ErrInvalid) {
		t.Fatal("referral link must be a URL")
	}
	if err := s.SetFAQ(" "); !errors.Is(err, ErrInvalid) {
		t.Fatal("FAQ can't be empty")
	}
}

func TestAutonomySetting(t *testing.T) {
	s, br, _ := newService(t)
	snap, _ := s.Snapshot()
	if snap.Autonomy != brain.AutonomyMedium || len(snap.Levels) != 3 || snap.AlwaysHandoff[0] != "Payments, refunds or accounts gone wrong" {
		t.Fatalf("default should be balanced: %q %v", snap.Autonomy, snap.AlwaysHandoff)
	}
	if err := s.SetAutonomy(brain.AutonomyHigh); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(br.SystemPrompt(), "John trusts you with the whole conversation") {
		t.Fatal("full autonomy should reach the prompt")
	}
	if snap, _ = s.Snapshot(); snap.Autonomy != brain.AutonomyHigh || len(snap.AlwaysHandoff) != 3 || !strings.Contains(snap.AlwaysHandoff[1], "Legal threats") {
		t.Fatalf("snapshot should show the new level: %q %v", snap.Autonomy, snap.AlwaysHandoff)
	}
	// Previews use the live autonomy too.
	cfg, _ := s.PreviewConfig(nil, nil)
	if cfg.Autonomy != brain.AutonomyHigh {
		t.Fatal("preview should use the saved autonomy")
	}
	if err := s.SetAutonomy("reckless"); !errors.Is(err, ErrInvalid) {
		t.Fatal("unknown level must be rejected")
	}
}

// Presets saved with list-shaped approaches (before the free-text editor)
// become text, and John's openers and closers move into his profile.
func TestListApproachBecomesText(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "lists.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.SetSetting(settingProfile, `{"name":"John Klensch","role":"Ambassador"}`)
	id, _ := st.AddPreset(store.Preset{Key: "curious", Name: "Curious first", Harness: `{"approach":{"how_he_sells":["Ask first.","Then pitch."],"openers":["Hey {name}! John here, so good to hear from you.","Hi {name}! Thanks for reaching out, I love that you asked.","Hey {name}, John here. Great to hear from you."],"closers":["Where are you based?"]},"style":{"min_words":35,"max_words":60,"link_timing":"when_interested","language":"match"}}`})
	st.SetSetting(settingActive, "1")
	br := &brain.Brain{}
	s, err := New(st, br, "../../data/persona/john.json", "../../data/faq.md")
	if err != nil {
		t.Fatal(err)
	}
	snap, _ := s.Snapshot()
	if got := snap.Presets[0].Harness.Approach; snap.Presets[0].ID != id || got != "Ask first.\n\nThen pitch.\n\nWays he likes to close:\n- Where are you based?" {
		t.Fatalf("list approach should become text (John's own openers dropped): %q", got)
	}
	if len(snap.Profile.Openers) == 0 || len(snap.Profile.Closers) == 0 {
		t.Fatal("profile should get John's openers and closers")
	}
	if p := br.SystemPrompt(); !strings.Contains(p, `"how_he_sells": "Ask first.`) || !strings.Contains(p, "Hey {name}! John here") {
		t.Fatal("prompt should carry the approach text and the profile's openers")
	}
	if err := s.SavePreset(id, Harness{Approach: "   "}); !errors.Is(err, ErrInvalid) {
		t.Fatal("an empty approach is invalid")
	}
}

// The saved facts follow data/faq.md until John edits them.
func TestFAQFollowsShippedFileUntilEdited(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	faqPath := filepath.Join(dir, "faq.md")
	// start runs the server with this faq.md and returns the harness.
	start := func(file string) *Service {
		t.Helper()
		if err := os.WriteFile(faqPath, []byte(file), 0o644); err != nil {
			t.Fatal(err)
		}
		br := &brain.Brain{}
		br.Configure(brain.Config{})
		s, err := New(st, br, "../../data/persona/john.json", faqPath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(br.SystemPrompt(), s.FAQ()) {
			t.Fatal("the brain should get the saved facts")
		}
		return s
	}

	if got := start("# Facts v1").FAQ(); got != "# Facts v1" {
		t.Fatalf("first run copies the file: %q", got)
	}
	s := start("# Facts v2")
	if got := s.FAQ(); got != "# Facts v2" {
		t.Fatalf("an unedited copy follows the new file: %q", got)
	}
	if err := s.SetFAQ("# John's own facts"); err != nil {
		t.Fatal(err)
	}
	if got := start("# Facts v3").FAQ(); got != "# John's own facts" {
		t.Fatalf("John's edits survive a new file: %q", got)
	}

	// Databases from before the shipped version was recorded.
	legacy := func(saved string) string {
		t.Helper()
		st.SetSetting(settingFAQ, saved)
		st.SetSetting(settingFAQShipped, "")
		return start("# Facts v4").FAQ()
	}
	shippedFAQs[sha("# Old shipped facts")] = true
	t.Cleanup(func() { delete(shippedFAQs, sha("# Old shipped facts")) })
	if got := legacy("# Old shipped facts"); got != "# Facts v4" {
		t.Fatalf("an old shipped copy is upgraded: %q", got)
	}
	if got := legacy("# Something John wrote"); got != "# Something John wrote" {
		t.Fatalf("an unknown saved FAQ is John's and is kept: %q", got)
	}
}

// A database from before the AI kept conversations going: Warm friend sent
// the link in the first reply and John's closers all pushed it. One startup
// moves it to "once ready" (keeping its length) and swaps the closers, once.
func TestKeepTalkingUpdate(t *testing.T) {
	s, br, st := newService(t)
	snap, _ := s.Snapshot()
	warm, direct := snap.Presets[0], snap.Presets[2]
	if warm.Harness.Style.LinkTiming != brain.LinkWhenInterested || direct.Harness.Style.LinkTiming != brain.LinkFirstReply {
		t.Fatalf("new installs: Warm friend once ready, Straight to the point right away: %+v %+v", warm.Harness.Style, direct.Harness.Style)
	}

	// Roll back to how a live database looked, with John's shorter notes.
	warm.Harness.Style = brain.Style{MinWords: 25, MaxWords: 45, LinkTiming: brain.LinkFirstReply}
	s.updatePreset(warm.ID, warm.Name, oldWarmDescription, warm.Harness)
	prof := snap.Profile
	prof.Closers = oldClosers
	s.putProfile(prof)
	st.SetSetting(settingKeepTalking, "")

	s2, err := New(st, br, "../../data/persona/john.json", "../../data/faq.md")
	if err != nil {
		t.Fatal(err)
	}
	snap, _ = s2.Snapshot()
	got := snap.Presets[0]
	if got.Harness.Style != (brain.Style{MinWords: 25, MaxWords: 45, LinkTiming: brain.LinkWhenInterested}) || got.Description == oldWarmDescription {
		t.Fatalf("Warm friend should send the link once ready and keep its length: %+v %q", got.Harness.Style, got.Description)
	}
	if snap.Presets[2].Harness.Style.LinkTiming != brain.LinkFirstReply {
		t.Fatal("Straight to the point keeps sending the link right away")
	}
	if strings.Contains(strings.Join(snap.Profile.Closers, " "), "sending you my link right now") {
		t.Fatalf("the old closers should be replaced: %q", snap.Profile.Closers)
	}

	// It runs once: John's later choices stick.
	got.Harness.Style.LinkTiming = brain.LinkFirstReply
	s2.updatePreset(got.ID, got.Name, got.Description, got.Harness)
	if _, err := New(st, br, "../../data/persona/john.json", "../../data/faq.md"); err != nil {
		t.Fatal(err)
	}
	if snap, _ = s2.Snapshot(); snap.Presets[0].Harness.Style.LinkTiming != brain.LinkFirstReply {
		t.Fatal("after the update, John's own link timing is kept")
	}
}
