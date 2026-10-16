// Package brain decides what John says: one Claude call per reply that returns
// a spoken script, a follow-up text, and flags. Code then enforces the rules
// that must never depend on the model (length cap, no URLs aloud, handoff on
// money and legal topics, referral link at most once).
package brain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"

	"ureach/internal/config"
)

// MaxWords caps the spoken script for the default style: ~110 words is
// ~45 s of speech. A style's cap is its MaxWords plus a little slack.
const MaxWords = 110

// Link timing: when John's referral link goes out.
const (
	LinkFirstReply     = "first_reply"     // in the first reply (the original behaviour)
	LinkWhenInterested = "when_interested" // once the lead is ready to join (the default)
	LinkWhenAsked      = "when_asked"      // only when they ask how to join (enforced in code)
)

// Style is how John replies. Each dashboard preset carries one.
type Style struct {
	MinWords   int    `json:"min_words"`
	MaxWords   int    `json:"max_words"`
	LinkTiming string `json:"link_timing"`
}

// DefaultStyle: 50 to 90 words, and the link only once they're ready to join.
// Like John, the AI keeps the conversation going first.
var DefaultStyle = Style{MinWords: 50, MaxWords: 90, LinkTiming: LinkWhenInterested}

// Info pages John can point a lead to. The model only names one; code adds
// the address, so a link in a reply is always one of these.
var infoLinks = map[string]struct{ URL, Intro string }{
	"u.center":    {"https://u.center/", "Here's U.Center, with all the packages and prices:"},
	"u-topia.com": {"https://u-topia.com/", "Here's more about the app:"},
}

// Normalized fills unset fields from DefaultStyle and keeps lengths sane.
func (st Style) Normalized() Style {
	if st.MaxWords <= 0 {
		st.MaxWords = DefaultStyle.MaxWords
	}
	if st.MinWords <= 0 {
		st.MinWords = DefaultStyle.MinWords
	}
	st.MaxWords = min(max(st.MaxWords, 20), 150)
	st.MinWords = min(max(st.MinWords, 10), st.MaxWords)
	switch st.LinkTiming {
	case LinkFirstReply, LinkWhenInterested, LinkWhenAsked:
	default:
		st.LinkTiming = DefaultStyle.LinkTiming
	}
	return st
}

// Doc is a sales playbook document John reads before every reply.
type Doc struct{ Name, Text string }

// Config is everything about John that can be edited from the dashboard.
type Config struct {
	Persona     string // persona card, JSON
	FAQ         string // facts the assistant may state, markdown
	Playbook    []Doc  // active sales playbook documents
	Style       Style
	ReferralURL string
	Autonomy    string // low | medium | high; see autonomy.go
}

// Contact is what we know about the lead.
type Contact struct {
	Name     string   // DB name wins over WhatsApp push name
	Context  string   // e.g. "met at the Bangkok meetup"
	Memory   []string // facts remembered from earlier replies
	LinkSent bool     // referral link already sent to them
}

// Turn is one message in the chat, oldest first. The lead's newest
// message(s) are last.
type Turn struct {
	FromLead bool
	Voice    bool // a voice note (Text is its transcript or script)
	Text     string
}

// Reply is the brain's contract.
type Reply struct {
	SpokenScript    string   `json:"spoken_script"`
	FollowupText    string   `json:"followup_text"`
	IncludeReferral bool     `json:"include_referral"`
	Handoff         bool     `json:"handoff"`
	HandoffReason   string   `json:"handoff_reason"`
	OptOut          bool     `json:"opt_out"` // asked to stop; the chat pauses for John
	Remember        []string `json:"remember"`
	// How close the lead is to joining (cold | warm | hot), judged from the
	// whole conversation, and the clue: the Inbox's triage.
	Interest       string `json:"interest"`
	InterestReason string `json:"interest_reason"`
	// An info page to share after the note: "u.center", "u-topia.com" or
	// "none". After enforce, "" when nothing was added.
	InfoLink string `json:"info_link"`
}

