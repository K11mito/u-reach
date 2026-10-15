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

// The autonomy slider moves what goes to John; income claims never go out.
func TestAutonomyLevels(t *testing.T) {
	lead := func(s string) []Turn { return []Turn{{FromLead: true, Text: s}} }
	run := func(level, msg, script string) (Reply, Meta) {
		b := &Brain{}
		b.Configure(Config{Autonomy: level, ReferralURL: "https://u.center/?ref=john"})
		var m Meta
		return b.enforce(Reply{SpokenScript: script, IncludeReferral: true}, Contact{Name: "Raj"}, lead(msg), &m), m
	}
	cases := []struct {
		level, msg string
		handoff    bool
	}{
		{AutonomyLow, "is it safe to put money in?", true},
		{AutonomyMedium, "is it safe to put money in?", false},
		{AutonomyHigh, "is it safe to put money in?", false},
		{AutonomyLow, "how do I pay for the package?", true},
		{AutonomyMedium, "how much can I earn?", true},
		{AutonomyHigh, "how much can I earn?", false},
		{AutonomyHigh, "be honest, is this a pyramid scheme?", false},
		{AutonomyMedium, "be honest, is this a pyramid scheme?", true},
		{AutonomyHigh, "you scammed me, I want a refund", true},
		{AutonomyHigh, "my lawyer will contact you", true},
		{AutonomyLow, "hi John! what is U.Center?", false},
		{"", "how much can I earn?", true}, // unset means balanced
	}
	for _, c := range cases {
		r, m := run(c.level, c.msg, "Hey Raj! It depends on you, let's talk on a quick call.")
		if r.Handoff != c.handoff {
			t.Errorf("%s / %q: handoff=%v, want %v (forced by %q)", c.level, c.msg, r.Handoff, c.handoff, m.ForcedHandoff)
		}
	}

	for _, script := range []string{"You can easily earn five thousand dollars a month!", "People make $2,000 a week with this.", "It's real passive income.", "You could make thousands from home."} {
		r, m := run(AutonomyHigh, "tell me more", script)
		if !r.Handoff || r.IncludeReferral || !strings.HasPrefix(m.ForcedHandoff, "income claim") || strings.Contains(r.SpokenScript, "earn") {
			t.Errorf("income claim %q must be blocked: %+v %q", script, r, m.ForcedHandoff)
		}
	}
	for _, script := range []string{"You can get started with one hundred dollars.", "You just make one payment, that's it.", "Packages start at one hundred dollars."} {
		if r, m := run(AutonomyHigh, "how much is it?", script); r.Handoff {
			t.Errorf("not an income claim %q, forced by %q", script, m.ForcedHandoff)
		}
	}

	if p := systemPrompt(Config{Autonomy: AutonomyHigh}); !strings.Contains(p, "end to end") || strings.Contains(p, "When in doubt, hand off") {
		t.Fatal("full autonomy prompt")
	}
	if p := systemPrompt(Config{Autonomy: AutonomyLow}); !strings.Contains(p, "When in doubt, hand off") {
		t.Fatal("cautious prompt")
	}
	if p := systemPrompt(Config{}); !strings.Contains(p, "Handoff: if the lead asks about earnings") {
		t.Fatal("balanced prompt should keep the original rules")
	}
}

// A more cautious level hands John everything a bolder one does, and more.
func TestAutonomyLevelsAreNested(t *testing.T) {
	msgs := []string{
		"what is U.Center?", "is it safe?", "how do I pay?", "how much can I earn?", "is this a pyramid?",
		"my lawyer will contact you", "I lost my money", "I want a refund", "the police are involved", "can we meet tomorrow?",
		"this is an emergency", "you scammed me", "what are the fees?", "I'm worried", "tell me more",
	}
	rank := map[string]int{AutonomyLow: 0, AutonomyMedium: 1, AutonomyHigh: 2}
	for _, m := range msgs {
		for bold, rb := range rank {
			for careful, rc := range rank {
				if rc < rb && handoffTrigger(bold, m) != "" && handoffTrigger(careful, m) == "" {
					t.Errorf("%q goes to John at %s but not at the more cautious %s", m, bold, careful)
				}
			}
		}
	}
}
