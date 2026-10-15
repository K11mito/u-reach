package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"

	"ureach/internal/brain"
	"ureach/internal/store"
	"ureach/internal/voice"
)

// These tests run the real pipeline (store, brain, voice notes) against a
// stand-in for Claude and a fake WhatsApp, so nothing leaves the machine.
// The mock voice renders a tone with ffmpeg, which must be installed.

// fakeGateway records what would have gone to WhatsApp.
type fakeGateway struct {
	mu        sync.Mutex
	voice     int
	texts     []string
	failVoice error
}

func (g *fakeGateway) SendText(_ context.Context, _ types.JID, text string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.texts = append(g.texts, text)
	return "text-" + time.Now().Format("150405.000000000"), nil
}

func (g *fakeGateway) SendVoiceNote(context.Context, types.JID, voice.VoiceNote) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.failVoice != nil {
		return "", g.failVoice
	}
	g.voice++
	return "voice-" + time.Now().Format("150405.000000000"), nil
}

func (g *fakeGateway) Recording(types.JID) func()    { return func() {} }
func (g *fakeGateway) MarkRead(context.Context, Msg) {}

func (g *fakeGateway) sent() (voice int, texts []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.voice, append([]string(nil), g.texts...)
}

// fakeClaude answers every request with reply, or fails with status.
type fakeClaude struct {
	reply  brain.Reply
	status atomic.Int32 // non-zero: answer with this HTTP error
	calls  atomic.Int32
}