// Meta describes how a reply was produced, for logs and the Stage view.
type Meta struct {
	Model         string
	Latency       time.Duration
	InputTokens   int64
	OutputTokens  int64
	CacheRead     int64
	CacheWrite    int64
	Words         int
	Trimmed       bool   // script was over MaxWords and cut at a sentence end
	ForcedHandoff string // keyword that forced a handoff the model missed
}

// Brain holds the client and the cached system prompt (persona + FAQ +
// playbook + rules). Configure swaps them live; replies in flight keep the
// version they started with.
type Brain struct {
	Model       string
	Effort      string
	ReferralURL string       // read through Referral(); set through Configure
	OnUsage     func(Meta)   // called after every billed request, e.g. to record spend
	OnHealth    func(Health) // called when Claude starts or stops answering

	client   anthropic.Client
	mu       sync.RWMutex
	system   string
	style    Style
	autonomy string
	health   Health
}

// Health is whether the last request reached Claude, for the dashboard: when
// the API key or its credit runs out, John sees it before the leads pile up.
type Health struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	At    int64  `json:"at,omitempty"` // Unix ms of the last request
}

// Health reports the last request's outcome (OK before the first one).
func (b *Brain) Health() Health {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.health.At == 0 {
		return Health{OK: true}
	}
	return b.health
}

// setHealth records a request's outcome and reports a change.
func (b *Brain) setHealth(err error) {
	h := Health{OK: err == nil, At: time.Now().UnixMilli()}
	if err != nil {
		h.Error = err.Error()
	}
	b.mu.Lock()
	changed := b.health.At == 0 || b.health.OK != h.OK || b.health.Error != h.Error
	b.health = h
	b.mu.Unlock()
	if changed && b.OnHealth != nil {
		b.OnHealth(h)
	}
}

// New loads the persona card and FAQ. Call config.Load first.
func New(personaPath, faqPath string) (*Brain, error) {
	persona, err := os.ReadFile(personaPath)
	if err != nil {
		return nil, err
	}
	if !json.Valid(persona) {
		return nil, fmt.Errorf("%s is not valid JSON", personaPath)
	}
	faq, err := os.ReadFile(faqPath)
	if err != nil {
		return nil, err
	}
	b := &Brain{
		Model:  config.Get("BRAIN_MODEL", "claude-opus-5-5"),
		Effort: config.Get("BRAIN_EFFORT", "low"),
		client: anthropic.NewClient(option.WithRequestTimeout(45 * time.Second)),
	}
	b.Configure(Config{Persona: string(persona), FAQ: string(faq), Style: DefaultStyle, ReferralURL: config.Get("REFERRAL_URL", "")})
	return b, nil
}

// Configure rebuilds the system prompt. Safe to call while replies run.
func (b *Brain) Configure(c Config) {
	c.Style = c.Style.Normalized()
	system := systemPrompt(c)
	b.mu.Lock()
	b.system, b.style, b.ReferralURL, b.autonomy = system, c.Style, c.ReferralURL, NormalizeAutonomy(c.Autonomy)
	b.mu.Unlock()
}

// Referral is John's referral link ("" when none is set).
func (b *Brain) Referral() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.ReferralURL
}

// Style is the reply style currently in use.
func (b *Brain) Style() Style {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.style.Normalized()
}

// SystemPrompt is the prompt currently sent with every reply (for previews).
func (b *Brain) SystemPrompt() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.system
}

