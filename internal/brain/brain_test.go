package brain

import (
	"context"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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
		{AutonomyLow, "hi John! what is U.Center?", false},
		{AutonomyLow, "is it safe to put money in?", true},
		{AutonomyLow, "how do I pay for the package?", true},
		{AutonomyLow, "how much can I earn?", true},
		{AutonomyLow, "be honest, is this a pyramid scheme?", true},
		{AutonomyMedium, "is it safe to put money in?", false},
		{AutonomyMedium, "how much can I earn?", false},
		{AutonomyMedium, "be honest, is this a pyramid scheme?", false},
		{AutonomyMedium, "I want to complain about how slow this is", true},
		{AutonomyMedium, "can I talk to a real person?", true},
		{AutonomyHigh, "I want to complain about how slow this is", false},
		{AutonomyHigh, "can I talk to a real person?", false},
		{AutonomyHigh, "you scammed me, I want a refund", true},
		{AutonomyHigh, "my lawyer will contact you", true},
		{AutonomyHigh, "my account got hacked", true},
		{"", "how much can I earn?", false},      // unset means balanced
		{"", "I complained twice already", true}, // …with balanced's nets
		{"bogus", "I want a refund", true},       // unknown means balanced too
	}
	for _, c := range cases {
		r, m := run(c.level, c.msg, "Hey Raj! It depends on you, let's talk on a quick call.")
		if r.Handoff != c.handoff {
			t.Errorf("%s / %q: handoff=%v, want %v (forced by %q)", c.level, c.msg, r.Handoff, c.handoff, m.ForcedHandoff)
		}
	}

	for _, script := range []string{"You can easily earn five thousand dollars a month!", "People make $2,000 a week with this.", "Some earn 30% a year on it."} {
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

	marks := map[string]string{AutonomyLow: "When in doubt, hand off.", AutonomyMedium: "Handoff is rare", AutonomyHigh: "John trusts you with the whole conversation"}
	for level, mark := range marks {
		p := systemPrompt(Config{Autonomy: level})
		if !strings.Contains(p, mark) || !strings.Contains(p, "Honesty, always") {
			t.Errorf("%s prompt is missing %q or the honesty rule", level, mark)
		}
		for other, m := range marks {
			if other != level && strings.Contains(p, m) {
				t.Errorf("%s prompt also has %s's rules", level, other)
			}
		}
	}
	if p := systemPrompt(Config{}); !strings.Contains(p, marks[AutonomyMedium]) {
		t.Fatal("unset autonomy should get the balanced prompt")
	}
}

