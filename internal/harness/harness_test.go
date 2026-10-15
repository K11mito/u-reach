package harness

import (
	"errors"
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

func TestSeedsPresetsAndConfiguresBrain(t *testing.T) {
	s, br, _ := newService(t)
	snap, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Presets) != 4 || snap.ActiveID != snap.Presets[0].ID || snap.Presets[0].Key != "warm" {
		t.Fatalf("want 4 built-ins with Warm friend active, got %d active=%d", len(snap.Presets), snap.ActiveID)
	}
	if snap.ReferralURL != "https://u.center/?ref=john" || !strings.Contains(snap.FAQ, "U-topia") {
		t.Fatalf("referral and FAQ should be seeded: %q", snap.ReferralURL)
	}
	if p := br.SystemPrompt(); !strings.Contains(p, "John Klensch") || !strings.Contains(p, "non-custodial") || strings.Contains(p, "<sales_playbook>") {
		t.Fatal("system prompt should carry the persona and FAQ, and no playbook yet")
	}
}

func TestEditsAndPresetsReachTheBrain(t *testing.T) {
	s, br, _ := newService(t)
	snap, _ := s.Snapshot()
	h := snap.Presets[0].Harness
	h.Persona.HowHeSells = append(h.Persona.HowHeSells, "  He always mentions the Bangkok meetup.  ", "")
	h.Style = brain.Style{MinWords: 30, MaxWords: 45, LinkTiming: brain.LinkWhenAsked, Language: brain.LangMatch}
	if err := s.SaveActive(h); err != nil {
		t.Fatal(err)
	}
	p := br.SystemPrompt()
	for _, want := range []string{"He always mentions the Bangkok meetup.", "30 to 45 words", "Don't push your link", "language the lead writes in"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
	if st := br.Style(); st.LinkTiming != brain.LinkWhenAsked || st.MaxWords != 45 {
		t.Fatalf("brain style not updated: %+v", st)
	}
	saved, _ := s.Snapshot()
	if got := saved.Presets[0].Harness.Persona.HowHeSells; got[len(got)-1] != "He always mentions the Bangkok meetup." {
		t.Fatalf("list items should be trimmed and blanks dropped: %q", got[len(got)-3:])
	}

	call := snap.Presets[3]
	if err := s.Activate(call.ID); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(br.SystemPrompt(), "three-way call on WhatsApp with him and someone experienced") {
		t.Fatal("switching preset should change John's sales approach")
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
