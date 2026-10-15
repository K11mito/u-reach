// Package core is the orchestrator: it debounces a lead's burst of messages,
// runs one reply at a time per chat (hear, think, speak, send), records every
// stage, and streams it all to the dashboard.
package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"

	"ureach/internal/brain"
	"ureach/internal/store"
	"ureach/internal/voice"
)

// Gateway is how the core talks to WhatsApp. cmd/server adapts wa.Client to
// it; dev mode uses a gateway that only logs.
type Gateway interface {
	SendText(ctx context.Context, to types.JID, text string) (string, error)
	SendVoiceNote(ctx context.Context, to types.JID, vn voice.VoiceNote) (string, error)
	Recording(chat types.JID) (stop func())
	MarkRead(ctx context.Context, m Msg)
}

// Msg is an inbound message from a lead.
type Msg struct {
	WAID     string
	Chat     types.JID // reply here
	From     types.JID // identity: phone JID if known, else LID
	Phone    string
	PushName string
	Text     string
	// Voice downloads the voice note (nil for text messages).
	Voice func(ctx context.Context) ([]byte, error)
	// Simulated messages come from the dashboard's test box: the pipeline runs
	// for real (brain, TTS) but nothing is sent to WhatsApp.
	Simulated bool
	Raw       any // gateway-specific (wa.Inbound), for read receipts
	At        time.Time
}

// Core wires the pipeline together.
type Core struct {
	Store    *store.Store
	Brain    *brain.Brain
	Voice    *voice.Eleven
	Gateway  Gateway
	Hub      *Hub
	AudioDir string // data/audio

	VoiceID  string
	TTSModel string
	Settings voice.Settings
	Debounce time.Duration
	// MaxVoicePerHour caps AI voice notes per contact; beyond it replies go out as text.
	MaxVoicePerHour int

	mu    sync.Mutex
	chats map[int64]*chat
}

type chat struct {
	pending  []pending
	timer    *time.Timer
	running  bool
	voiceLog []time.Time
}

type pending struct {
	msg   Msg
	msgID int64
}

