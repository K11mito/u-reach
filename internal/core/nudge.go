package core

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"

	"ureach/internal/brain"
	"ureach/internal/store"
)

// The follow-up queue. A lead who hasn't answered John's last message for a
// couple of days gets a short personal follow-up drafted in the Campaign tab;
// nothing goes out until John approves it. Drafts are text only: the voice is
// recorded when John listens or sends, so skipped drafts cost no credits.

// NudgeConfig tunes the follow-up queue. Zero values use the defaults.
type NudgeConfig struct {
	After  time.Duration // quiet this long before a follow-up (48h)
	MaxAge time.Duration // quiet longer than this and a follow-up comes out of the blue (30 days)
	Every  time.Duration // draft automatically this often; 0 only drafts when John asks
	PerRun int           // most drafts per pass (5)
	PerDay int           // most automatic drafts per day (20)
	Quick  bool          // skip the human-like pauses while sending (tests)
}

func (n NudgeConfig) normalized() NudgeConfig {
	if n.After <= 0 {
		n.After = 48 * time.Hour
	}
	if n.MaxAge <= n.After {
		n.MaxAge = max(30*24*time.Hour, 2*n.After)
	}
	if n.PerRun <= 0 {
		n.PerRun = 5
	}
	if n.PerDay <= 0 {
		n.PerDay = 20
	}
	return n
}

// ErrDrafting means follow-ups are already being drafted.
var ErrDrafting = errors.New("follow-ups are already being drafted")

// maxScript caps an edited follow-up, so a pasted essay can't burn credits.
const maxScript = 700

// NudgeItem is a nudge with what the Campaign tab shows about the lead.
type NudgeItem struct {
	store.Nudge
	Name           string `json:"name"`
	Avatar         string `json:"avatar,omitempty"`
	Demo           bool   `json:"demo"` // made-up number: added to the chat, never sent on WhatsApp
	Interest       string `json:"interest"`
	InterestReason string `json:"interest_reason"`
	LastLeadText   string `json:"last_lead_text"`
	LinkSent       bool   `json:"link_sent"`
}

// NudgeQueue is the Follow-ups card.
type NudgeQueue struct {
	Items      []NudgeItem `json:"items"`       // waiting for John, then the last couple of days' outcomes
	Waiting    int         `json:"waiting"`     // quiet leads not drafted yet
	AfterHours int         `json:"after_hours"` // how long counts as quiet
	Auto       bool        `json:"auto"`        // drafts appear on their own
}

type nudger struct {
	drafting sync.Mutex
}

// Queue lists the follow-ups for the Campaign tab.
func (c *Core) Queue() (NudgeQueue, error) {
	cfg := c.Nudges.normalized()
	q := NudgeQueue{AfterHours: int(cfg.After.Hours()), Auto: c.Nudges.Every > 0}
	all, err := c.Store.Nudges()
	if err != nil {
		return q, err
	}
	recent := store.Now() - (48 * time.Hour).Milliseconds()
	var done []NudgeItem
	for _, n := range all {
		pending := n.Status == store.NudgeDraft || n.Status == store.NudgeFailed || n.Status == store.NudgeSending
		if !pending && n.UpdatedAt < recent {
			continue
		}
		it, err := c.nudgeItem(n)
		if err != nil {
			continue
		}
		if pending {
			q.Items = append(q.Items, it)
		} else {
			done = append(done, it)
		}
	}
	// Newest outcomes first, after everything still waiting.
	for i := len(done) - 1; i >= 0 && len(done)-i <= 12; i-- {
		q.Items = append(q.Items, done[i])
	}
	quiet, err := c.quietLeads()
	q.Waiting = len(quiet)
	return q, err
}

func (c *Core) nudgeItem(n store.Nudge) (NudgeItem, error) {
	ct, err := c.Store.Contact(n.ContactID)
	if err != nil {
		return NudgeItem{}, err
	}
	it := NudgeItem{Nudge: n, Name: ct.DisplayName(), Avatar: ct.Avatar, Demo: store.IsDemoNumber(ct.Phone),
		Interest: ct.Interest, InterestReason: ct.InterestReason, LinkSent: ct.LinkSentAt > 0}
	if it.Name == "" {
		it.Name = "+" + ct.Phone
	}
	msgs, _ := c.Store.Messages(ct.ID, 20)
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Direction == "in" && msgs[i].Text != "" {
			it.LastLeadText = msgs[i].Text
			break
		}
	}
	return it, nil
}

