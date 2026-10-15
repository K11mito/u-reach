package brain

import (
	"math"
	"strings"
	"testing"
)

// The guardrails run on every reply and never call the API.
func TestEnforce(t *testing.T) {
	b := &Brain{ReferralURL: "https://example.com/join/john"}
	lead := func(s string) []Turn { return []Turn{{FromLead: true, Text: s}} }

	// Earnings questions are answered, not handed off, as long as nothing is promised.
	var m Meta
	honest := "When someone joins through your link, you earn a direct commission. Nobody can promise anyone an income."
	r := b.enforce(Reply{SpokenScript: honest, IncludeReferral: true}, Contact{Name: "Priya"}, lead("how much can I earn? is this a scam?"), &m)
	if r.Handoff || m.ForcedHandoff != "" || r.SpokenScript != honest {
		t.Fatalf("an honest answer about earnings must go out as written: %+v %+v", r, m)
	}

	// A reply that promises money is swapped for a holding line.
	m = Meta{}
	r = b.enforce(Reply{SpokenScript: "Honestly, people make two thousand dollars a month. See https://x.y", IncludeReferral: true}, Contact{Name: "Priya"}, lead("how much can I earn?"), &m)
	if !r.Handoff || !strings.HasPrefix(m.ForcedHandoff, "income claim") || r.IncludeReferral || strings.Contains(r.SpokenScript, "thousand") {
		t.Fatalf("an income promise must force a handoff: %+v %+v", r, m)
	}

	// Money gone wrong still goes to John whatever the model says.
	m = Meta{}
	r = b.enforce(Reply{SpokenScript: "Sure thing!", IncludeReferral: true}, Contact{Name: "Ben"}, lead("I paid but nothing arrived, I want a refund"), &m)
	if !r.Handoff || m.ForcedHandoff != "refund" || r.IncludeReferral {
		t.Fatalf("a refund request must force a handoff: %+v %+v", r, m)
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

	m = Meta{}
	r = b.enforce(Reply{SpokenScript: "No problem, take care.", OptOut: true, IncludeReferral: true}, Contact{}, lead("please stop messaging me"), &m)
	if r.IncludeReferral {
		t.Fatal("never send the link to someone who opted out")
	}

	long := strings.Repeat("This is a sentence with seven words. ", 20)
	got, trimmed := capWords(long, MaxWords)
	if !trimmed || len(strings.Fields(got)) > MaxWords || !strings.HasSuffix(got, ".") {
		t.Fatalf("capWords: %d words, %q", len(strings.Fields(got)), got[len(got)-20:])
	}
}

// "Only when asked" holds even if the model tries to send the link.
func TestLinkWhenAsked(t *testing.T) {
	b := &Brain{}
	b.Configure(Config{ReferralURL: "https://u.center/?ref=john", Style: Style{LinkTiming: LinkWhenAsked}})
	lead := func(s string) []Turn { return []Turn{{FromLead: true, Text: s}} }
	var m Meta
	if r := b.enforce(Reply{SpokenScript: "Hi!", IncludeReferral: true}, Contact{}, lead("what is U-topia?"), &m); r.IncludeReferral {
		t.Fatal("not asked for the link: don't send it")
	}
	if r := b.enforce(Reply{SpokenScript: "Hi!", IncludeReferral: true}, Contact{}, lead("how do I join?"), &m); !r.IncludeReferral || !strings.HasSuffix(r.FollowupText, "https://u.center/?ref=john") {
		t.Fatalf("asked how to join: send the link: %+v", r)
	}
	if r := b.enforce(Reply{SpokenScript: "Hi!", IncludeReferral: true}, Contact{}, nil, &m); !r.IncludeReferral {
		t.Fatal("campaign openers always carry the link")
	}
	long := strings.Repeat("One two three four five. ", 40)
	b.Configure(Config{Style: Style{MinWords: 20, MaxWords: 30}})
	if r := b.enforce(Reply{SpokenScript: long}, Contact{}, lead("hi"), &m); len(strings.Fields(r.SpokenScript)) > 50 {
		t.Fatalf("style cap should be max words plus slack, got %d", len(strings.Fields(r.SpokenScript)))
	}
}

func TestCostUSD(t *testing.T) {
	for _, tc := range []struct {
		m    Meta
		want float64
	}{
		{Meta{Model: "claude-opus-5-5", InputTokens: 1_000_000}, 4},
		{Meta{Model: "claude-opus-5-5", OutputTokens: 1_000_000}, 20},
		{Meta{Model: "claude-opus-5-5", CacheRead: 1_000_000, CacheWrite: 1_000_000}, 0.20 + 5},
		{Meta{Model: "claude-opus-5-20260101", InputTokens: 1_000_000}, 5}, // dated, not mistaken for 5.5
		{Meta{Model: "claude-sonnet-5-5", InputTokens: 1_000_000}, 2},
		{Meta{Model: "something-new", InputTokens: 1_000_000}, 4}, // priced as the default
	} {
		if got := tc.m.CostUSD(); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: got %v, want %v", tc.m.Model, got, tc.want)
		}
	}
}

func TestIncomeClaims(t *testing.T) {
	for _, s := range []string{
		"You could make two thousand dollars a month.",
		"People earn $500 every week.",
		"Some earn 30% a year on it.",
		"You'll be earning six figures.",
		"It can double your money.",
		"I made five hundred dollars last week.",
	} {
		if !incomeClaimRe.MatchString(s) {
			t.Errorf("should flag: %q", s)
		}
	}
	for _, s := range []string{
		"When someone joins through your link, you earn a direct commission on their membership.",
		"Starter is twenty-five dollars and Bronze is one hundred dollars, one time, no monthly subscription.",
		"Let's make one quick call this week, and make it simple.",
		"Nobody can promise anyone an income; it depends on what you do.",
		"Sending money home is free and arrives in seconds.",
	} {
		if incomeClaimRe.MatchString(s) {
			t.Errorf("should not flag: %q (matched %q)", s, incomeClaimRe.FindString(s))
		}
	}
}
