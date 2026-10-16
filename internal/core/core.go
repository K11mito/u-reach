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
	// Avatar fetches a WhatsApp profile photo (nil when WhatsApp is off).
	Avatar func(ctx context.Context, jid types.JID) ([]byte, error)

	// Nudges tunes the follow-up queue for quiet leads.
	Nudges NudgeConfig

	mu            sync.Mutex
	chats         map[int64]*chat
	avatarChecked map[int64]time.Time
	nudge         nudger
}

// Go runs fn in a goroutine and turns a panic into a log line, so one bad
// message can never take the whole server down mid-demo.
func Go(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("PANIC in %s (recovered): %v", name, r)
			}
		}()
		fn()
	}()
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
	if !m.Simulated {
		c.FetchAvatar(contact)
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
	c.staleNudges(contact.ID, "they wrote back")
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
		// Always release the chat, even if the run panics, so the lead isn't
		// stuck behind a dead run.
		defer func() {
			if r := recover(); r != nil {
				log.Printf("PANIC in reply for contact %d (recovered): %v", contactID, r)
			}
			c.mu.Lock()
			ch.running = false
			again := len(ch.pending) > 0
			c.mu.Unlock()
			if again {
				c.fire(contactID)
			}
		}()
		c.run(contactID, batch)
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
	if err != nil && retryable(err) {
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

	// Sent. If the voice note won't go through (a media upload hiccup), the
	// lead still gets the same words as a text rather than nothing.
	if spoke {
		if err := c.sendVoice(ctx, contactID, runID, chatJID, simulated, reply.SpokenScript, vn, mp3Rel); err != nil {
			log.Printf("core: voice note failed, sending text instead: %v", err)
			spoke = false
		}
	}
	if !spoke {
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
	c.Store.SetInterest(contactID, reply.Interest, reply.InterestReason)
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

// retryable says whether asking Claude again could help. A 4xx means the
// request itself was refused (credit used up, bad key): a second try fails
// the same way and only delays the holding note. Rate limits and server
// errors are already retried by the SDK; a malformed answer may come out
// right the second time.
func retryable(err error) bool {
	return !strings.HasPrefix(err.Error(), "claude 4")
}

// FallbackScript is the holding note sent when the brain is unavailable.
const FallbackScript = "Hey! Got your message, thank you. Give me a little moment and I'll get back to you properly, okay?"

// unavailable starts the handoff reason of a chat paused because the brain
// failed, rather than because the AI chose to hand it to John.
const unavailable = "assistant unavailable: "

// Prewarm renders the fallback note ahead of time, so the safety net is
// instant and works even if ElevenLabs is slow at that moment.
func (c *Core) Prewarm(ctx context.Context) {
	if _, _, err := c.speak(ctx, 0, FallbackScript); err != nil {
		log.Printf("core: prewarm fallback note: %v", err)
	}
}

// fallback keeps the lead from being left hanging when the brain is down:
// a short, pre-rendered holding note, and the chat goes to John.
func (c *Core) fallback(ctx context.Context, runID int64, contact store.Contact, chatJID types.JID, simulated bool, cause error) {
	log.Printf("core: brain unavailable for run %d: %v", runID, cause)
	script := FallbackScript
	vn, mp3Rel, err := c.speak(ctx, runID, script)
	if err == nil {
		err = c.sendVoice(ctx, contact.ID, runID, chatJID, simulated, script, vn, mp3Rel)
	}
	if err != nil {
		log.Printf("core: holding voice note failed, sending text instead: %v", err)
		if err := c.sendText(ctx, contact.ID, runID, chatJID, simulated, script); err != nil {
			log.Printf("core: holding text failed too: %v", err)
		}
	}
	c.Store.SetStatus(contact.ID, store.StatusNeedsJohn, unavailable+cause.Error())
	c.Store.FinishRun(runID, "failed", "brain: "+cause.Error(), "", "")
	c.publishRun(runID)
	c.publishContact(contact.ID)
}

func (c *Core) sendVoice(ctx context.Context, contactID, runID int64, to types.JID, simulated bool, script string, vn voice.VoiceNote, mp3Rel string) error {
	waID := ""
	if !simulated {
		var err error
		if waID, err = c.Gateway.SendVoiceNote(ctx, to, vn); err != nil {
			return err
		}
	}
	c.addOut(contactID, runID, waID, "voice", script, mp3Rel, int64(vn.Duration*1000))
	return nil
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

// HandleReceipt records delivered/read/played ticks for messages we sent.
func (c *Core) HandleReceipt(waIDs []string, kind string) {
	for _, id := range waIDs {
		m, changed, err := c.Store.SetDelivery(id, kind)
		if err != nil {
			log.Printf("core: receipt: %v", err)
			continue
		}
		if changed {
			c.Hub.Publish("message", m)
			if kind == "played" && m.Kind == "voice" {
				c.Store.AddEvent(m.ContactID, "played", map[string]int64{"message_id": m.ID})
			}
		}
	}
}

// HandleOwn records a message John typed on the demo phone itself. John
// stepping in by hand means the AI steps back for that chat.
func (c *Core) HandleOwn(m Msg) {
	contact, created, err := c.Store.UpsertInbound(m.From.String(), m.Chat.String(), m.Phone, "")
	if err != nil {
		log.Printf("core: own message contact: %v", err)
		return
	}
	if created {
		c.Hub.Publish("contact", contact)
	}
	c.FetchAvatar(contact)
	kind := "text"
	if m.Voice != nil {
		kind = "voice"
		if m.Text == "" {
			m.Text = "(voice note from John's phone)"
		}
	}
	msg, err := c.Store.AddMessage(store.Message{ContactID: contact.ID, WAID: m.WAID, Direction: "out", Kind: kind, Source: "john", Text: m.Text, CreatedAt: m.At.UnixMilli()})
	if err != nil {
		if !errors.Is(err, store.ErrDuplicate) {
			log.Printf("core: own message: %v", err)
		}
		return
	}
	c.Hub.Publish("message", msg)
	if contact.Status != store.StatusHuman {
		log.Printf("core: John replied to %s from the phone; pausing the AI for this chat", contact.DisplayName())
		c.SetStatus(contact.ID, store.StatusHuman)
	}
}

// SendAsJohn sends a text John typed in the dashboard. Seeded and simulated
// leads have made-up numbers, so their replies are only recorded, the same
// way the simulate box never sends to WhatsApp.
func (c *Core) SendAsJohn(ctx context.Context, contactID int64, text string) error {
	contact, err := c.Store.Contact(contactID)
	if err != nil {
		return err
	}
	waID := ""
	if !store.IsDemoNumber(contact.Phone) {
		to, err := types.ParseJID(contact.Chat)
		if err != nil {
			return err
		}
		if waID, err = c.Gateway.SendText(ctx, to, text); err != nil {
			return err
		}
	}
	m, err := c.Store.AddMessage(store.Message{ContactID: contactID, WAID: waID, Direction: "out", Kind: "text", Source: "john", Text: text})
	if err == nil {
		c.Hub.Publish("message", m)
		c.staleNudges(contactID, "John messaged them")
	}
	return err
}

// SetStatus changes a chat's mode (take over / hand back) and tells the dashboard.
// Handing back a chat that was paused because the AI was down answers the
// lead's waiting message, since their holding note promised a proper reply.
func (c *Core) SetStatus(contactID int64, status string) error {
	before, err := c.Store.Contact(contactID)
	if err != nil {
		return err
	}
	if err := c.Store.SetStatus(contactID, status, ""); err != nil {
		return err
	}
	typ := map[string]string{store.StatusHuman: "taken_over", store.StatusAuto: "handed_back"}[status]
	if typ != "" {
		c.Store.AddEvent(contactID, typ, nil)
	}
	if status != store.StatusAuto {
		c.staleNudges(contactID, "John took over the chat")
	}
	c.publishContact(contactID)
	if status == store.StatusAuto && strings.HasPrefix(before.HandoffReason, unavailable) {
		c.answerWaiting(before)
	}
	return nil
}

// answerWaiting replies to the lead's last message if nobody has really
// answered it yet (only the holding note went out).
func (c *Core) answerWaiting(ct store.Contact) {
	msgs, err := c.Store.Messages(ct.ID, 20)
	if err != nil {
		return
	}
	var last *store.Message
	for i := len(msgs) - 1; i >= 0 && last == nil; i-- {
		switch m := msgs[i]; {
		case m.Direction == "in":
			last = &msgs[i]
		case m.Source == "ai" && m.Text == FallbackScript:
			continue
		default:
			return // John or the AI already answered
		}
	}
	chatJID, err := types.ParseJID(ct.Chat)
	if last == nil || err != nil {
		return
	}
	log.Printf("core: %s handed back; answering their waiting message", ct.DisplayName())
	m := Msg{WAID: last.WAID, Chat: chatJID, Phone: ct.Phone, Text: last.Text, Simulated: store.IsDemoNumber(ct.Phone), At: time.UnixMilli(last.CreatedAt)}
	c.mu.Lock()
	if c.chats == nil {
		c.chats = map[int64]*chat{}
	}
	ch := c.chats[ct.ID]
	if ch == nil {
		ch = &chat{}
		c.chats[ct.ID] = ch
	}
	ch.pending = append(ch.pending, pending{msg: m, msgID: last.ID})
	c.mu.Unlock()
	c.fire(ct.ID)
}

// FetchAvatar pulls a contact's WhatsApp profile photo in the background,
// at most once an hour per contact. Demo numbers are never looked up.
func (c *Core) FetchAvatar(ct store.Contact) {
	if c.Avatar == nil || store.IsDemoNumber(ct.Phone) {
		return
	}
	c.mu.Lock()
	if c.avatarChecked == nil {
		c.avatarChecked = map[int64]time.Time{}
	}
	if time.Since(c.avatarChecked[ct.ID]) < time.Hour {
		c.mu.Unlock()
		return
	}
	c.avatarChecked[ct.ID] = time.Now()
	c.mu.Unlock()

	Go("avatar", func() {
		jid, err := types.ParseJID(ct.JID)
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		img, err := c.Avatar(ctx, jid)
		if err != nil || img == nil {
			if err != nil {
				log.Printf("core: avatar for %s: %v", ct.DisplayName(), err)
			}
			return
		}
		rel := fmt.Sprintf("avatars/%d.jpg", ct.ID)
		if err := writeFile(filepath.Join(c.AudioDir, rel), img); err != nil {
			return
		}
		c.Store.SetAvatar(ct.ID, rel+fmt.Sprintf("?v=%d", time.Now().Unix()))
		c.publishContact(ct.ID)
	})
}

// FetchOwnAvatar saves the linked account's own photo as John's avatar.
func (c *Core) FetchOwnAvatar(own types.JID) {
	if c.Avatar == nil || own.IsEmpty() {
		return
	}
	Go("own avatar", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		img, err := c.Avatar(ctx, own)
		if err != nil || img == nil {
			return
		}
		if writeFile(filepath.Join(c.AudioDir, "avatars", "john.jpg"), img) == nil {
			c.Hub.Publish("ambassador_avatar", map[string]string{"avatar": fmt.Sprintf("avatars/john.jpg?v=%d", time.Now().Unix())})
		}
	})
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ResetChat removes a lead and their whole conversation (e.g. the presenter's
// own chat between rehearsals) and forgets any pending reply for them. If they
// message again they arrive as a brand-new lead.
func (c *Core) ResetChat(contactID int64) error {
	c.mu.Lock()
	if ch := c.chats[contactID]; ch != nil && ch.timer != nil {
		ch.timer.Stop()
	}
	delete(c.chats, contactID)
	delete(c.avatarChecked, contactID)
	c.mu.Unlock()
	if err := c.Store.DeleteContact(contactID); err != nil {
		return err
	}
	c.Hub.Publish("chat_reset", map[string]int64{"contact_id": contactID})
	return nil
}

// Reset forgets in-flight chats (pending debounces, rate limits), for a
// demo reset. Runs already in progress finish against the old data.
func (c *Core) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ch := range c.chats {
		if ch.timer != nil {
			ch.timer.Stop()
		}
	}
	c.chats = nil
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
