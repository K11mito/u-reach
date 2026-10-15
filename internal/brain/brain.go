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
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"

	"ureach/internal/config"
)

// MaxWords caps the spoken script: ~110 words is ~45 s of speech.
const MaxWords = 110

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
	Remember        []string `json:"remember"`
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

// Brain holds the client and the cached system prompt (persona + FAQ + rules).
type Brain struct {
	Model       string
	Effort      string
	ReferralURL string

	client anthropic.Client
	system string
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
	return &Brain{
		Model:       config.Get("BRAIN_MODEL", "claude-opus-5-5"),
		Effort:      config.Get("BRAIN_EFFORT", "low"),
		ReferralURL: config.Get("REFERRAL_URL", ""),
		client:      anthropic.NewClient(option.WithRequestTimeout(45 * time.Second)),
		system:      systemPrompt(string(persona), string(faq)),
	}, nil
}

var schema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["spoken_script", "followup_text", "include_referral", "handoff", "handoff_reason", "remember"],
  "properties": {
    "spoken_script":    {"type": "string"},
    "followup_text":    {"type": "string"},
    "include_referral": {"type": "boolean"},
    "handoff":          {"type": "boolean"},
    "handoff_reason":   {"type": "string"},
    "remember":         {"type": "array", "items": {"type": "string"}}
  }
}`)

// fallbackModels accept server-side refusal fallbacks ("fallbacks": "default").
var fallbackModels = map[string]bool{
	"claude-opus-5-5": true, "claude-sonnet-5-5": true, "claude-opus-5": true, "claude-fable-5-1": true,
}

// Reply writes John's next voice note for the conversation.
func (b *Brain) Reply(ctx context.Context, c Contact, history []Turn) (Reply, Meta, error) {
	return b.generate(ctx, userPrompt(c, history), c, history)
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
		"End with one simple question that invites a reply. 40 to 70 words, about 15 to 25 seconds. " +
		"Set include_referral to true and say you're dropping your link below. handoff is false.")
	return b.generate(ctx, s.String(), c, nil)
}

func (b *Brain) generate(ctx context.Context, prompt string, c Contact, history []Turn) (Reply, Meta, error) {
	params := anthropic.BetaMessageNewParams{
		Model:     b.Model,
		MaxTokens: 8000,
		System: []anthropic.BetaTextBlockParam{{
			Text:         b.system,
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
			return Reply{}, meta, fmt.Errorf("claude %d: %w", apiErr.StatusCode, err)
		}
		return Reply{}, meta, fmt.Errorf("claude: %w", err)
	}
	meta.Model = string(msg.Model)
	meta.InputTokens = msg.Usage.InputTokens
	meta.OutputTokens = msg.Usage.OutputTokens
	meta.CacheRead = msg.Usage.CacheReadInputTokens
	meta.CacheWrite = msg.Usage.CacheCreationInputTokens

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
	r = b.enforce(r, c, history, &meta)
	return r, meta, nil
}

var (
	urlRe = regexp.MustCompile(`https?://\S+|www\.\S+`)
	// Topics John answers personally. The model is told the same; this is the
	// net for when it doesn't listen.
	handoffRe = regexp.MustCompile(`(?i)\b(earn\w*|income|commissions?|profits?|returns?|roi|passive|guarantee\w*|refund\w*|withdraw\w*|payouts?|salary|lawsuit|legal|illegal|scam|ponzi|pyramid|tax(es)?)\b`)
	linkAskRe = regexp.MustCompile(`(?i)\b(link|sign ?up|register|join|enrol+)\b`)
	sentEndRe = regexp.MustCompile(`[.!?…](["')\]]?)(\s|$)`)
)

// enforce applies the rules that must hold regardless of what the model wrote.
func (b *Brain) enforce(r Reply, c Contact, history []Turn, meta *Meta) Reply {
	latest := latestLeadText(history)

	if !r.Handoff {
		if kw := handoffRe.FindString(latest); kw != "" {
			meta.ForcedHandoff = strings.ToLower(kw)
			r.Handoff = true
			r.HandoffReason = "asked about " + meta.ForcedHandoff
			r.SpokenScript = holdingLine(c.Name)
			r.FollowupText = ""
		}
	}
	if r.Handoff {
		r.IncludeReferral = false
	}

	r.SpokenScript = strings.TrimSpace(urlRe.ReplaceAllString(r.SpokenScript, ""))
	r.SpokenScript, meta.Trimmed = capWords(r.SpokenScript, MaxWords)
	meta.Words = len(strings.Fields(r.SpokenScript))

	// The model never writes the link; code adds the real one, once, unless asked again.
	r.FollowupText = strings.TrimSpace(urlRe.ReplaceAllString(r.FollowupText, ""))
	r.IncludeReferral = r.IncludeReferral && b.ReferralURL != "" && (!c.LinkSent || linkAskRe.MatchString(latest))
	if r.IncludeReferral {
		if r.FollowupText == "" {
			r.FollowupText = "Here's my link to get started:"
		}
		r.FollowupText += "\n" + b.ReferralURL
	}
	return r
}

func holdingLine(name string) string {
	hey := "Hey"
	if name != "" {
		hey = "Hey " + name
	}
	return hey + ", that's a really important question, and I want to answer it properly myself. " +
		"Give me a little bit and I'll get back to you personally, okay?"
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
	s.WriteString("\nWrite John's next voice note replying to the lead's latest message(s).")
	return s.String()
}

func systemPrompt(persona, faq string) string {
	return `You write WhatsApp voice notes as John, a U.Center ambassador. Your words are spoken aloud by a clone of John's own voice and sent to a lead who messaged him. They must sound exactly like John talking into his phone: warm, personal, and excited to bring this person into U-topia with him as their sponsor.

<persona>
` + persona + `
</persona>

<faq>
` + faq + `
</faq>

How to write spoken_script:
- Write for the ear: short sentences, contractions, John's own openers, phrases and rhythm from the persona. No lists, no markdown, no emojis, no URLs. Write numbers as words.
- Start by greeting the lead by first name, once, if the name looks like a real first name. Otherwise greet without a name.
- You are selling, the way a good friend recommends something they love. Answer what they asked, make it personal to them, then answer the one or two questions they would most likely ask next (what it is, what it costs, how to start).
- Always end by moving them forward with one clear invitation: tell them you're sending your link so they can join with you, and offer to help them set up personally or jump on a quick call.
- 50 to 90 words, about 20 to 35 seconds. Shorter is better than rushed.
- Don't repeat lines you already said earlier in this conversation; vary how you open and close.
- Only state facts that are in the FAQ. If the FAQ doesn't cover something, say you'll walk them through it on a quick call. Never invent numbers, dates, features or promises.
- Never sound like a brochure or a support bot. Never say "I'd be happy to help", "Great question", "As an AI", or "Feel free to reach out".
- Reply in English.

Handoff: if the lead asks about earnings, income, commissions, returns, payments or withdrawals, anything legal or regulatory, complaints, or whether it's a scam or pyramid, set handoff to true. Then spoken_script is only a short, warm holding line (20 to 40 words) saying John will get back to them personally. Do not answer the question. Put the topic in handoff_reason.

followup_text: one short text message sent after the voice note, or an empty string. Never put a URL in it; the system appends John's referral link when include_referral is true.

include_referral: true unless handoff is true or the lead clearly says they're not interested. The system sends the link at most once unless they ask for it again, so when it was already sent, don't say you're sending it again; point them back to it instead.

remember: short new facts about the lead worth keeping (where they live, interests, concerns). Empty if none.`
}