var schema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["spoken_script", "followup_text", "include_referral", "handoff", "handoff_reason", "opt_out", "remember", "interest", "interest_reason", "info_link"],
  "properties": {
    "spoken_script":    {"type": "string"},
    "followup_text":    {"type": "string"},
    "include_referral": {"type": "boolean"},
    "handoff":          {"type": "boolean"},
    "handoff_reason":   {"type": "string"},
    "opt_out":          {"type": "boolean"},
    "remember":         {"type": "array", "items": {"type": "string"}},
    "interest":         {"type": "string", "enum": ["cold", "warm", "hot"]},
    "interest_reason":  {"type": "string"},
    "info_link":        {"type": "string", "enum": ["none", "u.center", "u-topia.com"]}
  }
}`)

// fallbackModels accept server-side refusal fallbacks ("fallbacks": "default").
var fallbackModels = map[string]bool{
	"claude-opus-5-5": true, "claude-sonnet-5-5": true, "claude-opus-5": true, "claude-fable-5-1": true,
}

// Reply writes John's next voice note for the conversation.
func (b *Brain) Reply(ctx context.Context, c Contact, history []Turn) (Reply, Meta, error) {
	return b.generate(ctx, b.current(), userPrompt(c, history), c, history)
}

// Preview writes a reply with a draft config, for trying unsaved edits.
// Live replies keep using the configured version.
func (b *Brain) Preview(ctx context.Context, cfg Config, c Contact, history []Turn) (Reply, Meta, error) {
	cfg.Style = cfg.Style.Normalized()
	return b.generate(ctx, setup{system: systemPrompt(cfg), style: cfg.Style, referral: cfg.ReferralURL, autonomy: NormalizeAutonomy(cfg.Autonomy)}, userPrompt(c, history), c, history)
}

// setup is one consistent view of the config, taken once per reply.
type setup struct {
	system, referral, autonomy string
	style                      Style
}

func (b *Brain) current() setup {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return setup{system: b.system, style: b.style.Normalized(), referral: b.ReferralURL, autonomy: NormalizeAutonomy(b.autonomy)}
}

// Nudge writes a follow-up for a lead who hasn't answered John's last message
// for a while. There's no new message from them, so the chat goes into the
// prompt rather than to enforce: the lead-side handoff net is about what a
// lead just said, which doesn't apply. The income-claim net still does.
func (b *Brain) Nudge(ctx context.Context, c Contact, history []Turn, quiet time.Duration) (Reply, Meta, error) {
	days := max(2, int(quiet.Hours()/24))
	prompt := conversation(c, history) + fmt.Sprintf(`
