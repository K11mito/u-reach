// Command clone-voice is Spike 2: make an Instant Voice Clone from clips and
// render one fixed line with it, so clones can be compared by ear.
//
//	go run ./cmd/clone-voice data/voice/clips/a.mp3                  # raw clip
//	go run ./cmd/clone-voice -isolate data/voice/clips/a.mp3         # Voice Isolator first (~1k credits/min)
//	go run ./cmd/clone-voice -voice <id> -models eleven_flash_v2_5,eleven_v3   # re-render, no new clone
//
// Outputs go to data/clones/<label>__<model>.{mp3,ogg}; data/clones/clones.tsv
// maps labels to voice IDs and clips, so listeners can judge by label, blind.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ureach/internal/config"
	"ureach/internal/voice"
)

const defaultLine = "Hey Raj, it's John! So good to hear from you. So you asked about U.Center. " +
	"Look, the short version: it's a community, and honestly the best way to get it is a quick call together. " +
	"When works for you this week?"

func main() {
	config.Load(".env")
	label := flag.String("label", "", "name for this clone's outputs (default v1, v2, ...)")
	isolate := flag.Bool("isolate", false, "run Voice Isolator on each clip before cloning")
	removeNoise := flag.Bool("remove-noise", false, "ask ElevenLabs to denoise during cloning")
	voiceID := flag.String("voice", "", "skip cloning; render with this existing voice_id")
	models := flag.String("models", "eleven_multilingual_v2", "comma-separated TTS models to render the line with")
	line := flag.String("line", defaultLine, "the sentence to render")
	out := flag.String("out", "data/clones", "output directory")
	flag.Parse()

	ctx := context.Background()
	el := voice.New()
	el.Mock = false // this spike exists to spend a few credits deliberately

	before, err := el.Subscription(ctx)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("ElevenLabs %s: %d credits left (%d/%d used), voices %d/%d, IVC allowed: %v\n",
		before.Tier, before.Credits(), before.CharacterCount, before.CharacterLimit,
		before.VoiceSlotsUsed, before.VoiceLimit, before.CanUseIVC)

	os.MkdirAll(*out, 0o755)
	if *label == "" {
		*label = nextLabel(*out)
	}
	clips := flag.Args()

	if *voiceID == "" {
		if len(clips) == 0 {
			fatal(fmt.Errorf("give clip paths, or -voice <id> to re-render"))
		}
		if !before.CanUseIVC {
			fatal(fmt.Errorf("this ElevenLabs plan can't use Instant Voice Cloning"))
		}
		if before.VoiceLimit > 0 && before.VoiceSlotsUsed >= before.VoiceLimit {
			fatal(fmt.Errorf("all %d voice slots are used; delete a test clone in the ElevenLabs UI", before.VoiceLimit))
		}
		files := clips
		if *isolate {
			files = nil
			for _, c := range clips {
				iso, err := el.Isolate(ctx, c)
				if err != nil {
					fatal(err)
				}
				fmt.Printf("isolated %s -> %s\n", c, iso)
				files = append(files, iso)
			}
		}
		id, err := el.CloneIVC(ctx, "John (U-Reach "+*label+")", "U-Reach test clone from "+strings.Join(clips, ", "), files, *removeNoise)
		if err != nil {
			fatal(err)
		}
		*voiceID = id
		fmt.Printf("cloned %s: voice_id %s\n", *label, id)
	}

	for _, m := range strings.Split(*models, ",") {
		m = strings.TrimSpace(m)
		mp3, cached, err := el.TTS(ctx, *voiceID, m, *line, voice.DefaultSettings)
		if err != nil {
			fmt.Printf("  %s: %v\n", m, err)
			continue
		}
		base := filepath.Join(*out, *label+"__"+m)
		copyFile(mp3, base+".mp3")
		if err := voice.ToOggOpus(ctx, mp3, base+".ogg"); err != nil {
			fatal(err)
		}
		dur, _ := voice.Duration(ctx, base+".mp3")
		fmt.Printf("  %s: %s.{mp3,ogg} (%.1fs%s)\n", m, base, dur, map[bool]string{true: ", cached"}[cached])
	}

	appendTSV(filepath.Join(*out, "clones.tsv"), *label, *voiceID, *isolate, clips)

	if after, err := el.Subscription(ctx); err == nil {
		fmt.Printf("credits used by this run: %d (%d left)\n", after.CharacterCount-before.CharacterCount, after.Credits())
	}
}

func nextLabel(dir string) string {
	for i := 1; ; i++ {
		l := fmt.Sprintf("v%d", i)
		if m, _ := filepath.Glob(filepath.Join(dir, l+"__*")); len(m) == 0 {
			return l
		}
	}
}

func appendTSV(path, label, voiceID string, isolated bool, clips []string) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s\t%s\tisolated=%v\t%s\t%s\n", label, voiceID, isolated, strings.Join(clips, ","), time.Now().Format(time.RFC3339))
}

func copyFile(src, dst string) {
	if data, err := os.ReadFile(src); err == nil {
		os.WriteFile(dst, data, 0o644)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
