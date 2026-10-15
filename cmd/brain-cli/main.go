// Command brain-cli prints the brain's reply for a sample message, so the
// prompt, persona and FAQ can be tuned without WhatsApp or TTS.
//
//	go run ./cmd/brain-cli -name Raj "hey John, saw your post about U.Center, what is it?"
//	go run ./cmd/brain-cli -samples data/brain-samples.txt
//	go run ./cmd/brain-cli -models claude-opus-5-5,claude-sonnet-5-5,claude-haiku-5-5 -name Raj "what is it?"
//	go run ./cmd/brain-cli -speak -name Raj "what is it?"   # one TTS render (costs credits unless TTS_MOCK=1)
//
// Several message args are treated as one burst ("hi" "saw your post" "what is it?").
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ureach/internal/brain"
	"ureach/internal/config"
	"ureach/internal/voice"
)

func main() {
	config.Load(".env")
	name := flag.String("name", "", "lead's first name")
	contactCtx := flag.String("context", "", "how John knows the lead")
	linkSent := flag.Bool("link-sent", false, "pretend the referral link was already sent")
	samples := flag.String("samples", "", "file of `Name|message` lines to run (LLM only, never TTS)")
	models := flag.String("models", "", "comma-separated models to compare (default BRAIN_MODEL)")
	effort := flag.String("effort", "", "effort level (default BRAIN_EFFORT or low)")
	persona := flag.String("persona", "data/persona/john.json", "persona card")
	faq := flag.String("faq", "data/faq.md", "FAQ")
	speak := flag.Bool("speak", false, "render the reply with TTS once (single message, single model only)")
	flag.Parse()

	b, err := brain.New(*persona, *faq)
	if err != nil {
		fatal(err)
	}
	if *effort != "" {
		b.Effort = *effort
	}
	modelList := []string{b.Model}
	if *models != "" {
		modelList = strings.Split(*models, ",")
	}

	type run struct {
		contact brain.Contact
		history []brain.Turn
	}
	var runs []run
	if *samples != "" {
		lines, err := readSamples(*samples)
		if err != nil {
			fatal(err)
		}
		for _, l := range lines {
			n, msg, _ := strings.Cut(l, "|")
			runs = append(runs, run{brain.Contact{Name: strings.TrimSpace(n)}, []brain.Turn{{FromLead: true, Text: strings.TrimSpace(msg)}}})
		}
	} else {
		if flag.NArg() == 0 {
			fatal(fmt.Errorf("give a message, or -samples <file>"))
		}
		var h []brain.Turn
		for _, m := range flag.Args() {
			h = append(h, brain.Turn{FromLead: true, Text: m})
		}
		runs = append(runs, run{brain.Contact{Name: *name, Context: *contactCtx, LinkSent: *linkSent}, h})
	}

	ctx := context.Background()
	var last brain.Reply
	for _, r := range runs {
		fmt.Printf("\n=== %s: %q\n", or(r.contact.Name, "(no name)"), r.history[len(r.history)-1].Text)
		for _, m := range modelList {
			b.Model = strings.TrimSpace(m)
			reply, meta, err := b.Reply(ctx, r.contact, r.history)
			if err != nil {
				fmt.Printf("--- %s: ERROR %v (after %s)\n", b.Model, err, meta.Latency.Round(time.Millisecond))
				continue
			}
			out, _ := json.MarshalIndent(reply, "", "  ")
			fmt.Printf("--- %s  %s  %d words (~%.0fs)  in=%d (cache read %d, write %d) out=%d%s%s\n%s\n",
				meta.Model, meta.Latency.Round(time.Millisecond), meta.Words, float64(meta.Words)/2.5,
				meta.InputTokens, meta.CacheRead, meta.CacheWrite, meta.OutputTokens,
				flagIf(meta.Trimmed, "  TRIMMED"), flagIf(meta.ForcedHandoff != "", "  FORCED HANDOFF ("+meta.ForcedHandoff+")"),
				out)
			last = reply
		}
	}

	if *speak {
		if len(runs) != 1 || len(modelList) != 1 {
			fatal(fmt.Errorf("-speak renders one reply: use a single message and a single model"))
		}
		speakReply(ctx, last)
	}
}

func speakReply(ctx context.Context, r brain.Reply) {
	el := voice.New()
	voiceID := config.Must("JOHN_VOICE_ID")
	model := config.Get("TTS_MODEL", "eleven_flash_v2_5")
	mp3, cached, err := el.TTS(ctx, voiceID, model, r.SpokenScript, voice.DefaultSettings)
	if err != nil {
		fatal(err)
	}
	out := filepath.Join("data/audio/brain-cli", time.Now().Format("150405"))
	os.MkdirAll(filepath.Dir(out), 0o755)
	if err := voice.ToOggOpus(ctx, mp3, out+".ogg"); err != nil {
		fatal(err)
	}
	if data, err := os.ReadFile(mp3); err == nil {
		os.WriteFile(out+".mp3", data, 0o644)
	}
	fmt.Printf("\nspoken with %s/%s (cached=%v, mock=%v): %s.mp3 / .ogg\n", voiceID, model, cached, el.Mock, out)
}

func readSamples(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l != "" && !strings.HasPrefix(l, "#") {
			lines = append(lines, l)
		}
	}
	return lines, sc.Err()
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func flagIf(cond bool, s string) string {
	if cond {
		return s
	}
	return ""
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
