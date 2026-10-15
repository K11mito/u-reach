package voice

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// VoiceNote is audio ready to send as a WhatsApp push-to-talk bubble.
type VoiceNote struct {
	Ogg      []byte  // Ogg Opus, mono, 48 kHz
	Seconds  uint32  // shown on the bubble; 0 renders as 0:00 on some clients
	Waveform []byte  // 64 values 0-100; without it the bubble shows a flat line
	Duration float64 // exact length in seconds
}

// PrepareVoiceNote converts any audio file to a WhatsApp voice note.
func PrepareVoiceNote(ctx context.Context, inPath string) (VoiceNote, error) {
	tmp, err := os.CreateTemp("", "ureach-*.ogg")
	if err != nil {
		return VoiceNote{}, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	if err := ToOggOpus(ctx, inPath, tmp.Name()); err != nil {
		return VoiceNote{}, err
	}
	ogg, err := os.ReadFile(tmp.Name())
	if err != nil {
		return VoiceNote{}, err
	}
	dur, err := Duration(ctx, tmp.Name())
	if err != nil {
		return VoiceNote{}, err
	}
	wf, err := Waveform(ctx, tmp.Name())
	if err != nil {
		return VoiceNote{}, err
	}
	// Round, not ceil: Opus padding makes a 4.0 s clip probe as ~4.01 s.
	secs := uint32(max(1, math.Round(dur)))
	return VoiceNote{Ogg: ogg, Seconds: secs, Waveform: wf, Duration: dur}, nil
}

// ToOggOpus transcodes to the format WhatsApp expects for voice notes.
func ToOggOpus(ctx context.Context, inPath, outPath string) error {
	return ffmpeg(ctx, "-i", inPath, "-vn", "-map_metadata", "-1",
		"-c:a", "libopus", "-b:a", "48k", "-ac", "1", "-ar", "48000", "-application", "voip",
		"-f", "ogg", outPath)
}

// ToMP3 transcodes for browser playback (Safari can't always play Ogg Opus).
func ToMP3(ctx context.Context, inPath, outPath string) error {
	return ffmpeg(ctx, "-i", inPath, "-vn", "-c:a", "libmp3lame", "-q:a", "4", outPath)
}

// Cut copies [start, start+dur) of inPath to outPath as high-quality mono mp3.
func Cut(ctx context.Context, inPath, outPath string, start, dur time.Duration) error {
	return ffmpeg(ctx, "-ss", fmt.Sprintf("%.3f", start.Seconds()), "-t", fmt.Sprintf("%.3f", dur.Seconds()),
		"-i", inPath, "-vn", "-ac", "1", "-c:a", "libmp3lame", "-b:a", "192k", outPath)
}

// Tone writes a pulsing test tone (so the waveform visibly moves) as Ogg Opus.
func Tone(ctx context.Context, outPath string, seconds float64) error {
	src := fmt.Sprintf("aevalsrc=0.6*sin(2*PI*220*t)*abs(sin(2*PI*1.3*t)):s=48000:d=%.1f", seconds)
	return ffmpeg(ctx, "-f", "lavfi", "-i", src, "-c:a", "libopus", "-b:a", "48k", "-ac", "1", "-ar", "48000", "-f", "ogg", outPath)
}

// Duration returns the audio length in seconds.
func Duration(ctx context.Context, path string) (float64, error) {
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", path).Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe %s: %w", filepath.Base(path), err)
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}

// Waveform returns 64 loudness buckets scaled 0-100, the shape WhatsApp draws.
func Waveform(ctx context.Context, path string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-i", path, "-ac", "1", "-ar", "8000", "-f", "s16le", "pipe:1")
	raw, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg decode for waveform: %w", err)
	}
	samples := make([]int16, len(raw)/2)
	if err := binary.Read(bytes.NewReader(raw), binary.LittleEndian, samples); err != nil {
		return nil, err
	}
	const n = 64
	rms := make([]float64, n)
	var peak float64
	for i := range n {
		lo, hi := i*len(samples)/n, (i+1)*len(samples)/n
		var sum float64
		for _, s := range samples[lo:hi] {
			sum += float64(s) * float64(s)
		}
		if hi > lo {
			rms[i] = math.Sqrt(sum / float64(hi-lo))
		}
		peak = max(peak, rms[i])
	}
	wf := make([]byte, n)
	for i, v := range rms {
		if peak > 0 {
			wf[i] = byte(math.Round(v / peak * 100))
		}
	}
	return wf, nil
}

func ffmpeg(ctx context.Context, args ...string) error {
	args = append([]string{"-y", "-v", "error"}, args...)
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