func (c *Core) quietLeads() ([]store.Quiet, error) {
	cfg := c.Nudges.normalized()
	return c.Store.QuietLeads(store.Now(), cfg.After.Milliseconds(), cfg.MaxAge.Milliseconds())
}

// DraftNudges writes follow-ups for up to limit quiet leads, longest silence
// first, and returns how many it drafted. A lead whose draft fails because
// Claude is unavailable is left for the next pass; one whose draft breaks a
// rule (it promised money) is kept as failed for John to redraft or skip.
func (c *Core) DraftNudges(ctx context.Context, limit int) (int, error) {
	if !c.nudge.drafting.TryLock() {
		return 0, ErrDrafting
	}
	defer c.nudge.drafting.Unlock()

	quiet, err := c.quietLeads()
	if err != nil {
		return 0, err
	}
	if len(quiet) > limit {
		quiet = quiet[:limit]
	}
	drafts := make([]store.Nudge, len(quiet))
	errs := make([]error, len(quiet))
	var wg sync.WaitGroup
	for i, q := range quiet {
		wg.Add(1)
		go func() {
			defer wg.Done()
			drafts[i], errs[i] = c.writeNudge(ctx, q.Contact, q.LastOut)
		}()
	}
	wg.Wait()

	made := 0
	var firstErr error
	for i, n := range drafts {
		if errs[i] != nil {
			log.Printf("nudge: %s: %v", quiet[i].Contact.DisplayName(), errs[i])
			if firstErr == nil {
				firstErr = errs[i]
			}
			continue
		}
		// They may have written back while Claude was thinking.
		if ct, err := c.Store.Contact(n.ContactID); err != nil || c.nudgeBlocked(ct, n) != "" {
			continue
		}
		if _, err := c.Store.AddNudge(n); err != nil {
			log.Printf("nudge: store: %v", err)
			continue
		}
		made++
	}
	if made > 0 {
		c.Hub.Publish("nudges", nil)
	}
	if made == 0 && firstErr != nil {
		return 0, firstErr
	}
	return made, nil
}

// writeNudge asks the brain for a follow-up to the silence that began at
// quietSince. An error means nothing was written (try again later); a
// returned nudge may still be failed when the draft broke a rule.
func (c *Core) writeNudge(ctx context.Context, ct store.Contact, quietSince int64) (store.Nudge, error) {
	n := store.Nudge{ContactID: ct.ID, QuietSince: quietSince, Status: store.NudgeDraft}
	history, err := c.history(ct.ID)
	if err != nil {
		return n, err
	}
	bc := brain.Contact{Name: firstName(ct.DisplayName()), Context: ct.Context, Memory: ct.Memory, LinkSent: ct.LinkSentAt > 0}
	quiet := time.Since(time.UnixMilli(quietSince))
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	r, meta, err := c.Brain.Nudge(ctx, bc, history, quiet)
	if err != nil {
		return n, err
	}
	if r.Handoff || r.OptOut || strings.TrimSpace(r.SpokenScript) == "" {
		n.Status = store.NudgeFailed
		n.Error = "The AI held this one back. Redraft, write it yourself, or skip."
		if strings.HasPrefix(meta.ForcedHandoff, "income claim") {
			n.Error = "The draft talked about earnings, so it was held back. Redraft, write it yourself, or skip."
		}
		return n, nil
	}
	n.Script = r.SpokenScript
	// The link goes in at send time, and only if they still don't have it.
	ref := c.Brain.Referral()
	n.Link = r.IncludeReferral && ref != "" && ct.LinkSentAt == 0 && c.Brain.Style().LinkTiming != brain.LinkWhenAsked
	n.Followup = strings.TrimSpace(strings.TrimSuffix(r.FollowupText, ref))
	if n.Followup == "Here's my link to get started:" {
		n.Followup = ""
	}
	return n, nil
}

// Redraft writes a fresh follow-up in place of a draft John didn't like.
func (c *Core) Redraft(ctx context.Context, id int64) (store.Nudge, error) {
	n, err := c.Store.Nudge(id)
	if err != nil {
		return n, err
	}
	if n.Status != store.NudgeDraft && n.Status != store.NudgeFailed {
		return n, fmt.Errorf("this follow-up is %s", n.Status)
	}
	ct, err := c.Store.Contact(n.ContactID)
	if err != nil {
		return n, err
	}
	if why := c.nudgeBlocked(ct, n); why != "" {
		c.retire(n, why)
		return n, fmt.Errorf("not needed any more: %s", why)
	}
	fresh, err := c.writeNudge(ctx, ct, n.QuietSince)
	if err != nil {
		return n, err
	}
	n.Script, n.Followup, n.Link, n.Status, n.Error = fresh.Script, fresh.Followup, fresh.Link, fresh.Status, fresh.Error
	n.AudioPath, n.DurationMS = "", 0
	return n, c.saveDraft(n)
}