// Handle is called for every inbound message. It returns quickly.
func (c *Core) Handle(m Msg) {
	ctx := context.Background()
	if m.At.IsZero() {
		m.At = time.Now()
	}
	contact, created, err := c.Store.UpsertInbound(m.From.String(), m.Chat.String(), m.Phone, m.PushName)
	if err != nil {
		log.Printf("core: upsert contact: %v", err)
		return
	}
	if created {
		c.Hub.Publish("contact", contact)
	}

	kind := "text"
	if m.Voice != nil {
		kind = "voice"
	}
	msg, err := c.Store.AddMessage(store.Message{
		ContactID: contact.ID, WAID: m.WAID, Direction: "in", Kind: kind, Source: "lead",
		Text: m.Text, CreatedAt: m.At.UnixMilli(),
	})
	if errors.Is(err, store.ErrDuplicate) {
		return
	}
	if err != nil {
		log.Printf("core: store message: %v", err)
		return
	}
	c.Hub.Publish("message", msg)
	if !m.Simulated {
		c.Gateway.MarkRead(ctx, m)
	}

	if contact.Status == store.StatusHuman || contact.Status == store.StatusNeedsJohn {
		log.Printf("core: %s is with John (%s); not auto-replying", contact.DisplayName(), contact.Status)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.chats == nil {
		c.chats = map[int64]*chat{}
	}
	ch := c.chats[contact.ID]
	if ch == nil {
		ch = &chat{}
		c.chats[contact.ID] = ch
	}
	ch.pending = append(ch.pending, pending{msg: m, msgID: msg.ID})
	if ch.timer != nil {
		ch.timer.Stop()
	}
	ch.timer = time.AfterFunc(c.Debounce, func() { c.fire(contact.ID) })
}

// fire starts a run for a chat unless one is in flight; the in-flight run
// picks up anything that arrived meanwhile when it finishes.
func (c *Core) fire(contactID int64) {
	c.mu.Lock()
	ch := c.chats[contactID]
	if ch == nil || ch.running || len(ch.pending) == 0 {
		c.mu.Unlock()
		return
	}
	batch := ch.pending
	ch.pending = nil
	ch.running = true
	c.mu.Unlock()

	go func() {
		c.run(contactID, batch)
		c.mu.Lock()
		ch.running = false
		again := len(ch.pending) > 0
		c.mu.Unlock()
		if again {
			c.fire(contactID)
		}
	}()
}

func (c *Core) run(contactID int64, batch []pending) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	contact, err := c.Store.Contact(contactID)
	if err != nil {
		log.Printf("core: load contact: %v", err)
		return
	}
	// Taken over while we were debouncing.
	if contact.Status == store.StatusHuman || contact.Status == store.StatusNeedsJohn {
		return
	}
	last := batch[len(batch)-1].msg
	simulated := last.Simulated
	chatJID := last.Chat

	runID, err := c.Store.NewRun(contactID, batch[0].msg.At.UnixMilli())
	if err != nil {
		log.Printf("core: new run: %v", err)
		return
	}
	c.publishRun(runID)

	stopRec := func() {}
	if !simulated {
		stopRec = c.Gateway.Recording(chatJID)
	}
	defer stopRec()

	fail := func(stage string, err error) {
		log.Printf("core: run %d failed at %s: %v", runID, stage, err)
		c.Store.FinishRun(runID, "failed", stage+": "+err.Error(), "", "")
		c.publishRun(runID)
	}

	// Heard: transcribe voice notes.
	for _, p := range batch {
		if p.msg.Voice == nil {
			continue
		}
		text, audioPath, durMS, err := c.hear(ctx, p)
		if err != nil {
			log.Printf("core: transcribe: %v", err)
			text = "(voice note could not be transcribed)"
		}
		c.Store.SetMessageText(p.msgID, text)
		if audioPath != "" {
			c.Store.SetMessageAudio(p.msgID, audioPath, durMS)
		}
		c.publishMessage(contactID, p.msgID)
	}
	c.stage(runID, "heard")

	// Thought: the brain writes the reply.
	history, err := c.history(contactID)
	if err != nil {
		fail("history", err)
		return
	}
	bc := brain.Contact{Name: firstName(contact.DisplayName()), Context: contact.Context, Memory: contact.Memory, LinkSent: contact.LinkSentAt > 0}
	reply, meta, err := c.Brain.Reply(ctx, bc, history)
	if err != nil {
		log.Printf("core: brain failed, retrying once: %v", err)
		reply, meta, err = c.Brain.Reply(ctx, bc, history)
	}
	if err != nil {
		c.fallback(ctx, runID, contact, chatJID, simulated, err)
		return
	}
	brainJSON, _ := json.Marshal(reply)
	c.Store.FinishRun(runID, "running", "", meta.Model, string(brainJSON))
	c.stage(runID, "thought")
	log.Printf("core: run %d brain %s in %s (%d words, handoff=%v, link=%v)", runID, meta.Model, meta.Latency.Round(time.Millisecond), meta.Words, reply.Handoff, reply.IncludeReferral)

	// Spoken: TTS and voice-note prep. Over the hourly cap, reply as text.
	var vn voice.VoiceNote
	var mp3Rel string
	spoke := c.allowVoice(contactID)
	if spoke {
		vn, mp3Rel, err = c.speak(ctx, runID, reply.SpokenScript)
		if err != nil {
			log.Printf("core: tts failed, sending text instead: %v", err)
			spoke = false
		}
	}
	c.stage(runID, "spoken")

	// Sent.
	if spoke {
		waID := ""
		if !simulated {
			waID, err = c.Gateway.SendVoiceNote(ctx, chatJID, vn)
			if err != nil {
				fail("send voice", err)
				return
			}
		}
		c.addOut(contactID, runID, waID, "voice", reply.SpokenScript, mp3Rel, int64(vn.Duration*1000))
	} else {
		if err := c.sendText(ctx, contactID, runID, chatJID, simulated, reply.SpokenScript); err != nil {
			fail("send text", err)
			return
		}
	}
	stopRec()
	if reply.FollowupText != "" {
		time.Sleep(1200 * time.Millisecond) // a beat between the note and the text, like a person
		if err := c.sendText(ctx, contactID, runID, chatJID, simulated, reply.FollowupText); err != nil {
			log.Printf("core: follow-up text: %v", err)
		}
	}
	c.stage(runID, "sent")

	if reply.IncludeReferral {
		c.Store.MarkLinkSent(contactID)
	}
	c.Store.Remember(contactID, reply.Remember)
	status := "sent"
	if reply.OptOut {
		status = "opt_out"
		c.Store.SetStatus(contactID, store.StatusNeedsJohn, "Asked not to be contacted")
		c.Store.AddEvent(contactID, "opt_out", nil)
	} else if reply.Handoff {
		status = "handoff"
		c.Store.SetStatus(contactID, store.StatusNeedsJohn, reply.HandoffReason)
		c.Store.AddEvent(contactID, "handoff", map[string]string{"reason": reply.HandoffReason})
	} else if contact.Status == store.StatusNew {
		c.Store.SetStatus(contactID, store.StatusAuto, "")
	}
	c.Store.AddEvent(contactID, "reply_sent", map[string]any{"run_id": runID, "voice": spoke, "link": reply.IncludeReferral})
	c.Store.FinishRun(runID, status, "", meta.Model, string(brainJSON))
	c.publishRun(runID)
	c.publishContact(contactID)
}

