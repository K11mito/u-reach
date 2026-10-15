// Command wa-spike is Spike 1 (go/no-go): link the demo number by QR, persist
// the session, echo inbound text, and answer "voice" with a voice-note bubble.
//
//	go run ./cmd/wa-spike                 # QR in the terminal on first run
//	go run ./cmd/wa-spike -pair-phone 66812345678   # pairing code instead of QR
//	go run ./cmd/wa-spike -audio data/clones/x.ogg  # send a different file on "voice"
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	waLog "go.mau.fi/whatsmeow/util/log"

	"ureach/internal/config"
	"ureach/internal/voice"
	"ureach/internal/wa"
)

func main() {
	config.Load(".env")
	audio := flag.String("audio", "data/test.ogg", "file sent when someone texts \"voice\" (any format; converted to Ogg Opus)")
	pairPhone := flag.String("pair-phone", "", "link with a pairing code for this number instead of a QR")
	session := flag.String("session", config.Get("WA_SESSION", "data/wa-session.db"), "whatsmeow session DB")
	level := flag.String("log", "INFO", "whatsmeow log level (DEBUG for protocol detail)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.MkdirAll(filepath.Dir(*session), 0o755)
	os.MkdirAll("data/audio/inbound", 0o755)
	if _, err := os.Stat(*audio); os.IsNotExist(err) && *audio == "data/test.ogg" {
		if err := voice.Tone(ctx, *audio, 4); err != nil {
			fatal(err)
		}
		fmt.Println("generated test tone at", *audio)
	}

	log := waLog.Stdout("WA", *level, true)
	var allow []string
	if a := config.Get("ALLOWLIST", ""); a != "" {
		allow = strings.Split(a, ",")
	}
	c, err := wa.Open(ctx, *session, allow, log)
	if err != nil {
		fatal(err)
	}
	defer c.Close()

	c.OnMessage = func(in wa.Inbound) {
		ctx := context.Background()
		fmt.Printf("\n<- %s  chat=%s sender=%s from=%s phone=%q push=%q\n",
			in.ID, in.Chat, in.Sender, in.From, in.Phone, in.PushName)
		c.MarkRead(ctx, in)

		switch {
		case in.Audio != nil:
			data, err := c.DownloadAudio(ctx, in.Audio)
			if err != nil {
				fmt.Println("   download failed:", err)
				return
			}
			path := filepath.Join("data/audio/inbound", string(in.ID)+".ogg")
			os.WriteFile(path, data, 0o644)
			fmt.Printf("   voice note: %ds ptt=%v mime=%q saved %s\n",
				in.Audio.GetSeconds(), in.Audio.GetPTT(), in.Audio.GetMimetype(), path)
			reply(c, in, fmt.Sprintf("got your voice note (%ds)", in.Audio.GetSeconds()))

		case strings.EqualFold(strings.TrimSpace(in.Text), "voice"):
			t0 := time.Now()
			stopRec := c.Recording(in.Chat)
			time.Sleep(3 * time.Second) // long enough to see "recording audio..."
			vn, err := voice.PrepareVoiceNote(ctx, *audio)
			if err != nil {
				stopRec()
				fmt.Println("   prepare voice note:", err)
				return
			}
			id, err := c.SendVoiceNote(ctx, in.Chat, vn.Ogg, vn.Seconds, vn.Waveform)
			stopRec()
			if err != nil {
				fmt.Println("   send voice note:", err)
				return
			}
			fmt.Printf("-> voice note %s (%.1fs, %d bytes) in %s\n", id, vn.Duration, len(vn.Ogg), time.Since(t0).Round(time.Millisecond))

		default:
			fmt.Printf("   text: %q\n", in.Text)
			reply(c, in, "echo: "+in.Text)
		}
	}

	if err := c.Connect(ctx, *pairPhone); err != nil {
		fatal(err)
	}
	fmt.Println("\nReady. From another phone, text the demo number: anything -> echo, \"voice\" -> voice note, or send a voice note. Ctrl-C to quit.")
	<-ctx.Done()
}

func reply(c *wa.Client, in wa.Inbound, text string) {
	if id, err := c.SendText(context.Background(), in.Chat, text); err != nil {
		fmt.Println("   send text:", err)
	} else {
		fmt.Printf("-> text %s: %q\n", id, text)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
