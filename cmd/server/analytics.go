package main

import (
	"context"
	"hash/fnv"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"ureach/internal/voice"
)

// The analytics page: John's funnel over two weeks, and what U-Reach really
// costs to run.
//
// Days are rolling 24-hour windows ending now, so the newest point is a whole
// day rather than the first few hours of today, and the last seven add up to
// the dashboard's "this week" numbers. Those seven are counted from the
// database. The week before and the purchases are illustrative demo numbers:
// the seeded leads only go back a week, and nothing records sign-ups. Spend is real:
// ElevenLabs reports credits per hour, and every Claude call is priced from
// its token usage as it happens.

type funnelDay struct {
	Date      string `json:"date"` // YYYY-MM-DD the window ends on, local time
	Leads     int    `json:"leads"`
	Voice     int    `json:"voice"`
	Links     int    `json:"links"`
	Purchased int    `json:"purchased"`
}

type spendHour struct {
	T      int64   `json:"t"` // Unix ms, start of the hour
	Eleven float64 `json:"elevenlabs"`
	Claude float64 `json:"claude"`
}

const spendHours = 24

func (a *api) analytics(w http.ResponseWriter, r *http.Request) {
	days, err := a.funnel()
	if err != nil {
		httpErr(w, err)
		return
	}
	hours, spend := a.spend()
	writeJSON(w, map[string]any{
		"days":     days,
		"hours":    hours,
		"spend":    spend,
		"response": a.responseTime(),
	})
}

func (a *api) funnel() ([]funnelDay, error) {
	const dayLen = 24 * time.Hour
	end := time.Now()
	start := end.Add(-14 * dayLen)
	days := make([]funnelDay, 14)
	for i := range days {
		days[i].Date = start.Add(time.Duration(i+1) * dayLen).Format("2006-01-02")
	}
	day := func(ms int64) (int, bool) {
		t := time.UnixMilli(ms)
		if t.Before(start) || t.After(end) {
			return 0, false
		}
		return min(int(t.Sub(start)/dayLen), len(days)-1), true
	}

	msgs, err := a.store.MessagesSince(start.UnixMilli())
	if err != nil {
		return nil, err
	}
	leads := make([]map[int64]bool, len(days))
	for _, m := range msgs {
		i, ok := day(m.CreatedAt)
		if !ok {
			continue
		}
		switch {
		case m.Direction == "in":
			if leads[i] == nil {
				leads[i] = map[int64]bool{}
			}
			leads[i][m.ContactID] = true
		case m.Kind == "voice" && (m.Source == "ai" || m.Source == "seed"):
			days[i].Voice++
		}
	}
	for i := range days {
		days[i].Leads = len(leads[i])
	}
	contacts, err := a.store.Contacts()
	if err != nil {
		return nil, err
	}
	for _, c := range contacts {
		if i, ok := day(c.LinkSentAt); ok && c.LinkSentAt > 0 {
			days[i].Links++
		}
	}

	// Demo numbers: the first week ramps up towards the second, and about a
	// third to a half of the leads who get the link join.
	var avg funnelDay
	for _, d := range days[7:] {
		avg.Leads += d.Leads
		avg.Voice += d.Voice
		avg.Links += d.Links
	}
	for i := range days {
		if i < 7 {
			ramp := 0.45 + 0.05*float64(i) + 0.15*jitter(days[i].Date, "ramp")
			days[i].Leads = int(math.Round(float64(avg.Leads) / 7 * ramp))
			days[i].Voice = int(math.Round(float64(avg.Voice) / 7 * ramp))
			days[i].Links = int(math.Round(float64(avg.Links) / 7 * ramp))
		}
		days[i].Purchased = int(math.Round(float64(days[i].Links) * (0.32 + 0.18*jitter(days[i].Date, "buy"))))
	}
	return days, nil
}

// jitter is a stable pseudo-random number in [0, 1) for a key.
func jitter(parts ...string) float64 {
	h := fnv.New32a()
	for _, p := range parts {
		h.Write([]byte(p))
	}
	return float64(h.Sum32()%1000) / 1000
}

// elevenUsage caches ElevenLabs' numbers for a minute, so reloading the page
// doesn't hit their API every time.
var elevenUsage struct {
	sync.Mutex
	at    time.Time
	times []time.Time
	used  []float64
	sub   voice.Subscription
	err   error
}

