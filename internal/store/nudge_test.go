package store

import (
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// Every rule for who gets offered a follow-up, one lead per rule.
func TestQuietLeads(t *testing.T) {
	st := openTest(t)
	now := Now()
	h := func(n float64) int64 { return now - int64(n*float64(time.Hour/time.Millisecond)) }
	lead := func(name, status string, msgs ...any) Contact {
		t.Helper()
		c, err := st.CreateContact(Contact{JID: name + "@s.whatsapp.net", Phone: "990" + name, Name: name, Status: status})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < len(msgs); i += 2 {
			dir, at := msgs[i].(string), msgs[i+1].(int64)
			src := "ai"
			if dir == "in" {
				src = "lead"
			}
			if _, err := st.AddMessage(Message{ContactID: c.ID, Direction: dir, Kind: "text", Source: src, Text: name + " " + dir, CreatedAt: at}); err != nil {
				t.Fatal(err)
			}
		}
		return c
	}

	lead("quiet", StatusAuto, "in", h(100), "out", h(99))
	lead("opener", StatusAuto, "out", h(60)) // never wrote back to John's first message
	lead("recent", StatusAuto, "in", h(12), "out", h(11))
	lead("replied", StatusAuto, "out", h(99), "in", h(98))
	lead("john", StatusHuman, "in", h(100), "out", h(99))
	lead("waiting", StatusNeedsJohn, "in", h(100), "out", h(99))
	lead("fresh", StatusNew, "out", h(99))
	lead("old", StatusAuto, "in", h(24*41), "out", h(24*40))
	optOut := lead("optout", StatusAuto, "in", h(100), "out", h(99))
	st.AddEvent(optOut.ID, "opt_out", nil)
	nudged := lead("nudged", StatusAuto, "in", h(150), "out", h(149), "out", h(99))
	st.AddNudge(Nudge{ContactID: nudged.ID, Status: NudgeSent, QuietSince: h(149), CreatedAt: h(99)})
	skipped := lead("skipped", StatusAuto, "in", h(100), "out", h(99))
	st.AddNudge(Nudge{ContactID: skipped.ID, Status: NudgeSkipped, QuietSince: h(99), CreatedAt: h(50)})
	drafted := lead("drafted", StatusAuto, "in", h(100), "out", h(99))
	st.AddNudge(Nudge{ContactID: drafted.ID, Status: NudgeDraft, QuietSince: h(99), CreatedAt: h(50)})
	// Nudged during an earlier silence, wrote back, then went quiet again.
	again := lead("again", StatusAuto, "out", h(220), "in", h(200), "out", h(150))
	st.AddNudge(Nudge{ContactID: again.ID, Status: NudgeSent, QuietSince: h(220), CreatedAt: h(215)})
	// A stale nudge (they replied before it went out) doesn't block the next silence.
	stale := lead("stale", StatusAuto, "in", h(100), "out", h(99))
	st.AddNudge(Nudge{ContactID: stale.ID, Status: NudgeStale, QuietSince: h(99), CreatedAt: h(60)})

	quiet, err := st.QuietLeads(now, int64(48*time.Hour/time.Millisecond), int64(30*24*time.Hour/time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, q := range quiet {
		got = append(got, q.Contact.Name)
	}
	want := []string{"again", "quiet", "stale", "opener"} // oldest silence first
	if !slices.Equal(got, want) {
		t.Fatalf("quiet leads: got %v, want %v", got, want)
	}
	if q := quiet[1]; q.LastText != "quiet in" || q.LastOut != h(99) || q.LastIn != h(100) {
		t.Fatalf("quiet lead details: %+v", q)
	}
	if q := quiet[3]; q.LastIn != 0 || q.LastText != "" {
		t.Fatalf("a lead who never wrote has no last message: %+v", q)
	}
}

func TestNudgeLifecycle(t *testing.T) {
	st := openTest(t)
	c, _ := st.CreateContact(Contact{JID: "a@s.whatsapp.net", Phone: "991", Status: StatusAuto})
	n, err := st.AddNudge(Nudge{ContactID: c.ID, Script: "Hey!", QuietSince: 1})
	if err != nil || n.Status != NudgeDraft {
		t.Fatalf("new nudge: %+v %v", n, err)
	}

	// Two clicks on Send: only one claims it.
	if ok, _ := st.ClaimNudge(n.ID); !ok {
		t.Fatal("first claim should win")
	}
	if ok, _ := st.ClaimNudge(n.ID); ok {
		t.Fatal("a nudge already sending must not be claimed again")
	}

	// A failed send can be retried.
	n, _ = st.Nudge(n.ID)
	n.Status, n.Error = NudgeFailed, "network"
	st.SaveNudge(n)
	if ok, _ := st.ClaimNudge(n.ID); !ok {
		t.Fatal("a failed nudge can be retried")
	}
	if n, _ = st.Nudge(n.ID); n.Error != "" {
		t.Fatalf("retrying clears the old error: %+v", n)
	}

	// A held-back draft with no words can't be sent.
	empty, _ := st.AddNudge(Nudge{ContactID: c.ID, Status: NudgeFailed, Error: "held back"})
	if ok, _ := st.ClaimNudge(empty.ID); ok {
		t.Fatal("an empty nudge must not be claimed")
	}
	st.SkipNudge(empty.ID)

	// Writing back retires unsent nudges but never a sent one.
	n.Status = NudgeSent
	st.SaveNudge(n)
	draft, _ := st.AddNudge(Nudge{ContactID: c.ID, Script: "Another"})
	if k, _ := st.StaleNudges(c.ID, "they wrote back"); k != 1 {
		t.Fatalf("want 1 stale nudge, got %d", k)
	}
	if n, _ := st.Nudge(n.ID); n.Status != NudgeSent {
		t.Fatalf("a sent nudge stays sent: %+v", n)
	}
	if d, _ := st.Nudge(draft.ID); d.Status != NudgeStale || d.Error != "they wrote back" {
		t.Fatalf("the draft should be stale: %+v", d)
	}

	// Edits and skips only touch drafts; a crash mid-send never resends.
	if ok, _ := st.SaveDraft(Nudge{ID: draft.ID, Script: "edited", Status: NudgeDraft}); ok {
		t.Fatal("a stale nudge can't be edited")
	}
	live, _ := st.AddNudge(Nudge{ContactID: c.ID, Script: "Hi"})
	st.ClaimNudge(live.ID)
	if ok, _ := st.SkipNudge(live.ID); ok {
		t.Fatal("a nudge being sent can't be skipped")
	}
	st.RecoverNudges()
	if live, _ = st.Nudge(live.ID); live.Status != NudgeFailed || live.Error == "" {
		t.Fatalf("after a restart a half-sent nudge is failed, not resent: %+v", live)
	}
	if ok, _ := st.SkipNudge(live.ID); !ok {
		t.Fatal("a failed nudge can be skipped")
	}

	// Deleting the contact (Reset chat) deletes its nudges.
	st.DeleteContact(c.ID)
	if all, _ := st.Nudges(); len(all) != 0 {
		t.Fatalf("nudges outlived their contact: %+v", all)
	}
}