They haven't replied for %d days since John's last message. Write John's short follow-up voice note:
- Pick up exactly where the conversation left off and mention something specific they said or asked.
- Add one small, genuinely useful thing from the knowledge base that fits them and hasn't been said yet.
- End with one easy question they can answer in a few words.
- If they've already joined, ask how they're getting on and offer one helpful next step instead of selling.
- Warm and light, no pressure and no guilt. Never say "just checking in", "following up", "did you get my message" or "haven't heard from you". They know it's John, so don't introduce yourself.
- 25 to 55 words, about 10 to 20 seconds.
handoff and opt_out are false. followup_text is usually empty. info_link: a page only if it fits what they asked about and you haven't shared it yet. include_referral: true only if they never got the link and were clearly ready to join; then say you're dropping your link below, rather than asking whether to send it.`, days)
	return b.generate(ctx, b.current(), prompt, c, nil)
}

// Opener writes John's first, unprompted voice note to someone he knows
// (the outbound campaign). Replies then flow through Reply.
func (b *Brain) Opener(ctx context.Context, c Contact) (Reply, Meta, error) {
	var s strings.Builder
	fmt.Fprintf(&s, "Today is %s.\n", time.Now().Format("Monday 2 January 2006"))
	fmt.Fprintf(&s, "Write John's FIRST voice note to %s. He is reaching out personally; they haven't messaged him.\n", c.Name)
	if c.Context != "" {
		fmt.Fprintf(&s, "How John knows them: %s\n", c.Context)
	}
	if len(c.Memory) > 0 {
		fmt.Fprintf(&s, "What John remembers about them: %s\n", strings.Join(c.Memory, "; "))
	}
	s.WriteString("Open warmly by name and mention how you know them. In a sentence or two, say why you thought of them and what U-topia is, made personal to them. " +
		"End with one simple question that invites a reply. 40 to 70 words, about 15 to 25 seconds. handoff is false. info_link is none. ")
	cfg := b.current()
	if cfg.style.LinkTiming == LinkFirstReply {
		s.WriteString("Set include_referral to true and say you're dropping your link below.")
	} else {
		// A first message starts a conversation; the link comes once they're ready.
		s.WriteString("Don't mention or send your link yet: include_referral is false.")
		cfg.referral = ""
	}
	return b.generate(ctx, cfg, s.String(), c, nil)
}

func (b *Brain) generate(ctx context.Context, cfg setup, prompt string, c Contact, history []Turn) (Reply, Meta, error) {
	params := anthropic.BetaMessageNewParams{
		Model:     b.Model,
		MaxTokens: 8000,
		System: []anthropic.BetaTextBlockParam{{
			Text:         cfg.system,
			CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
		}},
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(prompt)),
		},
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: anthropic.BetaOutputConfigEffort(b.Effort),
			Format: anthropic.BetaJSONOutputFormatParam{Schema: schema},
		},
	}
	if fallbackModels[b.Model] {
		params.Fallbacks = anthropic.BetaFallbacksParamUnion{OfDefault: constant.ValueOf[constant.Default]()}
		params.Betas = []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01}
	}

	t0 := time.Now()
	msg, err := b.client.Beta.Messages.New(ctx, params)
	meta := Meta{Model: b.Model, Latency: time.Since(t0)}
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			err = fmt.Errorf("claude %d: %w", apiErr.StatusCode, err)
		} else {
			err = fmt.Errorf("claude: %w", err)
		}
		if ctx.Err() == nil { // a request we gave up on says nothing about Claude
			b.setHealth(err)
		}
		return Reply{}, meta, err
	}
	b.setHealth(nil)
	meta.Model = string(msg.Model)
	meta.InputTokens = msg.Usage.InputTokens
	meta.OutputTokens = msg.Usage.OutputTokens
	meta.CacheRead = msg.Usage.CacheReadInputTokens
	meta.CacheWrite = msg.Usage.CacheCreationInputTokens
	if b.OnUsage != nil {
		b.OnUsage(meta) // billed even if the reply is refused or cut off below
	}

	switch msg.StopReason {
	case anthropic.BetaStopReasonRefusal:
		return Reply{}, meta, fmt.Errorf("claude refused (%s)", msg.StopDetails.Category)
	case anthropic.BetaStopReasonMaxTokens:
		return Reply{}, meta, errors.New("claude hit max_tokens")
	}
	var text strings.Builder
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	var r Reply
	if err := json.Unmarshal([]byte(text.String()), &r); err != nil {
		return Reply{}, meta, fmt.Errorf("brain JSON: %w: %q", err, text.String())
	}
	r = enforce(cfg, r, c, history, &meta)
	return r, meta, nil
}

var (
	urlRe = regexp.MustCompile(`https?://\S+|www\.\S+`)
	// Replies that would promise money. The model is told never to; if one
	// slips through anyway it's swapped for a holding line and John takes over.
	incomeClaimRe = regexp.MustCompile(`(?i)\b(` +
		// "earn/make" plus an amount of money
		`(earn|earned|earning|make|made|making)( up to| over| around| about| at least)? (\$\s?\d[\d,.]*|\d[\d,.]*\s?(k|dollars?|usd|usdt|bucks|baht|euros?|pounds?|%|percent)|(one|two|three|four|five|six|seven|eight|nine|ten|twenty|thirty|forty|fifty|sixty|seventy|eighty|ninety|a|several|hundreds of|thousands of)( hundred| thousand)?( dollars?| bucks| usd| usdt| baht| euros?| pounds?)|a lot of money|good money|serious money|big money|a fortune|six figures)` +
		// money per period, or a rate of return
		`|(dollars?|usd|usdt|bucks|baht|euros?|pounds?) (a|per|every) (day|week|month)|(\d+ ?%|percent) (a|per|every) (day|week|month|year)|double (your|their) (money|investment)` +
		`)`)
	linkAskRe = regexp.MustCompile(`(?i)\b(link|sign ?up|register|join|enrol+)\b`)
	// "I'm sending you my link", "dropping my link below", "here's my link".
	promisesLinkRe = regexp.MustCompile(`(?i)\b(send(ing)?|drop(ping)?|pop(ping)?|here'?s|share|sharing)\b[^.!?]{0,25}\bmy link\b`)
	sentEndRe      = regexp.MustCompile(`[.!?…](["')\]]?)(\s|$)`)
)