// EditNudge saves John's changes to a follow-up's words.
func (c *Core) EditNudge(id int64, script, followup string) (store.Nudge, error) {
	n, err := c.Store.Nudge(id)
	if err != nil {
		return n, err
	}
	script, followup = strings.TrimSpace(script), strings.TrimSpace(followup)
	if script == "" {
		return n, errors.New("the voice note needs some words")
	}
	if len(script) > maxScript {
		return n, fmt.Errorf("keep the voice note under %d characters", maxScript)
	}
	if script != n.Script {
		n.AudioPath, n.DurationMS = "", 0 // record it again
	}
	n.Script, n.Followup = script, followup
	if n.Status == store.NudgeFailed {
		n.Status, n.Error = store.NudgeDraft, ""
	}
	return n, c.saveDraft(n)
}

// ListenNudge records the follow-up in John's voice so he can hear it first.
// The audio is cached, so sending it afterwards costs nothing more.
func (c *Core) ListenNudge(ctx context.Context, id int64) (store.Nudge, error) {
	n, err := c.Store.Nudge(id)
	if err != nil {
		return n, err
	}
	if n.AudioPath != "" {
		return n, nil
	}
	if n.Script == "" {
		return n, errors.New("nothing to listen to yet")
	}
	vn, mp3, err := c.speak(ctx, 0, n.Script)
	if err != nil {
		return n, err
	}
	n.AudioPath, n.DurationMS = mp3, int64(vn.Duration*1000)
	return n, c.saveDraft(n)
}

// SkipNudge sets a follow-up aside until the lead writes again.
func (c *Core) SkipNudge(id int64) error {
	ok, err := c.Store.SkipNudge(id)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("this follow-up was already sent or set aside")
	}
	c.Hub.Publish("nudges", nil)
	return nil
}

func (c *Core) saveDraft(n store.Nudge) error {
	ok, err := c.Store.SaveDraft(n)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("this follow-up is already being sent")
	}
	c.Hub.Publish("nudges", nil)
	return nil
}

// SendNudges approves follow-ups and sends them one by one with a short
// random gap, like a person working through a list. It returns at once with
// the ones it took; the rest were already sent, sending or set aside.
func (c *Core) SendNudges(ids []int64) ([]int64, error) {
	var claimed []int64
	for _, id := range ids {
		ok, err := c.Store.ClaimNudge(id)
		if err != nil {
			return claimed, err
		}
		if ok {
			claimed = append(claimed, id)
		}
	}
	if len(claimed) == 0 {
		return nil, errors.New("nothing to send: these follow-ups were already sent or set aside")
	}
	c.Hub.Publish("nudges", nil)
	quick := c.Nudges.Quick
	Go("nudges", func() {
		for i, id := range claimed {
			if i > 0 && !quick {
				time.Sleep(time.Duration(4000+rand.IntN(5000)) * time.Millisecond)
			}
			c.sendNudge(context.Background(), id)
			c.Hub.Publish("nudges", nil)
		}
	})
	return claimed, nil
}