// hear downloads and transcribes one inbound voice note.
func (c *Core) hear(ctx context.Context, p pending) (text, mp3Rel string, durMS int64, err error) {
	data, err := p.msg.Voice(ctx)
	if err != nil {
		return "", "", 0, fmt.Errorf("download: %w", err)
	}
	base := filepath.Join(c.AudioDir, "inbound", fmt.Sprintf("%d", p.msgID))
	os.MkdirAll(filepath.Dir(base), 0o755)
	if err := os.WriteFile(base+".ogg", data, 0o644); err != nil {
		return "", "", 0, err
	}
	if err := voice.ToMP3(ctx, base+".ogg", base+".mp3"); err == nil {
		mp3Rel = rel(c.AudioDir, base+".mp3")
	}
	if d, err := voice.Duration(ctx, base+".ogg"); err == nil {
		durMS = int64(d * 1000)
	}
	t, err := c.Voice.Transcribe(ctx, base+".ogg")
	return strings.TrimSpace(t.Text), mp3Rel, durMS, err
}

// speak renders the script in John's voice and prepares the voice note.
func (c *Core) speak(ctx context.Context, runID int64, script string) (voice.VoiceNote, string, error) {
	mp3, _, err := c.Voice.TTS(ctx, c.VoiceID, c.TTSModel, script, c.Settings)
	if err != nil {
		return voice.VoiceNote{}, "", err
	}
	vn, err := voice.PrepareVoiceNote(ctx, mp3)
	if err != nil {
		return voice.VoiceNote{}, "", err
	}
	return vn, rel(c.AudioDir, mp3), nil
}

// fallback keeps the lead from being left hanging when the brain is down:
// a short, pre-rendered holding note, and the chat goes to John.
func (c *Core) fallback(ctx context.Context, runID int64, contact store.Contact, chatJID types.JID, simulated bool, cause error) {
	log.Printf("core: brain unavailable for run %d: %v", runID, cause)
	script := "Hey! Got your message, thank you. Give me a little moment and I'll get back to you properly, okay?"
	if vn, mp3Rel, err := c.speak(ctx, runID, script); err == nil {
		waID := ""
		if !simulated {
			waID, err = c.Gateway.SendVoiceNote(ctx, chatJID, vn)
		}
		if err == nil {
			c.addOut(contact.ID, runID, waID, "voice", script, mp3Rel, int64(vn.Duration*1000))
		}
	} else {
		c.sendText(ctx, contact.ID, runID, chatJID, simulated, script)
	}
	c.Store.SetStatus(contact.ID, store.StatusNeedsJohn, "assistant unavailable: "+cause.Error())
	c.Store.FinishRun(runID, "failed", "brain: "+cause.Error(), "", "")
	c.publishRun(runID)
	c.publishContact(contact.ID)
}

func (c *Core) sendText(ctx context.Context, contactID, runID int64, to types.JID, simulated bool, text string) error {
	waID := ""
	if !simulated {
		var err error
		if waID, err = c.Gateway.SendText(ctx, to, text); err != nil {
			return err
		}
	}
	c.addOut(contactID, runID, waID, "text", text, "", 0)
	return nil
}