// enforce applies the rules that must hold regardless of what the model wrote.
func (b *Brain) enforce(r Reply, c Contact, history []Turn, meta *Meta) Reply {
	return enforce(b.current(), r, c, history, meta)
}

func enforce(cfg setup, r Reply, c Contact, history []Turn, meta *Meta) Reply {
	latest := latestLeadText(history)
	style, referral := cfg.style, cfg.referral

	if !r.Handoff {
		if kw := handoffTrigger(cfg.autonomy, latest); kw != "" {
			meta.ForcedHandoff = strings.ToLower(kw)
			r.Handoff = true
			r.HandoffReason = "mentioned " + meta.ForcedHandoff
			r.SpokenScript = holdingLine(c.Name)
			r.FollowupText = ""
		}
	}
	if claim := incomeClaimRe.FindString(r.SpokenScript + " " + r.FollowupText); claim != "" {
		meta.ForcedHandoff = "income claim: " + strings.ToLower(claim)
		r.Handoff = true
		r.HandoffReason = "reply promised money (" + strings.ToLower(claim) + "); check what to tell them"
		r.SpokenScript = holdingLine(c.Name)
		r.FollowupText = ""
	}
	if r.Handoff || r.OptOut {
		r.IncludeReferral = false
		r.FollowupText = "" // the voice note already says John will follow up
	}

	r.SpokenScript = strings.TrimSpace(urlRe.ReplaceAllString(r.SpokenScript, ""))
	r.SpokenScript, meta.Trimmed = capWords(r.SpokenScript, style.MaxWords+20)
	meta.Words = len(strings.Fields(r.SpokenScript))

	// The model never writes the link; code adds the real one, once, unless asked again.
	r.FollowupText = strings.TrimSpace(urlRe.ReplaceAllString(r.FollowupText, ""))
	asked := linkAskRe.MatchString(latest)
	wanted := r.IncludeReferral
	r.IncludeReferral = r.IncludeReferral && referral != "" && (!c.LinkSent || asked)
	// The timing holds in code too: "only when asked" means asked, and "once
	// ready" means they asked or the AI rates them hot. Openers and
	// follow-ups (no history) decide for themselves.
	r.Interest, r.InterestReason = rateInterest(r)
	if len(history) > 0 && !asked {
		switch style.LinkTiming {
		case LinkWhenAsked:
			r.IncludeReferral = false
		case LinkWhenInterested:
			// A voice note that says the link is coming gets it, so the lead
			// never hears a promise the chat doesn't keep.
			r.IncludeReferral = r.IncludeReferral && (r.Interest == "hot" || promisesLinkRe.MatchString(r.SpokenScript))
		}
	}
	if wanted && !r.IncludeReferral && strings.Contains(strings.ToLower(r.FollowupText), "link") {
		r.FollowupText = "" // "Here's my link…" with no link under it
	}
	if r.IncludeReferral {
		if r.FollowupText == "" {
			r.FollowupText = "Here's my link to get started:"
		}
		r.FollowupText += "\n" + referral
	}

	// An info page, once per chat, and never next to the referral link
	// (that's u.center too). A text that only introduced it goes with it.
	info, ok := infoLinks[strings.ToLower(strings.TrimSpace(r.InfoLink))]
	if ok && !r.IncludeReferral && !r.Handoff && !r.OptOut && !shared(history, info.URL) {
		if r.FollowupText == "" {
			r.FollowupText = info.Intro
		}
		r.FollowupText += "\n" + info.URL
		r.InfoLink = strings.TrimSuffix(strings.TrimPrefix(info.URL, "https://"), "/")
	} else {
		if ok && !r.IncludeReferral && strings.HasSuffix(r.FollowupText, ":") {
			r.FollowupText = ""
		}
		r.InfoLink = ""
	}
	return r
}

// shared says whether John already sent this address in the chat.
func shared(history []Turn, url string) bool {
	for _, t := range history {
		if !t.FromLead && strings.Contains(t.Text, url) {
			return true
		}
	}
	return false
}

