package core

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"

	"ureach/internal/brain"
	"ureach/internal/store"
)

// Draft is a personal opener waiting for John's approval.
type Draft struct {
	ContactID  int64  `json:"contact_id"`
	Name       string `json:"name"`
	Script     string `json:"script"`
	Followup   string `json:"followup"`
	AudioPath  string `json:"audio_path"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

// DraftOpeners writes and voices an opener per contact, in parallel. The audio
// is cached, so sending an unedited draft costs no extra credits.
func (c *Core) DraftOpeners(ctx context.Context, ids []int64) []Draft {
	out := make([]Draft, len(ids))
	done := make(chan struct{}, len(ids))
	for i, id := range ids {
		go func() {
			defer func() { done <- struct{}{} }()
			out[i] = c.draft(ctx, id)
		}()
	}
	for range ids {
		<-done
	}
	return out
}

func (c *Core) draft(ctx context.Context, id int64) Draft {
	ct, err := c.Store.Contact(id)
	if err != nil {
		return Draft{ContactID: id, Error: err.Error()}
	}
	d := Draft{ContactID: id, Name: ct.DisplayName()}
	r, _, err := c.Brain.Opener(ctx, brain.Contact{Name: firstName(ct.DisplayName()), Context: ct.Context, Memory: ct.Memory})
	if err != nil {
		d.Error = err.Error()
		return d
	}
	d.Script = r.SpokenScript
	d.Followup = strings.TrimSpace(strings.TrimSuffix(r.FollowupText, c.Brain.ReferralURL))
	vn, mp3, err := c.speak(ctx, 0, d.Script)
	if err != nil {
		d.Error = "voice: " + err.Error()
		return d
	}
	d.AudioPath, d.DurationMS = mp3, int64(vn.Duration*1000)
	return d
}

// SendOpeners sends approved drafts one by one with a short random gap, like
// a person working through their list. It returns immediately.
func (c *Core) SendOpeners(drafts []Draft) {
	go func() {
		for i, d := range drafts {
			if i > 0 {
				time.Sleep(time.Duration(4000+rand.IntN(5000)) * time.Millisecond)
			}
			if err := c.sendOpener(context.Background(), d); err != nil {
				log.Printf("campaign: %s: %v", d.Name, err)
				c.Hub.Publish("campaign", map[string]any{"contact_id": d.ContactID, "status": "failed", "error": err.Error()})
				continue
			}
			c.Hub.Publish("campaign", map[string]any{"contact_id": d.ContactID, "status": "sent"})
		}
	}()
}

func (c *Core) sendOpener(ctx context.Context, d Draft) error {
	ct, err := c.Store.Contact(d.ContactID)
	if err != nil {
		return err
	}
	to, err := types.ParseJID(ct.Chat)
	if err != nil {
		return fmt.Errorf("bad chat %q: %w", ct.Chat, err)
	}
	c.Hub.Publish("campaign", map[string]any{"contact_id": d.ContactID, "status": "sending"})

	stopRec := c.Gateway.Recording(to)
	vn, mp3, err := c.speak(ctx, 0, d.Script) // cached unless the script was edited
	if err != nil {
		stopRec()
		return err
	}
	time.Sleep(2 * time.Second) // let "recording audio..." show
	waID, err := c.Gateway.SendVoiceNote(ctx, to, vn)
	stopRec()
	if err != nil {
		return err
	}
	c.addOut(ct.ID, 0, waID, "voice", d.Script, mp3, int64(vn.Duration*1000))

	text := strings.TrimSpace(d.Followup)
	if c.Brain.ReferralURL != "" {
		if text == "" {
			text = "Here's my link:"
		}
		text += "\n" + c.Brain.ReferralURL
	}
	if text != "" {
		time.Sleep(1200 * time.Millisecond)
		if err := c.sendText(ctx, ct.ID, 0, to, false, text); err != nil {
			return err
		}
		c.Store.MarkLinkSent(ct.ID)
	}
	if ct.Status == store.StatusNew {
		c.Store.SetStatus(ct.ID, store.StatusAuto, "")
	}
	c.Store.AddEvent(ct.ID, "opener_sent", nil)
	c.publishContact(ct.ID)
	return nil
}
