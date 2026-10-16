package store

import (
	"path/filepath"
	"testing"
)

func TestLastMessages(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a, _ := st.CreateContact(Contact{JID: "a@s.whatsapp.net", Phone: "991"})
	b, _ := st.CreateContact(Contact{JID: "b@s.whatsapp.net", Phone: "992"})
	st.CreateContact(Contact{JID: "c@s.whatsapp.net", Phone: "993"}) // no messages yet
	add := func(c Contact, text string, at int64) {
		if _, err := st.AddMessage(Message{ContactID: c.ID, Direction: "in", Kind: "text", Source: "lead", Text: text, CreatedAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	add(a, "first", 100)
	add(a, "newest", 300)
	add(a, "older, stored later", 200)
	add(b, "tie 1", 500)
	add(b, "tie 2", 500) // same millisecond: the later one wins, as in Messages

	last, err := st.LastMessages()
	if err != nil {
		t.Fatal(err)
	}
	if len(last) != 2 || last[a.ID].Text != "newest" || last[b.ID].Text != "tie 2" {
		t.Fatalf("got %+v", last)
	}
	for _, c := range []Contact{a, b} {
		if m, _ := st.Messages(c.ID, 1); m[0].ID != last[c.ID].ID {
			t.Fatalf("contact %d: LastMessages and Messages disagree", c.ID)
		}
	}
}

func TestSetInterest(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	c, _ := st.CreateContact(Contact{JID: "a@s.whatsapp.net", Phone: "991"})
	st.SetInterest(c.ID, InterestHot, "asked how to pay")
	st.SetInterest(c.ID, "lukewarm", "not a level") // ignored
	got, _ := st.Contact(c.ID)
	if got.Interest != InterestHot || got.InterestReason != "asked how to pay" || got.InterestAt == 0 {
		t.Fatalf("interest: %+v", got)
	}
}
