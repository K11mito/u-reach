package voice

import (
	"context"
	"path/filepath"
	"testing"
)

// Exercises the local ffmpeg path only; never calls ElevenLabs.
func TestPrepareVoiceNote(t *testing.T) {
	ctx := context.Background()
	in := filepath.Join(t.TempDir(), "tone.ogg")
	if err := Tone(ctx, in, 4); err != nil {
		t.Fatal(err)
	}
	vn, err := PrepareVoiceNote(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if vn.Seconds != 4 || len(vn.Waveform) != 64 || len(vn.Ogg) == 0 {
		t.Fatalf("seconds=%d waveform=%d bytes=%d", vn.Seconds, len(vn.Waveform), len(vn.Ogg))
	}
	t.Logf("bytes=%d seconds=%d dur=%.2f waveform=%v", len(vn.Ogg), vn.Seconds, vn.Duration, vn.Waveform)
}

func TestSpeakable(t *testing.T) {
	got := Speakable("Join U.Center here: https://u.center/join?ref=john  it's U-Reach!")
	if got != "Join U Center here: it's U Reach!" {
		t.Fatalf("got %q", got)
	}
}

// TTS_MOCK path: renders a local tone and never needs an API key.
func TestTTSMock(t *testing.T) {
	e := &Eleven{Mock: true, DataDir: t.TempDir()}
	p, cached, err := e.TTS(context.Background(), "voice", "model", "one two three four five", DefaultSettings)
	if err != nil || cached {
		t.Fatal(err, cached)
	}
	if _, cached, _ = e.TTS(context.Background(), "voice", "model", "one two three four five", DefaultSettings); !cached {
		t.Fatal("second call should hit the cache")
	}
	t.Log(p)
}