// rateInterest keeps the triage tidy: a known level and a short clue.
// Someone who asked to stop is cold whatever the model said.
func rateInterest(r Reply) (level, reason string) {
	level = strings.ToLower(strings.TrimSpace(r.Interest))
	reason = strings.TrimRight(strings.TrimSpace(r.InterestReason), ".")
	if r.OptOut {
		level = "cold"
		if reason == "" {
			reason = "asked not to be contacted"
		}
	}
	switch level {
	case "cold", "warm", "hot":
	default:
		return "", ""
	}
	if words := strings.Fields(reason); len(words) > 12 {
		reason = strings.Join(words[:12], " ") + "…"
	}
	return level, reason
}

// holdingLine replaces a reply the code won't send. It fits a question, a
// complaint or a statement ("my lawyer will contact you") alike.
func holdingLine(name string) string {
	hey := "Hey"
	if name != "" {
		hey = "Hey " + name
	}
	return hey + ", thanks for your message. This is one I want to handle myself, " +
		"so give me a little bit and I'll get back to you personally, okay?"
}

func latestLeadText(history []Turn) string {
	var parts []string
	for i := len(history) - 1; i >= 0 && history[i].FromLead; i-- {
		parts = append([]string{history[i].Text}, parts...)
	}
	return strings.Join(parts, "\n")
}

// capWords cuts text to at most max words, at the last sentence end if possible.
func capWords(text string, max int) (string, bool) {
	words := strings.Fields(text)
	if len(words) <= max {
		return text, false
	}
	cut := strings.Join(words[:max], " ")
	if locs := sentEndRe.FindAllStringIndex(cut, -1); len(locs) > 0 {
		return strings.TrimSpace(cut[:locs[len(locs)-1][1]]), true
	}
	return cut + "...", true
}

func userPrompt(c Contact, history []Turn) string {
	return conversation(c, history) + "\nWrite John's next voice note replying to the lead's latest message(s)."
}

// conversation is what the model knows about a lead: who they are and the
// chat so far.
func conversation(c Contact, history []Turn) string {
	var s strings.Builder
	// The date lives here, not in the cached system prompt.
	fmt.Fprintf(&s, "Today is %s.\n", time.Now().Format("Monday 2 January 2006"))
	name := c.Name
	if name == "" {
		name = "(unknown; don't use a name)"
	}
	fmt.Fprintf(&s, "Lead's name: %s\n", name)
	if c.Context != "" {
		fmt.Fprintf(&s, "How John knows them: %s\n", c.Context)
	}
	if len(c.Memory) > 0 {
		fmt.Fprintf(&s, "What John remembers about them: %s\n", strings.Join(c.Memory, "; "))
	}
	fmt.Fprintf(&s, "Referral link already sent: %v\n\nConversation (oldest first):\n", c.LinkSent)
	for _, t := range history {
		who, kind := "John", "text"
		if t.FromLead {
			who = "Lead"
		}
		if t.Voice {
			kind = "voice note"
		}
		fmt.Fprintf(&s, "[%s, %s] %s\n", who, kind, t.Text)
	}
	return s.String()
}