// A more cautious level hands John everything a bolder one does, and more.
func TestAutonomyLevelsAreNested(t *testing.T) {
	msgs := []string{
		"what is U.Center?", "is it safe?", "how do I pay?", "how much can I earn?", "is this a pyramid?",
		"my lawyer will contact you", "I lost my money", "I want a refund", "the police are involved", "can we meet tomorrow?",
		"this is an emergency", "you scammed me", "what are the fees?", "I'm worried", "tell me more",
		"I want to complain", "let me talk to a real person", "my account was hacked",
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

// Health follows Claude: a failing request flips it, the next good one
// flips it back, and only changes are reported.
func TestHealthFollowsClaude(t *testing.T) {
	var fail atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if fail.Load() {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"Your credit balance is too low"}}`))
			return
		}
		w.Write([]byte(`{"id":"m","type":"message","role":"assistant","model":"claude-test","stop_reason":"end_turn",
			"content":[{"type":"text","text":"{\"spoken_script\":\"Hi!\",\"followup_text\":\"\",\"include_referral\":false,\"handoff\":false,\"handoff_reason\":\"\",\"opt_out\":false,\"remember\":[]}"}],
			"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
	t.Setenv("ANTHROPIC_API_KEY", "test")
	t.Setenv("BRAIN_MODEL", "claude-test")
	b, err := New("../../data/persona/john.json", "../../data/faq.md")
	if err != nil {
		t.Fatal(err)
	}
	var reports []Health
	b.OnHealth = func(h Health) { reports = append(reports, h) }
	if !b.Health().OK {
		t.Fatal("healthy before the first request")
	}
	say := func() { b.Reply(context.Background(), Contact{Name: "Raj"}, []Turn{{FromLead: true, Text: "hi"}}) }

	say()
	fail.Store(true)
	say()
	say()
	if h := b.Health(); h.OK || !strings.Contains(h.Error, "credit balance") {
		t.Fatalf("a failing request should show: %+v", h)
	}
	fail.Store(false)
	say()
	if !b.Health().OK {
		t.Fatal("a good request clears it")
	}
	if len(reports) != 3 || !reports[0].OK || reports[1].OK || !reports[2].OK {
		t.Fatalf("want ok, failing, ok (each change once), got %+v", reports)
	}
}

func TestInterestRating(t *testing.T) {
	b := &Brain{}
	lead := []Turn{{FromLead: true, Text: "how do I pay?"}}
	for _, tc := range []struct {
		in         Reply
		want, clue string
	}{
		{Reply{SpokenScript: "Sure.", Interest: "Hot", InterestReason: "asked how to pay with USDT."}, "hot", "asked how to pay with USDT"},
		{Reply{SpokenScript: "Sure.", Interest: "lukewarm", InterestReason: "?"}, "", ""},
		{Reply{SpokenScript: "Of course, take care.", OptOut: true, Interest: "warm"}, "cold", "asked not to be contacted"},
		{Reply{SpokenScript: "Sure.", Interest: "warm", InterestReason: "one two three four five six seven eight nine ten eleven twelve thirteen"}, "warm", "one two three four five six seven eight nine ten eleven twelve…"},
	} {
		var m Meta
		r := b.enforce(tc.in, Contact{Name: "Raj"}, lead, &m)
		if r.Interest != tc.want || r.InterestReason != tc.clue {
			t.Errorf("%+v: got %q %q, want %q %q", tc.in, r.Interest, r.InterestReason, tc.want, tc.clue)
		}
	}
	if !strings.Contains(string(schema), `"interest"`) || !strings.Contains(systemPrompt(Config{}), "interest_reason") {
		t.Fatal("the model must be asked for the rating")
	}
}

// By default the link waits until the lead is ready, and the chat stays
// coherent: no "here's my link" without one, and no promise left unkept.
func TestLinkOnceReady(t *testing.T) {
	b := &Brain{}
	b.Configure(Config{ReferralURL: "https://u.center/?ref=john"}) // the default style
	lead := func(s string) []Turn { return []Turn{{FromLead: true, Text: s}} }
	ref := "https://u.center/?ref=john"
	for _, tc := range []struct {
		name, msg string
		in        Reply
		link      bool
		followup  string
	}{
		{"warm lead, a plain answer", "what is U-topia?",
			Reply{SpokenScript: "So, it's one app for your money. What made you curious?", FollowupText: "Here's my link, Raj.", IncludeReferral: true, Interest: "warm"}, false, ""},
		{"hot lead", "ok, how do I pay with USDT?",
			Reply{SpokenScript: "Easy, I'll walk you through it.", IncludeReferral: true, Interest: "hot"}, true, "Here's my link to get started:\n" + ref},
		{"asked to join", "how do I join?",
			Reply{SpokenScript: "Love it.", IncludeReferral: true, Interest: "warm"}, true, "Here's my link to get started:\n" + ref},
		{"the voice note promised it", "sounds good",
			Reply{SpokenScript: "Great, I'm sending you my link now.", IncludeReferral: true, Interest: "warm"}, true, "Here's my link to get started:\n" + ref},
		{"not wanted, text kept", "what is U-topia?",
			Reply{SpokenScript: "It's one app for your money.", FollowupText: "Any questions, just send a voice note!", Interest: "warm"}, false, "Any questions, just send a voice note!"},
	} {
		var m Meta
		r := b.enforce(tc.in, Contact{Name: "Raj"}, lead(tc.msg), &m)
		if r.IncludeReferral != tc.link || r.FollowupText != tc.followup {
			t.Errorf("%s: link %v, followup %q; want %v, %q", tc.name, r.IncludeReferral, r.FollowupText, tc.link, tc.followup)
		}
	}
	if !strings.Contains(systemPrompt(Config{}), "Keep the conversation going") {
		t.Fatal("the default prompt should keep the conversation going")
	}
}

// Info pages are named by the model and added by code: once per chat, never
// with the referral link, never on a handoff, never an address it made up.
func TestInfoLinks(t *testing.T) {
	b := &Brain{}
	b.Configure(Config{ReferralURL: "https://u.center/?ref=john"})
	chat := func(johnSaid, leadSays string) []Turn {
		h := []Turn{}
		if johnSaid != "" {
			h = append(h, Turn{Text: johnSaid})
		}
		return append(h, Turn{FromLead: true, Text: leadSays})
	}
	for _, tc := range []struct {
		name     string
		history  []Turn
		in       Reply
		info     string
		followup string
	}{
		{"packages", chat("", "how much are the packages?"),
			Reply{SpokenScript: "Starter is twenty-five dollars.", InfoLink: "u.center"}, "u.center", "Here's U.Center, with all the packages and prices:\nhttps://u.center/"},
		{"the app, with John's own words", chat("", "what does the app do?"),
			Reply{SpokenScript: "It's your money in one app.", FollowupText: "Have a look around:", InfoLink: "u-topia.com"}, "u-topia.com", "Have a look around:\nhttps://u-topia.com/"},
		{"already shared", chat("Here's more about the app:\nhttps://u-topia.com/", "cool"),
			Reply{SpokenScript: "Glad you like it.", FollowupText: "Here it is again:", InfoLink: "u-topia.com"}, "", ""},
		{"referral link wins", chat("", "how do I join?"),
			Reply{SpokenScript: "Let's do it.", IncludeReferral: true, InfoLink: "u.center"}, "", "Here's my link to get started:\nhttps://u.center/?ref=john"},
		{"they have the referral link, which is u.center", chat("Here's my link:\nhttps://u.center/?ref=john", "and the prices?"),
			Reply{SpokenScript: "Starter is twenty-five dollars.", InfoLink: "u.center"}, "", ""},
		{"handoff", chat("", "I paid and nothing arrived"),
			Reply{SpokenScript: "Oh no.", Handoff: true, InfoLink: "u-topia.com"}, "", ""},
		{"not a page we know", chat("", "hi"),
			Reply{SpokenScript: "Hi!", FollowupText: "See https://evil.example", InfoLink: "evil.example"}, "", "See"},
		{"none", chat("", "hi"),
			Reply{SpokenScript: "Hi!", InfoLink: "none"}, "", ""},
	} {
		var m Meta
		r := b.enforce(tc.in, Contact{Name: "Raj"}, tc.history, &m)
		if r.InfoLink != tc.info || r.FollowupText != tc.followup {
			t.Errorf("%s: info %q, followup %q; want %q, %q", tc.name, r.InfoLink, r.FollowupText, tc.info, tc.followup)
		}
	}
}

// A campaign opener starts a conversation: no link unless the style sends
// it in the first message.
func TestOpenerLink(t *testing.T) {
	var prompt atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		prompt.Store(string(body))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"m","type":"message","role":"assistant","model":"claude-test","stop_reason":"end_turn",
			"content":[{"type":"text","text":"{\"spoken_script\":\"Hey Anna! Thought of you. Have you heard of U-topia?\",\"followup_text\":\"\",\"include_referral\":true,\"handoff\":false,\"handoff_reason\":\"\",\"opt_out\":false,\"remember\":[],\"interest\":\"warm\",\"interest_reason\":\"\",\"info_link\":\"none\"}"}],
			"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
	t.Setenv("ANTHROPIC_API_KEY", "test")
	t.Setenv("BRAIN_MODEL", "claude-test")
	b, err := New("../../data/persona/john.json", "../../data/faq.md")
	if err != nil {
		t.Fatal(err)
	}
	b.Configure(Config{Persona: "{}", FAQ: "facts", ReferralURL: "https://u.center/?ref=john"})
	r, _, err := b.Opener(context.Background(), Contact{Name: "Anna"})
	if err != nil {
		t.Fatal(err)
	}
	if r.IncludeReferral || strings.Contains(r.FollowupText, "u.center") || !strings.Contains(prompt.Load().(string), "include_referral is false") {
		t.Fatalf("default style: no link in an opener: %+v", r)
	}
	b.Configure(Config{Persona: "{}", FAQ: "facts", ReferralURL: "https://u.center/?ref=john", Style: Style{LinkTiming: LinkFirstReply}})
	if r, _, _ = b.Opener(context.Background(), Contact{Name: "Anna"}); !r.IncludeReferral || !strings.HasSuffix(r.FollowupText, "https://u.center/?ref=john") {
		t.Fatalf("link in the first reply: the opener carries it: %+v", r)
	}
}
