// Package voice talks to ElevenLabs (TTS, Instant Voice Clone, Voice Isolator,
// Scribe) and prepares audio with ffmpeg.
//
// Credits are finite: every TTS result is cached on disk, TTS_MOCK=1 swaps in a
// local tone, and a per-process character ceiling stops runaway loops.
package voice

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"ureach/internal/config"
)

const (
	apiBase      = "https://api.elevenlabs.io"
	outputFormat = "mp3_44100_128"
)

// Settings are ElevenLabs voice settings. They are part of the TTS cache key.
type Settings struct {
	Stability       float64 `json:"stability"`
	SimilarityBoost float64 `json:"similarity_boost"`
	Style           float64 `json:"style"`
	SpeakerBoost    bool    `json:"use_speaker_boost"`
	Speed           float64 `json:"speed"`
}

// DefaultSettings are ElevenLabs' defaults.
var DefaultSettings = Settings{Stability: 0.5, SimilarityBoost: 0.75, SpeakerBoost: true, Speed: 1.0}

// Eleven is an ElevenLabs client with an on-disk TTS cache.
type Eleven struct {
	Key      string
	Mock     bool   // TTS_MOCK=1: never call TTS, render a tone instead
	DataDir  string // cache root, default data/audio
	MaxChars int    // TTS characters this process may spend (0 = no limit)

	http *http.Client
	mu   sync.Mutex
	used int
}

// New builds a client from the environment (call config.Load first).
func New() *Eleven {
	return &Eleven{
		Key:      os.Getenv("ELEVENLABS_API_KEY"),
		Mock:     config.Get("TTS_MOCK", "0") == "1",
		DataDir:  "data/audio",
		MaxChars: 20000,
		http:     &http.Client{Timeout: 3 * time.Minute},
	}
}

// TTS renders text in voiceID and returns the path of an mp3. Identical
// requests are served from the cache and cost nothing.
func (e *Eleven) TTS(ctx context.Context, voiceID, model, text string, s Settings) (path string, cached bool, err error) {
	text = Speakable(text)
	if strings.TrimSpace(text) == "" {
		return "", false, fmt.Errorf("tts: empty text")
	}
	settings, _ := json.Marshal(s)
	sum := sha256.Sum256([]byte(strings.Join([]string{voiceID, model, outputFormat, string(settings), text}, "\x00")))
	key := hex.EncodeToString(sum[:])

	if e.Mock {
		path = filepath.Join(e.DataDir, "tts-mock", key+".mp3")
		if exists(path) {
			return path, true, nil
		}
		os.MkdirAll(filepath.Dir(path), 0o755)
		secs := max(2, float64(len(strings.Fields(text)))/2.5) // ~150 wpm
		src := fmt.Sprintf("aevalsrc=0.5*sin(2*PI*180*t)*abs(sin(2*PI*1.7*t)):s=44100:d=%.1f", secs)
		return path, false, ffmpeg(ctx, "-f", "lavfi", "-i", src, "-c:a", "libmp3lame", "-q:a", "6", path)
	}

	path = filepath.Join(e.DataDir, "tts", key+".mp3")
	if exists(path) {
		return path, true, nil
	}
	if err := e.spend(len(text)); err != nil {
		return "", false, err
	}

	body, _ := json.Marshal(map[string]any{"text": text, "model_id": model, "voice_settings": s})
	u := fmt.Sprintf("%s/v1/text-to-speech/%s?output_format=%s", apiBase, url.PathEscape(voiceID), outputFormat)
	audio, err := e.do(ctx, http.MethodPost, u, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", false, fmt.Errorf("tts: %w", err)
	}
	return path, false, writeAtomic(path, audio)
}

func (e *Eleven) spend(n int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.MaxChars > 0 && e.used+n > e.MaxChars {
		return fmt.Errorf("tts: per-process ceiling of %d characters reached (runaway loop?)", e.MaxChars)
	}
	e.used += n
	return nil
}

// Isolate runs Voice Isolator on a file (~1000 credits per minute of audio).
// Results are cached by input content, so re-running a spike is free.
func (e *Eleven) Isolate(ctx context.Context, inPath string) (string, error) {
	data, err := os.ReadFile(inPath)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	path := filepath.Join(e.DataDir, "isolated", hex.EncodeToString(sum[:])+".mp3")
	if exists(path) {
		return path, nil
	}
	body, ctype, err := multipartBody(nil, map[string][]string{"audio": {inPath}})
	if err != nil {
		return "", err
	}
	audio, err := e.do(ctx, http.MethodPost, apiBase+"/v1/audio-isolation", ctype, body)
	if err != nil {
		return "", fmt.Errorf("isolate %s: %w", filepath.Base(inPath), err)
	}
	return path, writeAtomic(path, audio)
}