func (c *Core) addOut(contactID, runID int64, waID, kind, text, audio string, durMS int64) {
	m, err := c.Store.AddMessage(store.Message{
		ContactID: contactID, WAID: waID, Direction: "out", Kind: kind, Source: "ai",
		Text: text, AudioPath: audio, DurationMS: durMS, RunID: runID,
	})
	if err != nil {
		log.Printf("core: store outbound: %v", err)
		return
	}
	c.Hub.Publish("message", m)
}

// SendAsJohn sends a text John typed in the dashboard.
func (c *Core) SendAsJohn(ctx context.Context, contactID int64, text string) error {
	contact, err := c.Store.Contact(contactID)
	if err != nil {
		return err
	}
	to, err := types.ParseJID(contact.Chat)
	if err != nil {
		return err
	}
	waID, err := c.Gateway.SendText(ctx, to, text)
	if err != nil {
		return err
	}
	m, err := c.Store.AddMessage(store.Message{ContactID: contactID, WAID: waID, Direction: "out", Kind: "text", Source: "john", Text: text})
	if err == nil {
		c.Hub.Publish("message", m)
	}
	return err
}

// SetStatus changes a chat's mode (take over / hand back) and tells the dashboard.
func (c *Core) SetStatus(contactID int64, status string) error {
	if err := c.Store.SetStatus(contactID, status, ""); err != nil {
		return err
	}
	typ := map[string]string{store.StatusHuman: "taken_over", store.StatusAuto: "handed_back"}[status]
	if typ != "" {
		c.Store.AddEvent(contactID, typ, nil)
	}
	c.publishContact(contactID)
	return nil
}

func (c *Core) allowVoice(contactID int64) bool {
	if c.MaxVoicePerHour <= 0 {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := c.chats[contactID]
	if ch == nil {
		return true
	}
	cutoff := time.Now().Add(-time.Hour)
	kept := ch.voiceLog[:0]
	for _, t := range ch.voiceLog {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	ch.voiceLog = kept
	if len(kept) >= c.MaxVoicePerHour {
		log.Printf("core: contact %d hit %d voice notes/hour; replying as text", contactID, c.MaxVoicePerHour)
		return false
	}
	ch.voiceLog = append(ch.voiceLog, time.Now())
	return true
}

func (c *Core) history(contactID int64) ([]brain.Turn, error) {
	msgs, err := c.Store.Messages(contactID, 20)
	if err != nil {
		return nil, err
	}
	var turns []brain.Turn
	for _, m := range msgs {
		if m.Text == "" {
			continue
		}
		turns = append(turns, brain.Turn{FromLead: m.Direction == "in", Voice: m.Kind == "voice", Text: m.Text})
	}
	return turns, nil
}

func (c *Core) stage(runID int64, stage string) {
	c.Store.Stage(runID, stage, store.Now())
	c.publishRun(runID)
}

func (c *Core) publishRun(runID int64) {
	runs, err := c.Store.Runs(50)
	if err != nil {
		return
	}
	for _, r := range runs {
		if r.ID == runID {
			c.Hub.Publish("run", r)
			return
		}
	}
}

func (c *Core) publishContact(id int64) {
	if ct, err := c.Store.Contact(id); err == nil {
		c.Hub.Publish("contact", ct)
	}
}

func (c *Core) publishMessage(contactID, msgID int64) {
	msgs, _ := c.Store.Messages(contactID, 50)
	for _, m := range msgs {
		if m.ID == msgID {
			c.Hub.Publish("message", m)
		}
	}
}

// firstName turns a push name like "Raj Patel 🔥" into "Raj"; empty if it
// doesn't look like a name.
func firstName(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	name := strings.TrimFunc(f[0], func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r > 127 && r < 0x2000)
	})
	if len([]rune(name)) < 2 {
		return ""
	}
	if strings.EqualFold(name, "aunty") || strings.EqualFold(name, "uncle") {
		return s // "Aunty Malee" is how she's addressed
	}
	return name
}

func rel(base, path string) string {
	if r, err := filepath.Rel(base, path); err == nil {
		return filepath.ToSlash(r)
	}
	return path
}
