package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"ureach/internal/brain"
	"ureach/internal/store"
)

var nudgeReply = brain.Reply{SpokenScript: "Hey Raj! You asked about sending money home. The fees are tiny, by the way. Want me to show you?", IncludeReferral: true}

// quietRig is a rig where a lead asked something, the AI answered, and a
// moment of silence already counts as quiet.
func quietRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t)
	r.core.Nudges = NudgeConfig{After: time.Millisecond, Quick: true}
	r.lead("hi John, what is U-topia?")
	r.waitRun(t)
	time.Sleep(5 * time.Millisecond)
	r.claude.reply = nudgeReply
	return r
}

func (r *rig) draftOne(t *testing.T) store.Nudge {
	t.Helper()
	made, err := r.core.DraftNudges(context.Background(), 5)
	if err != nil || made != 1 {
		t.Fatalf("want 1 draft, got %d (%v)", made, err)
	}
	all, _ := r.store.Nudges(store.NudgeDraft)
	if len(all) != 1 {
		t.Fatalf("want 1 stored draft, got %+v", all)
	}
	return all[0]
}

func (r *rig) waitNudge(t *testing.T, id int64, statuses ...string) store.Nudge {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		n, err := r.store.Nudge(id)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range statuses {
			if n.Status == s {
				return n
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	n, _ := r.store.Nudge(id)
	t.Fatalf("nudge never reached %v: %+v", statuses, n)
	return n
}

func TestNudgeDraftThenSend(t *testing.T) {
	r := quietRig(t)
	voiceBefore, textsBefore := r.wa.sent()
	n := r.draftOne(t)
	if n.Script != nudgeReply.SpokenScript || n.AudioPath != "" {
		t.Fatalf("a draft is words only, recorded later: %+v", n)
	}
	if n.Link {
		t.Fatal("they already have the link; the follow-up must not send it again")
	}
	if made, _ := r.core.DraftNudges(context.Background(), 5); made != 0 {
		t.Fatal("one follow-up per silence")
	}
	if q, _ := r.core.Queue(); len(q.Items) != 1 || q.Items[0].Name != "Raj Patel" || q.Items[0].LastLeadText != "hi John, what is U-topia?" || q.Waiting != 0 {
		t.Fatalf("queue: %+v", q)
	}

	ids, err := r.core.SendNudges([]int64{n.ID, n.ID})
	if err != nil || len(ids) != 1 {
		t.Fatalf("a double click sends once: %v %v", ids, err)
	}
	if _, err := r.core.SendNudges([]int64{n.ID}); err == nil {
		t.Fatal("sending again must be refused")
	}
	n = r.waitNudge(t, n.ID, store.NudgeSent, store.NudgeFailed, store.NudgeStale)
	if n.Status != store.NudgeSent || n.SentAt == 0 || n.AudioPath == "" {
		t.Fatalf("nudge: %+v", n)
	}
	voiceAfter, textsAfter := r.wa.sent()
	if voiceAfter != voiceBefore+1 || len(textsAfter) != len(textsBefore) {
		t.Fatalf("want one more voice note and no text, got %d→%d voice, %d→%d texts", voiceBefore, voiceAfter, len(textsBefore), len(textsAfter))
	}
	if made, _ := r.core.DraftNudges(context.Background(), 5); made != 0 {
		t.Fatal("no second follow-up until they write back")
	}
	if c := r.contact(t); c.Status != store.StatusAuto {
		t.Fatalf("the AI keeps the chat: %+v", c)
	}
}

func TestNudgeStaleWhenLeadWritesBack(t *testing.T) {
	r := quietRig(t)
	n := r.draftOne(t)
	r.lead("sorry, was busy! how do I join?")
	if got, _ := r.store.Nudge(n.ID); got.Status != store.NudgeStale || got.Error != "they wrote back" {
		t.Fatalf("their message makes the draft unnecessary: %+v", got)
	}
	if _, err := r.core.SendNudges([]int64{n.ID}); err == nil {
		t.Fatal("a stale follow-up must not send")
	}
	r.waitRuns(t, 2) // the AI answers them as usual
}

func TestNudgeCheckedAgainAtSendTime(t *testing.T) {
	r := quietRig(t)
	n := r.draftOne(t)
	// A message that slipped in without passing through Handle (e.g. while
	// the server was restarting) still stops the follow-up.
	r.store.AddMessage(store.Message{ContactID: n.ContactID, Direction: "in", Kind: "text", Source: "lead", Text: "hello?"})
	voiceBefore, _ := r.wa.sent()
	r.core.SendNudges([]int64{n.ID})
	n = r.waitNudge(t, n.ID, store.NudgeSent, store.NudgeStale, store.NudgeFailed)
	if n.Status != store.NudgeStale || n.Error != "they wrote back" {
		t.Fatalf("nudge: %+v", n)
	}
	if v, _ := r.wa.sent(); v != voiceBefore {
		t.Fatal("nothing may go out")
	}
}

func TestNudgeRetiredWhenJohnTakesOver(t *testing.T) {
	r := quietRig(t)
	n := r.draftOne(t)
	r.core.SetStatus(n.ContactID, store.StatusHuman)
	if got, _ := r.store.Nudge(n.ID); got.Status != store.NudgeStale {
		t.Fatalf("John has the chat now: %+v", got)
	}
	r.core.SetStatus(n.ContactID, store.StatusAuto)
	if made, _ := r.core.DraftNudges(context.Background(), 5); made != 1 {
		t.Fatal("handed back and still quiet: offered again")
	}
}

func TestNudgeDemoLeadStaysLocal(t *testing.T) {
	r := newRig(t)
	r.core.Nudges = NudgeConfig{After: time.Millisecond, Quick: true}
	r.claude.reply = nudgeReply
	ct, _ := r.store.CreateContact(store.Contact{JID: "990000000777@s.whatsapp.net", Phone: "990000000777", Name: "Mei Lin", Status: store.StatusAuto})
	r.store.AddMessage(store.Message{ContactID: ct.ID, Direction: "in", Kind: "text", Source: "lead", Text: "is this safe?", CreatedAt: store.Now() - 100})
	r.store.AddMessage(store.Message{ContactID: ct.ID, Direction: "out", Kind: "voice", Source: "seed", Text: "Yes, it's safe.", CreatedAt: store.Now() - 50})
	time.Sleep(5 * time.Millisecond)
	n := r.draftOne(t)
	if !n.Link {
		t.Fatal("she never got the link and the AI offered it")
	}
	r.core.SendNudges([]int64{n.ID})
	r.waitNudge(t, n.ID, store.NudgeSent, store.NudgeFailed)
	if v, texts := r.wa.sent(); v != 0 || len(texts) != 0 {
		t.Fatalf("a made-up number never reaches WhatsApp: %d voice, %q", v, texts)
	}
	msgs, _ := r.store.Messages(ct.ID, 10)
	if len(msgs) != 4 || msgs[2].Kind != "voice" || !strings.HasSuffix(msgs[3].Text, "https://u.center/?ref=john") {
		t.Fatalf("the follow-up and link should be in her chat: %+v", msgs)
	}
	if c, _ := r.store.Contact(ct.ID); c.LinkSentAt == 0 {
		t.Fatal("the link counts as sent")
	}
}

func TestNudgeVoiceFailureSendsText(t *testing.T) {
	r := quietRig(t)
	n := r.draftOne(t)
	r.wa.failVoice = errors.New("upload failed")
	r.core.SendNudges([]int64{n.ID})
	n = r.waitNudge(t, n.ID, store.NudgeSent, store.NudgeFailed)
	if n.Status != store.NudgeSent {
		t.Fatalf("nudge: %+v", n)
	}
	if _, texts := r.wa.sent(); texts[len(texts)-1] != nudgeReply.SpokenScript {
		t.Fatalf("the words should go out as text: %q", texts)
	}
}

func TestNudgeIncomeClaimHeldBack(t *testing.T) {
	r := quietRig(t)
	r.claude.reply = brain.Reply{SpokenScript: "Hey Raj! People are making $5,000 a month with this, you should join."}
	made, err := r.core.DraftNudges(context.Background(), 5)
	if err != nil || made != 1 {
		t.Fatalf("made %d, %v", made, err)
	}
	all, _ := r.store.Nudges(store.NudgeFailed)
	if len(all) != 1 || all[0].Script != "" || !strings.Contains(all[0].Error, "earnings") {
		t.Fatalf("a draft that promises money is held back: %+v", all)
	}
	if _, err := r.core.SendNudges([]int64{all[0].ID}); err == nil {
		t.Fatal("a follow-up with no words must never send")
	}
}

func TestNudgeEditAndClaudeDown(t *testing.T) {
	r := quietRig(t)
	n := r.draftOne(t)
	if _, err := r.core.EditNudge(n.ID, "  ", ""); err == nil {
		t.Fatal("an empty voice note is refused")
	}
	if _, err := r.core.EditNudge(n.ID, strings.Repeat("word ", 200), ""); err == nil {
		t.Fatal("an essay is refused")
	}
	n, err := r.core.ListenNudge(context.Background(), n.ID)
	if err != nil || n.AudioPath == "" {
		t.Fatalf("listen: %+v %v", n, err)
	}
	if n, _ = r.core.EditNudge(n.ID, "Hey Raj, quick one: did the app install okay?", "No rush!"); n.AudioPath != "" {
		t.Fatal("new words need a new recording")
	}

	// Claude is down: Redraft fails cleanly and keeps John's words.
	r.claude.status.Store(500)
	if _, err := r.core.Redraft(context.Background(), n.ID); err == nil {
		t.Fatal("redraft should report the failure")
	}
	if got, _ := r.store.Nudge(n.ID); got.Script != "Hey Raj, quick one: did the app install okay?" || got.Status != store.NudgeDraft {
		t.Fatalf("the draft must survive a failed redraft: %+v", got)
	}
}

func TestNudgeNoDraftWhileClaudeDown(t *testing.T) {
	r := quietRig(t)
	r.claude.status.Store(400)
	if made, err := r.core.DraftNudges(context.Background(), 5); made != 0 || err == nil {
		t.Fatalf("made %d, err %v", made, err)
	}
	if all, _ := r.store.Nudges(); len(all) != 0 {
		t.Fatalf("nothing is stored, so the next pass tries again: %+v", all)
	}
	r.claude.status.Store(0)
	r.draftOne(t)
}

// waitRuns waits until n runs have finished.
func (r *rig) waitRuns(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if runs, _ := r.store.Runs(n); len(runs) == n && runs[0].Status != "running" {
			time.Sleep(50 * time.Millisecond)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%d runs never finished", n)
}