// sendNudge sends one claimed follow-up, unless the chat moved on since it
// was drafted. Demo leads (made-up numbers) get it added to their chat only.
func (c *Core) sendNudge(ctx context.Context, id int64) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	n, err := c.Store.Nudge(id)
	if err != nil {
		return // the chat was reset and took its nudges with it
	}
	ct, err := c.Store.Contact(n.ContactID)
	if err != nil {
		return
	}
	if why := c.nudgeBlocked(ct, n); why != "" {
		c.retire(n, why)
		return
	}
	failed := func(err error) {
		log.Printf("nudge: %s: %v", ct.DisplayName(), err)
		n.Status, n.Error = store.NudgeFailed, err.Error()
		c.Store.SaveNudge(n)
	}
	if strings.TrimSpace(n.Script) == "" {
		failed(errors.New("there's nothing to say yet: redraft or write it yourself"))
		return
	}

	demo := store.IsDemoNumber(ct.Phone)
	var to types.JID
	if !demo {
		if to, err = types.ParseJID(ct.Chat); err != nil {
			failed(fmt.Errorf("bad chat %q: %w", ct.Chat, err))
			return
		}
	}
	stopRec := func() {}
	if !demo {
		stopRec = c.Gateway.Recording(to)
	}
	defer stopRec()

	vn, mp3, voiceErr := c.speak(ctx, 0, n.Script) // cached if John listened first
	c.pause(2 * time.Second)                       // let "recording audio..." show
	// The last moment to notice they wrote back while the note was recorded.
	if ct, err = c.Store.Contact(n.ContactID); err != nil {
		return
	}
	if why := c.nudgeBlocked(ct, n); why != "" {
		c.retire(n, why)
		return
	}
	if voiceErr == nil {
		voiceErr = c.sendVoice(ctx, ct.ID, 0, to, demo, n.Script, vn, mp3)
	}
	stopRec()
	if voiceErr != nil {
		// Same as a reply: the words still reach them, as text.
		log.Printf("nudge: voice note for %s failed, sending text: %v", ct.DisplayName(), voiceErr)
		if err := c.sendText(ctx, ct.ID, 0, to, demo, n.Script); err != nil {
			failed(err)
			return
		}
	} else {
		n.AudioPath, n.DurationMS = mp3, int64(vn.Duration*1000)
	}

	text := n.Followup
	ref := c.Brain.Referral()
	link := n.Link && ref != "" && ct.LinkSentAt == 0
	if link {
		if text == "" {
			text = "Here's my link:"
		}
		text += "\n" + ref
	}
	if text != "" {
		c.pause(1200 * time.Millisecond)
		if err := c.sendText(ctx, ct.ID, 0, to, demo, text); err != nil {
			log.Printf("nudge: follow-up text for %s: %v", ct.DisplayName(), err)
		} else if link {
			c.Store.MarkLinkSent(ct.ID)
		}
	}
	n.Status, n.Error, n.SentAt = store.NudgeSent, "", store.Now()
	c.Store.SaveNudge(n)
	c.Store.AddEvent(ct.ID, "nudge_sent", map[string]any{"nudge_id": n.ID, "demo": demo})
	c.publishContact(ct.ID)
}

// nudgeBlocked says why a follow-up shouldn't go out any more, or "".
// It only fits the exact silence it was written for: John's message at
// QuietSince still being the last word in a chat the AI is handling.
func (c *Core) nudgeBlocked(ct store.Contact, n store.Nudge) string {
	switch ct.Status {
	case store.StatusAuto:
	case store.StatusHuman:
		return "John took over the chat"
	case store.StatusNeedsJohn:
		return "the chat is waiting for John"
	default:
		return "the chat isn't with the AI"
	}
	last, err := c.Store.Messages(ct.ID, 1)
	if err != nil || len(last) == 0 {
		return "the chat is empty"
	}
	if last[0].Direction == "in" {
		return "they wrote back"
	}
	if last[0].CreatedAt != n.QuietSince {
		return "a newer message went out since"
	}
	return ""
}

// retire marks a follow-up as no longer needed.
func (c *Core) retire(n store.Nudge, why string) {
	n.Status, n.Error = store.NudgeStale, why
	c.Store.SaveNudge(n)
	c.Hub.Publish("nudges", nil)
}

// staleNudges retires a lead's waiting follow-ups when the chat moves on.
func (c *Core) staleNudges(contactID int64, why string) {
	if k, err := c.Store.StaleNudges(contactID, why); err == nil && k > 0 {
		c.Hub.Publish("nudges", nil)
	}
}

func (c *Core) pause(d time.Duration) {
	if !c.Nudges.Quick {
		time.Sleep(d)
	}
}

// RunNudges drafts follow-ups in the background every Nudges.Every, within
// the per-run and per-day caps, until ctx ends. Sending always waits for John.
func (c *Core) RunNudges(ctx context.Context) {
	if err := c.Store.RecoverNudges(); err != nil {
		log.Printf("nudge: recover: %v", err)
	}
	if c.Nudges.Every <= 0 {
		return
	}
	t := time.NewTicker(c.Nudges.Every)
	defer t.Stop()
	for {
		c.autoDraft(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (c *Core) autoDraft(ctx context.Context) {
	cfg := c.Nudges.normalized()
	y, m, d := time.Now().Date()
	today, err := c.Store.NudgesSince(time.Date(y, m, d, 0, 0, 0, 0, time.Local).UnixMilli())
	if err != nil {
		log.Printf("nudge: %v", err)
		return
	}
	left := cfg.PerDay - today
	if left <= 0 {
		return
	}
	made, err := c.DraftNudges(ctx, min(cfg.PerRun, left))
	if err != nil && !errors.Is(err, ErrDrafting) {
		log.Printf("nudge: auto draft: %v", err)
	}
	if made > 0 {
		log.Printf("nudge: drafted %d follow-up(s) for quiet leads", made)
	}
}