func systemPrompt(c Config) string {
	st := c.Style.Normalized()
	var playbook strings.Builder
	if len(c.Playbook) > 0 {
		playbook.WriteString("\n<sales_playbook>\nJohn's own sales strategy notes. Follow them for how he approaches leads: what to lead with, how to handle objections, what the next step is. They never override the handoff, opt-out and honesty rules below.\n")
		for _, d := range c.Playbook {
			fmt.Fprintf(&playbook, "<document name=%q>\n%s\n</document>\n", d.Name, strings.TrimSpace(d.Text))
		}
		playbook.WriteString("</sales_playbook>\n")
	}

	closing := "- Always end by moving them forward with one clear invitation: tell them you're sending your link so they can join with you, and offer to help them set up personally or jump on a quick call."
	linkRule := "include_referral: true unless handoff is true or the lead clearly says they're not interested."
	switch st.LinkTiming {
	case LinkWhenInterested:
		closing = "- Keep the conversation going, the way John would: answer, make it personal, then end with one open invitation to keep talking, like asking what else they'd like to know or one warm question about them. Now and then, not every time, say they can just send you a voice note with any questions. Don't end by pushing your link: it comes near the end, once they're clearly ready to join."
		linkRule = "include_referral: true only when the lead is clearly ready to join: they ask how to join or sign up, ask for your link, say they want in, or ask how to pay. A question about prices or packages alone isn't enough: answer it, and share u.center with info_link. False when handoff is true; the system also holds the link back until you rate them hot."
	case LinkWhenAsked:
		closing = "- Don't push your link. End with one warm question about them or an offer of a quick call. Send the link only when they ask how to join or ask for it."
		linkRule = "include_referral: true only when the lead asks how to join or asks for the link. The system enforces this."
	}
	length := fmt.Sprintf("- %d to %d words, about %d to %d seconds. Shorter is better than rushed.", st.MinWords, st.MaxWords, st.MinWords*2/5, st.MaxWords*2/5)

	return `You write WhatsApp voice notes as John, a U.Center ambassador. Your words are spoken aloud by a clone of John's own voice and sent to a lead who messaged him. They must sound exactly like John talking into his phone: warm, personal, and excited to bring this person into U-topia with him as their sponsor.

<persona>
` + c.Persona + `
</persona>

<knowledge_base>
` + c.FAQ + `
</knowledge_base>
` + playbook.String() + `
How to write spoken_script:
- Write for the ear: short sentences, contractions, John's own openers, phrases and rhythm from the persona. No lists, no markdown, no emojis, no URLs. Write numbers as words.
- Start by greeting the lead by first name, once, if the name looks like a real first name. Otherwise greet without a name.
- You are selling, the way a good friend recommends something they love. Answer what they asked, make it personal to them, then answer the one or two questions they would most likely ask next (what it is, what it costs, how to start).
` + closing + `
` + length + `
- Don't repeat lines you already said earlier in this conversation; vary how you open and close.
- The knowledge base and the sales playbook are your facts: answer from them with confidence, the way someone who knows this inside out would. If a detail isn't covered, say what you do know and offer to go through the rest on a quick call. Never invent numbers, dates, features or promises.
- Never sound like a brochure or a support bot. Never say "I'd be happy to help", "Great question", "As an AI", or "Feel free to reach out".
- Reply in English.

` + handoffPrompt(c.Autonomy) + `

Opt-out: if the lead asks you to stop messaging them or clearly says they're not interested, set opt_out to true. Then spoken_script is one or two kind sentences with no pitch, followup_text is empty, and include_referral is false.

followup_text: one short text message sent after the voice note, or an empty string. Use it to introduce a page or the link, or for a light nudge to keep talking (like "Any questions, just send me a voice note!"); never repeat the voice note's question. Never put a URL in it; the system appends the address of the link or page.

` + linkRule + ` The system sends the link at most once unless they ask for it again, so when it was already sent, don't say you're sending it again; point them back to it instead.

info_link: share one of these pages when it really helps, by name; the system adds the address after followup_text, so never write it yourself:
- "u.center": U.Center's site, with the membership packages and their prices, the DAO and the education side. Share it when they ask about packages, prices or the community.
- "u-topia.com": the U-topia app's site: what the app does, the Visa card, the team, its FAQs and the waitlist. Share it when they want to read more about the app.
Otherwise "none". At most one per reply, never one you already shared in this chat, and not when you send your referral link. When you share one, say so naturally in the voice note, like "I'll drop you the page with all the packages".

remember: short new facts about the lead worth keeping (where they live, interests, concerns). Empty if none.

interest: how close the lead is to joining, judged from the whole conversation. "hot": about to join, worth a call from John today: they ask how to pay or join or which package, ask for the link, or say they're signing up now or waiting for a payment to go through. "warm": interested and asking real questions, or already joined and settled in. "cold": just browsing, doubtful, worried or not interested. interest_reason: the clue in eight words or fewer, like "asked how to pay with USDT".`
}