func (a *api) spend() ([]spendHour, map[string]any) {
	now := time.Now()
	start := now.Truncate(time.Hour).Add(-(spendHours - 1) * time.Hour)
	hours := make([]spendHour, spendHours)
	for i := range hours {
		hours[i].T = start.Add(time.Duration(i) * time.Hour).UnixMilli()
	}
	bucket := func(t time.Time) (int, bool) {
		i := int(t.Truncate(time.Hour).Sub(start) / time.Hour)
		return i, i >= 0 && i < spendHours
	}

	eleven := map[string]any{}
	u := &elevenUsage
	u.Lock()
	ttl := time.Minute
	if u.err != nil {
		ttl = 10 * time.Second // retry soon after a hiccup
	}
	if time.Since(u.at) > ttl {
		// Not the request's context: a visitor leaving mustn't cache a cancelled fetch.
		c, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		u.times, u.used, u.err = a.core.Voice.Usage(c, start, now, "hour")
		if u.err == nil {
			u.sub, u.err = a.core.Voice.Subscription(c)
		}
		cancel()
		u.at = time.Now()
	}
	if u.err != nil {
		eleven["error"] = u.err.Error()
	} else {
		rate := u.sub.USDPerCredit()
		var credits float64
		for k, t := range u.times {
			if i, ok := bucket(t); ok {
				hours[i].Eleven = u.used[k] * rate
				credits += u.used[k]
			}
		}
		eleven["credits_window"] = credits
		eleven["usd_window"] = credits * rate
		eleven["credits_used"] = u.sub.CharacterCount
		eleven["credits_limit"] = u.sub.CharacterLimit
		eleven["usd_per_credit"] = rate
		eleven["plan"] = u.sub.Tier
	}
	u.Unlock()

	since := a.usageSince()
	claude := map[string]any{"tracked_since": since}
	calls, err := a.store.APICalls("anthropic", 0)
	if err != nil {
		claude["error"] = err.Error()
		return hours, map[string]any{"elevenlabs": eleven, "claude": claude}
	}
	var total, window, perCall float64
	for _, c := range calls {
		total += c.CostUSD
		if i, ok := bucket(time.UnixMilli(c.CreatedAt)); ok {
			hours[i].Claude += c.CostUSD
			window += c.CostUSD
		}
	}
	if len(calls) > 0 {
		perCall = total / float64(len(calls))
	}
	// Replies from before tracking existed weren't priced. Count them, and
	// once a call has been measured, estimate each at the average.
	var earlier int
	var estimated float64
	if runs, err := a.store.Runs(1000); err == nil {
		for _, r := range runs {
			if r.ThoughtAt == 0 || r.ThoughtAt >= since {
				continue
			}
			earlier++
			if perCall > 0 {
				estimated += perCall
				if i, ok := bucket(time.UnixMilli(r.ThoughtAt)); ok {
					hours[i].Claude += perCall
					window += perCall
				}
			}
		}
	}
	claude["calls"] = len(calls)
	claude["usd_per_call"] = perCall
	claude["usd_total"] = total + estimated
	claude["usd_window"] = window
	claude["earlier_replies"] = earlier
	claude["usd_estimated"] = estimated
	return hours, map[string]any{"elevenlabs": eleven, "claude": claude}
}

// usageSince is when spend tracking began (Unix ms).
func (a *api) usageSince() int64 {
	if v, ok, _ := a.store.Setting("usage_since"); ok {
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
			return ms
		}
	}
	return a.store.FirstAPICall("anthropic")
}

// responseTime is how long real replies took, message in to voice note out.
func (a *api) responseTime() map[string]any {
	runs, err := a.store.Runs(200)
	if err != nil {
		return map[string]any{}
	}
	var sum, n int64
	for _, r := range runs {
		if r.Status != "failed" && r.SentAt > r.ReceivedAt && r.ReceivedAt > 0 {
			sum += r.SentAt - r.ReceivedAt
			n++
		}
	}
	if n == 0 {
		return map[string]any{"replies": 0}
	}
	return map[string]any{"replies": n, "avg_ms": sum / n}
}
