package brain

import (
	"strings"
	"testing"
)

// The guardrails run on every reply and never call the API.
func TestEnforce(t *testing.T) {
	b := &Brain{ReferralURL: "https://example.com/join/john"}
	lead := func(s string) []Turn { return []Turn{{FromLead: true, Text: s}} }

	var m Meta
	r := b.enforce(Reply{SpokenScript: "You can make a lot. See https://x.y", IncludeReferral: true}, Contact{Name: "Priya"}, lead("how much can I earn?"), &m)
	if !r.Handoff || m.ForcedHandoff != "earn" || r.IncludeReferral || strings.Contains(r.SpokenScript, "lot") {
		t.Fatalf("earnings question must force a handoff: %+v %+v", r, m)
	}

	m = Meta{}
	r = b.enforce(Reply{SpokenScript: "Hey Tom! Here it is. https://evil.example", FollowupText: "Join: https://evil.example", IncludeReferral: true}, Contact{Name: "Tom"}, lead("how do I sign up?"), &m)
	if strings.Contains(r.SpokenScript, "http") || !strings.HasSuffix(r.FollowupText, b.ReferralURL) || strings.Contains(r.FollowupText, "evil") {
		t.Fatalf("URLs must be stripped and only the real link appended: %+v", r)
	}

	m = Meta{}
	r = b.enforce(Reply{SpokenScript: "Hi.", IncludeReferral: true}, Contact{LinkSent: true}, lead("cool, thanks"), &m)
	if r.IncludeReferral {
		t.Fatal("link already sent and not asked for again: don't resend")
	}

	long := strings.Repeat("This is a sentence with seven words. ", 20)
	got, trimmed := capWords(long, MaxWords)
	if !trimmed || len(strings.Fields(got)) > MaxWords || !strings.HasSuffix(got, ".") {
		t.Fatalf("capWords: %d words, %q", len(strings.Fields(got)), got[len(got)-20:])
	}
}