// CloneIVC creates an Instant Voice Clone from audio files and returns its voice_id.
func (e *Eleven) CloneIVC(ctx context.Context, name, description string, files []string, removeNoise bool) (string, error) {
	fields := map[string]string{
		"name":                    name,
		"description":             description,
		"remove_background_noise": fmt.Sprint(removeNoise),
	}
	body, ctype, err := multipartBody(fields, map[string][]string{"files": files})
	if err != nil {
		return "", err
	}
	raw, err := e.do(ctx, http.MethodPost, apiBase+"/v1/voices/add", ctype, body)
	if err != nil {
		return "", fmt.Errorf("clone: %w", err)
	}
	var out struct {
		VoiceID              string `json:"voice_id"`
		RequiresVerification bool   `json:"requires_verification"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if out.RequiresVerification {
		return out.VoiceID, fmt.Errorf("voice %s requires verification in the ElevenLabs UI before use", out.VoiceID)
	}
	return out.VoiceID, nil
}

// Transcription is Scribe's result.
type Transcription struct {
	Text         string `json:"text"`
	LanguageCode string `json:"language_code"`
}

// Transcribe runs Scribe speech-to-text on an audio file.
func (e *Eleven) Transcribe(ctx context.Context, path string) (Transcription, error) {
	fields := map[string]string{
		"model_id":               config.Get("STT_MODEL", "scribe_v2"),
		"tag_audio_events":       "false",
		"timestamps_granularity": "none",
	}
	body, ctype, err := multipartBody(fields, map[string][]string{"file": {path}})
	if err != nil {
		return Transcription{}, err
	}
	raw, err := e.do(ctx, http.MethodPost, apiBase+"/v1/speech-to-text", ctype, body)
	if err != nil {
		return Transcription{}, fmt.Errorf("transcribe: %w", err)
	}
	var t Transcription
	return t, json.Unmarshal(raw, &t)
}

// Subscription is the part of /v1/user/subscription we track.
type Subscription struct {
	Tier           string `json:"tier"`
	CharacterCount int    `json:"character_count"`
	CharacterLimit int    `json:"character_limit"`
	VoiceSlotsUsed int    `json:"voice_slots_used"`
	VoiceLimit     int    `json:"voice_limit"`
	CanUseIVC      bool   `json:"can_use_instant_voice_cloning"`
	NextResetUnix  int64  `json:"next_character_count_reset_unix"`
}

// Credits left this period.
func (s Subscription) Credits() int { return s.CharacterLimit - s.CharacterCount }

// Subscription reports credit and voice-slot usage.
func (e *Eleven) Subscription(ctx context.Context) (Subscription, error) {
	raw, err := e.do(ctx, http.MethodGet, apiBase+"/v1/user/subscription", "", nil)
	if err != nil {
		return Subscription{}, fmt.Errorf("subscription: %w", err)
	}
	var s Subscription
	return s, json.Unmarshal(raw, &s)
}

func (e *Eleven) do(ctx context.Context, method, u, ctype string, body io.Reader) ([]byte, error) {
	if e.Key == "" {
		return nil, fmt.Errorf("ELEVENLABS_API_KEY is not set (add it to .env)")
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("xi-api-key", e.Key)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		msg := string(data)
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return nil, fmt.Errorf("%s %s: %s", method, resp.Status, msg)
	}
	return data, nil
}

func multipartBody(fields map[string]string, files map[string][]string) (io.Reader, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		w.WriteField(k, v)
	}
	for field, paths := range files {
		for _, p := range paths {
			data, err := os.ReadFile(p)
			if err != nil {
				return nil, "", err
			}
			fw, err := w.CreateFormFile(field, filepath.Base(p))
			if err != nil {
				return nil, "", err
			}
			fw.Write(data)
		}
	}
	return &buf, w.FormDataContentType(), w.Close()
}

var (
	urlRe   = regexp.MustCompile(`https?://\S+|www\.\S+`)
	spaceRe = regexp.MustCompile(`\s+`)
	sayAs   = strings.NewReplacer(
		"U.Center", "U Center", "U.center", "U Center", "u.center", "U Center",
		"U-Reach", "U Reach", "U-reach", "U Reach",
	)
)

// Speakable makes text safe to read aloud: brand names spelled the way they
// are said, and URLs removed (they go in the follow-up text instead).
func Speakable(text string) string {
	text = urlRe.ReplaceAllString(text, "")
	text = sayAs.Replace(text)
	return strings.TrimSpace(spaceRe.ReplaceAllString(text, " "))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