func (f *fakeClaude) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.calls.Add(1)
	if s := f.status.Load(); s != 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(int(s))
		w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"Your credit balance is too low"}}`))
		return
	}
	text, _ := json.Marshal(f.reply)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id": "msg_test", "type": "message", "role": "assistant", "model": "claude-test",
		"content":     []map[string]any{{"type": "text", "text": string(text)}},
		"stop_reason": "end_turn", "usage": map[string]int{"input_tokens": 10, "output_tokens": 10},
	})
}

type rig struct {
	core   *Core
	store  *store.Store
	wa     *fakeGateway
	claude *fakeClaude
}

func newRig(t *testing.T) *rig {
	t.Helper()
	claude := &fakeClaude{reply: brain.Reply{SpokenScript: "Hey Raj! U-topia is one free app for your money, and you hold it yourself. I'm sending you my link now.", IncludeReferral: true}}
	srv := httptest.NewServer(claude)
	t.Cleanup(srv.Close)
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
	t.Setenv("ANTHROPIC_API_KEY", "test")
	t.Setenv("BRAIN_MODEL", "claude-test")

	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	br, err := brain.New("../../data/persona/john.json", "../../data/faq.md")
	if err != nil {
		t.Fatal(err)
	}
	br.Configure(brain.Config{Persona: "{}", FAQ: "U-topia is a money app.", Style: brain.DefaultStyle, ReferralURL: "https://u.center/?ref=john"})
	wa := &fakeGateway{}
	c := &Core{
		Store: st, Brain: br, Voice: &voice.Eleven{Mock: true, DataDir: dir}, Gateway: wa, Hub: NewHub(),
		AudioDir: dir, Debounce: 50 * time.Millisecond, MaxVoicePerHour: 10,
	}
	return &rig{core: c, store: st, wa: wa, claude: claude}
}

var leadJID = types.NewJID("66800000001", types.DefaultUserServer)

func (r *rig) lead(text string) {
	r.core.Handle(Msg{WAID: "in-" + text, Chat: leadJID, From: leadJID, Phone: leadJID.User, PushName: "Raj Patel", Text: text})
}

// waitRun waits for the newest run to finish and returns it.
func (r *rig) waitRun(t *testing.T) store.Run {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if runs, _ := r.store.Runs(1); len(runs) == 1 && runs[0].Status != "running" {
			time.Sleep(50 * time.Millisecond) // let the run's last writes land
			return runs[0]
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no finished run")
	return store.Run{}
}

func (r *rig) contact(t *testing.T) store.Contact {
	t.Helper()
	c, err := r.store.ContactByJID(leadJID.String())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestReplySendsVoiceNoteThenLink(t *testing.T) {
	r := newRig(t)
	r.lead("hi John, what is U-topia?")
	run := r.waitRun(t)
	if run.Status != "sent" || run.SentAt == 0 {
		t.Fatalf("run: %+v", run)
	}
	voiceNotes, texts := r.wa.sent()
	if voiceNotes != 1 || len(texts) != 1 || !strings.HasSuffix(texts[0], "https://u.center/?ref=john") {
		t.Fatalf("want one voice note then the link, got %d voice notes and texts %q", voiceNotes, texts)
	}
	if c := r.contact(t); c.Status != store.StatusAuto || c.LinkSentAt == 0 {
		t.Fatalf("contact should be with the AI and marked as having the link: %+v", c)
	}
}

func TestBurstGetsOneReply(t *testing.T) {
	r := newRig(t)
	r.core.Debounce = 300 * time.Millisecond
	for _, s := range []string{"hey", "John?", "what is U-topia"} {
		r.lead(s)
		time.Sleep(30 * time.Millisecond)
	}
	r.waitRun(t)
	if n := r.claude.calls.Load(); n != 1 {
		t.Fatalf("three quick messages should get one reply, Claude was called %d times", n)
	}
	if runs, _ := r.store.Runs(10); len(runs) != 1 {
		t.Fatalf("want one run, got %d", len(runs))
	}
}

func TestTakenOverChatStaysQuiet(t *testing.T) {
	r := newRig(t)
	r.lead("hello")
	r.waitRun(t)
	if err := r.core.SetStatus(r.contact(t).ID, store.StatusHuman); err != nil {
		t.Fatal(err)
	}
	calls := r.claude.calls.Load()
	r.lead("are you there?")
	time.Sleep(300 * time.Millisecond)
	if r.claude.calls.Load() != calls {
		t.Fatal("the AI must not reply in a chat John took over")
	}
}

func TestVoiceNoteFailureStillAnswers(t *testing.T) {
	r := newRig(t)
	r.wa.failVoice = errors.New("upload: media server unavailable")
	r.lead("what is U-topia?")
	run := r.waitRun(t)
	_, texts := r.wa.sent()
	if run.Status != "sent" || len(texts) != 2 || !strings.HasPrefix(texts[0], "Hey Raj!") || !strings.HasSuffix(texts[1], "ref=john") {
		t.Fatalf("a failed voice note should go out as text, then the link: run %q, texts %q", run.Status, texts)
	}
}

func TestBrainDownSendsHoldingNoteAndHandsOff(t *testing.T) {
	r := newRig(t)
	r.claude.status.Store(http.StatusBadRequest)
	r.lead("what is U-topia?")
	run := r.waitRun(t)
	if run.Status != "failed" || !strings.Contains(run.Error, "brain") {
		t.Fatalf("run: %+v", run)
	}
	if n, _ := r.wa.sent(); n != 1 {
		t.Fatalf("the lead should get the holding voice note, got %d voice notes", n)
	}
	if c := r.contact(t); c.Status != store.StatusNeedsJohn {
		t.Fatalf("the chat should wait for John: %+v", c)
	}
}

func TestHandoffPausesTheAI(t *testing.T) {
	r := newRig(t)
	r.lead("I paid yesterday and want a refund")
	run := r.waitRun(t)
	c := r.contact(t)
	if run.Status != "handoff" || c.Status != store.StatusNeedsJohn || c.LinkSentAt != 0 {
		t.Fatalf("a refund request should hand off without the link: run %q, contact %+v", run.Status, c)
	}
	if _, texts := r.wa.sent(); len(texts) != 0 {
		t.Fatalf("no link or text on a handoff, got %q", texts)
	}
}

func TestReplyAsJohnToDemoLeadIsNeverSent(t *testing.T) {
	r := newRig(t)
	demo, err := r.store.CreateContact(store.Contact{JID: "990000000042@s.whatsapp.net", Phone: "990000000042", Name: "Ben", Status: store.StatusNeedsJohn})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.core.SendAsJohn(context.Background(), demo.ID, "Hi Ben, John here!"); err != nil {
		t.Fatal(err)
	}
	if _, texts := r.wa.sent(); len(texts) != 0 {
		t.Fatalf("a made-up number must never be messaged, sent %q", texts)
	}
	msgs, _ := r.store.Messages(demo.ID, 10)
	if len(msgs) != 1 || msgs[0].Source != "john" || msgs[0].Text != "Hi Ben, John here!" {
		t.Fatalf("the reply should still show in the chat: %+v", msgs)
	}

	// A real lead does get it.
	r.lead("hello")
	r.waitRun(t)
	_, before := r.wa.sent()
	if err := r.core.SendAsJohn(context.Background(), r.contact(t).ID, "John here, calling you now"); err != nil {
		t.Fatal(err)
	}
	if _, after := r.wa.sent(); len(after) != len(before)+1 || after[len(after)-1] != "John here, calling you now" {
		t.Fatalf("a real lead should get John's text: %q", after)
	}
}
