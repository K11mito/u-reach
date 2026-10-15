// Command seed loads fake leads and past conversations from
// data/seed-leads.json so the dashboard looks lived-in. It replaces all app
// data (never the WhatsApp session).
//
//	go run ./cmd/seed                  # text only, no ElevenLabs credits
//	go run ./cmd/seed -render-audio    # also render the seeded voice notes once (asks first)
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"ureach/internal/config"
	"ureach/internal/store"
	"ureach/internal/voice"
)

type seedMsg struct {
	Ago  string `json:"ago"`  // Go duration before now, e.g. "26h"
	From string `json:"from"` // lead | ai | john
	Kind string `json:"kind"` // text (default) | voice
	Text string `json:"text"`
}

type seedLead struct {
	Name          string    `json:"name"`
	Phone         string    `json:"phone"`
	Context       string    `json:"context"`
	Status        string    `json:"status"`
	HandoffReason string    `json:"handoff_reason"`
	Memory        []string  `json:"memory"`
	Messages      []seedMsg `json:"messages"`
}

func main() {
	config.Load(".env")
	file := flag.String("file", "data/seed-leads.json", "seed data")
	dbPath := flag.String("db", "data/ureach.db", "app database")
	render := flag.Bool("render-audio", false, "render seeded AI voice notes with ElevenLabs (cached; asks first)")
	yes := flag.Bool("yes", false, "don't ask before wiping live data or spending credits")
	flag.Parse()

	raw, err := os.ReadFile(*file)
	must(err)
	var leads []seedLead
	must(json.Unmarshal(raw, &leads))

	st, err := store.Open(*dbPath)
	must(err)
	defer st.Close()

	if live := liveMessages(st); live > 0 && !*yes {
		confirm(fmt.Sprintf("%s has %d live (non-seed) messages. Wipe them and reseed?", *dbPath, live))
	}
	must(st.Reset())

	referral := config.Get("REFERRAL_URL", "https://example.com/join/john")
	now := time.Now()
	type voiceRef struct {
		msgID int64
		text  string
	}
	var voiceNotes []voiceRef
	for _, l := range leads {
		jid := l.Phone + "@s.whatsapp.net"
		c := store.Contact{JID: jid, Chat: jid, Phone: l.Phone, Name: l.Name, Context: l.Context,
			Memory: l.Memory, Status: l.Status, HandoffReason: l.HandoffReason, CreatedAt: now.Add(-7 * 24 * time.Hour).UnixMilli()}
		for _, m := range l.Messages {
			if strings.Contains(m.Text, "{REFERRAL_URL}") {
				c.LinkSentAt = at(now, m.Ago)
			}
		}
		created, err := st.CreateContact(c)
		must(err)
		for _, m := range l.Messages {
			dir, source := "out", "seed"
			switch m.From {
			case "lead":
				dir, source = "in", "lead"
			case "john":
				source = "john"
			}
			kind := m.Kind
			if kind == "" {
				kind = "text"
			}
			msg, err := st.AddMessage(store.Message{
				ContactID: created.ID, Direction: dir, Kind: kind, Source: source,
				Text: strings.ReplaceAll(m.Text, "{REFERRAL_URL}", referral), CreatedAt: at(now, m.Ago),
			})
			must(err)
			if kind == "voice" && source == "seed" {
				voiceNotes = append(voiceNotes, voiceRef{msg.ID, msg.Text})
			}
		}
		fmt.Printf("  %-8s %-11s %d messages\n", l.Name, l.Status, len(l.Messages))
	}
	fmt.Printf("seeded %d leads, %d AI voice notes\n", len(leads), len(voiceNotes))

	if !*render {
		fmt.Println("voice notes have transcripts but no audio; run with -render-audio once the voice is chosen")
		return
	}
	voiceID := config.Must("JOHN_VOICE_ID")
	model := config.Get("TTS_MODEL", "eleven_flash_v2_5")
	chars := 0
	for _, v := range voiceNotes {
		chars += len(voice.Speakable(v.text))
	}
	rate := 1.0
	if strings.Contains(model, "flash") || strings.Contains(model, "turbo") {
		rate = 0.5
	}
	if !*yes {
		confirm(fmt.Sprintf("Render %d voice notes (%d characters, ~%.0f credits on %s; cached ones are free)?", len(voiceNotes), chars, float64(chars)*rate, model))
	}
	el := voice.New()
	ctx := context.Background()
	for _, v := range voiceNotes {
		mp3, cached, err := el.TTS(ctx, voiceID, model, v.text, voice.DefaultSettings)
		if err != nil {
			fmt.Println("  tts:", err)
			continue
		}
		dur, _ := voice.Duration(ctx, mp3)
		rel := strings.TrimPrefix(mp3, "data/audio/")
		must(st.SetMessageAudio(v.msgID, rel, int64(dur*1000)))
		fmt.Printf("  note %d: %.1fs cached=%v\n", v.msgID, dur, cached)
	}
}

func liveMessages(st *store.Store) int {
	contacts, err := st.Contacts()
	if err != nil {
		return 0
	}
	n := 0
	for _, c := range contacts {
		msgs, _ := st.Messages(c.ID, 1000)
		for _, m := range msgs {
			if m.Source == "ai" || (m.Source == "lead" && m.WAID != "") {
				n++
			}
		}
	}
	return n
}

func at(now time.Time, ago string) int64 {
	d, err := time.ParseDuration(ago)
	if err != nil {
		d = 0
	}
	return now.Add(-d).UnixMilli()
}

func confirm(q string) {
	fmt.Printf("%s [y/N] ", q)
	ans, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(ans)), "y") {
		fmt.Println("aborted")
		os.Exit(1)
	}
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
