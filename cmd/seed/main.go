// Command seed loads fake leads and past conversations from
// data/seed-leads.json so the dashboard looks lived-in. It replaces all app
// data (never the WhatsApp session).
//
//	go run ./cmd/seed                  # text only, no ElevenLabs credits
//	go run ./cmd/seed -render-audio    # also render the seeded voice notes once (asks first)
//	go run ./cmd/seed -add             # only add leads that are missing, keeping live chats
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"ureach/internal/config"
	"ureach/internal/seed"
	"ureach/internal/store"
	"ureach/internal/voice"
)

func main() {
	config.Load(".env")
	file := flag.String("file", "data/seed-leads.json", "seed data")
	dbPath := flag.String("db", "data/ureach.db", "app database")
	render := flag.Bool("render-audio", false, "render seeded AI voice notes with ElevenLabs (cached; asks first)")
	yes := flag.Bool("yes", false, "don't ask before wiping live data or spending credits")
	add := flag.Bool("add", false, "only add seeded leads that are missing; keeps live chats")
	flag.Parse()

	st, err := store.Open(*dbPath)
	must(err)
	defer st.Close()

	referral := config.Get("REFERRAL_URL", "https://example.com/join/john")
	var res seed.Result
	if *add {
		res, err = seed.Add(st, *file, referral)
	} else {
		if live := seed.LiveMessages(st); live > 0 && !*yes {
			confirm(fmt.Sprintf("%s has %d live (non-seed) messages. Wipe them and reseed?", *dbPath, live))
		}
		res, err = seed.Load(st, *file, referral)
	}
	must(err)
	fmt.Printf("seeded %d leads, %d messages, %d AI voice notes\n", res.Leads, res.Messages, len(res.VoiceNotes))

	if !*render {
		fmt.Println("voice notes have transcripts but no audio; run with -render-audio once the voice is chosen")
		return
	}
	voiceID := config.Must("JOHN_VOICE_ID")
	model := config.Get("TTS_MODEL", "eleven_flash_v2_5")
	chars := seed.Characters(res.VoiceNotes)
	rate := 1.0
	if strings.Contains(model, "flash") || strings.Contains(model, "turbo") {
		rate = 0.5
	}
	if !*yes {
		confirm(fmt.Sprintf("Render %d voice notes (%d characters, ~%.0f credits on %s; cached ones are free)?", len(res.VoiceNotes), chars, float64(chars)*rate, model))
	}
	must(seed.RenderAudio(context.Background(), st, voice.New(), voiceID, model, res.VoiceNotes, func(v seed.VoiceNote, dur float64, cached bool) {
		fmt.Printf("  note %d: %.1fs cached=%v\n", v.MessageID, dur, cached)
	}))
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
