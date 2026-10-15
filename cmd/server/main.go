// Command server is the U-Reach app: WhatsApp gateway, reply pipeline, JSON
// API, live events (SSE) and the embedded dashboard, in one binary.
//
//	go run ./cmd/server             # links WhatsApp (QR in terminal on first run)
//	go run ./cmd/server -wa=false   # no WhatsApp: use the dashboard's simulate box
package main

import (
	"context"
	"encoding/json"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"

	"ureach/internal/brain"
	"ureach/internal/config"
	"ureach/internal/core"
	"ureach/internal/store"
	"ureach/internal/voice"
	"ureach/internal/wa"
	"ureach/web"
)

func main() {
	config.Load(".env")
	addr := flag.String("addr", config.Get("ADDR", ":8080"), "HTTP listen address")
	useWA := flag.Bool("wa", true, "connect to WhatsApp (false = dashboard simulate box only)")
	dbPath := flag.String("db", "data/ureach.db", "app database")
	session := flag.String("session", config.Get("WA_SESSION", "data/wa-session.db"), "whatsmeow session DB")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.MkdirAll("data/audio", 0o755)

	st, err := store.Open(*dbPath)
	must(err)
	defer st.Close()
	br, err := brain.New("data/persona/john.json", "data/faq.md")
	must(err)
	el := voice.New()
	voiceID := config.Get("JOHN_VOICE_ID", "")
	if voiceID == "" {
		log.Printf("JOHN_VOICE_ID is not set: voice notes use a mock tone (TTS_MOCK)")
		el.Mock = true
	}
	if el.Mock {
		log.Printf("TTS mock mode: no ElevenLabs credits will be spent")
	}

	hub := core.NewHub()
	c := &core.Core{
		Store: st, Brain: br, Voice: el, Hub: hub, AudioDir: "data/audio",
		VoiceID: voiceID, TTSModel: config.Get("TTS_MODEL", "eleven_flash_v2_5"), Settings: voice.DefaultSettings,
		Debounce:        time.Duration(atoi(config.Get("DEBOUNCE_MS", "3000"))) * time.Millisecond,
		MaxVoicePerHour: atoi(config.Get("MAX_VOICE_PER_HOUR", "10")),
		Gateway:         logGateway{},
	}

	var waClient *wa.Client
	if *useWA {
		var allow []string
		if a := config.Get("ALLOWLIST", ""); a != "" {
			allow = strings.Split(a, ",")
		}
		waClient, err = wa.Open(ctx, *session, allow, waLog.Stdout("WA", "WARN", true))
		must(err)
		defer waClient.Close()
		c.Gateway = waGateway{waClient}
		waClient.OnMessage = func(in wa.Inbound) { c.Handle(fromWA(waClient, in)) }
		go func() {
			if err := waClient.Connect(ctx, ""); err != nil {
				log.Printf("whatsapp: %v", err)
			}
		}()
	}

	api := &api{core: c, store: st, brain: br, wa: waClient, voiceID: voiceID}
	mux := http.NewServeMux()
	mux.Handle("GET /api/events", hub)
	mux.HandleFunc("GET /api/state", api.state)
	mux.HandleFunc("GET /api/contacts/{id}/messages", api.messages)
	mux.HandleFunc("POST /api/contacts/{id}/status", api.setStatus)
	mux.HandleFunc("POST /api/contacts/{id}/send", api.send)
	mux.HandleFunc("GET /api/runs", api.runs)
	mux.HandleFunc("POST /api/simulate", api.simulate)
	mux.HandleFunc("POST /api/contacts", api.addContact)
	mux.HandleFunc("POST /api/campaign/draft", api.campaignDraft)
	mux.HandleFunc("POST /api/campaign/send", api.campaignSend)
	mux.Handle("GET /audio/", http.StripPrefix("/audio/", http.FileServer(http.Dir("data/audio"))))
	mux.Handle("GET /", spa(api.boot))

	srv := &http.Server{Addr: *addr, Handler: logRequests(mux)}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	log.Printf("dashboard on http://localhost%s", *addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// fromWA converts a WhatsApp message for the core.
func fromWA(c *wa.Client, in wa.Inbound) core.Msg {
	m := core.Msg{
		WAID: string(in.ID), Chat: in.Chat, From: in.From, Phone: in.Phone, PushName: in.PushName,
		Text: in.Text, Raw: in, At: in.Timestamp,
	}
	if in.Audio != nil {
		audio := in.Audio
		m.Voice = func(ctx context.Context) ([]byte, error) { return c.DownloadAudio(ctx, audio) }
	}
	return m
}

type waGateway struct{ c *wa.Client }

func (g waGateway) SendText(ctx context.Context, to types.JID, text string) (string, error) {
	id, err := g.c.SendText(ctx, to, text)
	return string(id), err
}

func (g waGateway) SendVoiceNote(ctx context.Context, to types.JID, vn voice.VoiceNote) (string, error) {
	id, err := g.c.SendVoiceNote(ctx, to, vn.Ogg, vn.Seconds, vn.Waveform)
	return string(id), err
}

func (g waGateway) Recording(chat types.JID) func() { return g.c.Recording(chat) }

func (g waGateway) MarkRead(ctx context.Context, m core.Msg) {
	if in, ok := m.Raw.(wa.Inbound); ok {
		g.c.MarkRead(ctx, in)
	}
}

// logGateway stands in for WhatsApp in -wa=false mode.
type logGateway struct{}

func (logGateway) SendText(_ context.Context, to types.JID, text string) (string, error) {
	log.Printf("[no-wa] text to %s: %q", to, text)
	return "", nil
}
func (logGateway) SendVoiceNote(_ context.Context, to types.JID, vn voice.VoiceNote) (string, error) {
	log.Printf("[no-wa] voice note to %s (%.1fs)", to, vn.Duration)
	return "", nil
}
func (logGateway) Recording(types.JID) func()         { return func() {} }
func (logGateway) MarkRead(context.Context, core.Msg) {}

type api struct {
	core    *core.Core
	store   *store.Store
	brain   *brain.Brain
	wa      *wa.Client
	voiceID string
}

func (a *api) state(w http.ResponseWriter, r *http.Request) {
	st, err := a.snapshot()
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, st)
}

// boot is inlined into index.html so the first paint already has data.
func (a *api) boot() any {
	st, err := a.snapshot()
	if err != nil {
		return nil
	}
	runs, _ := a.store.Runs(30)
	msgs := map[int64][]store.Message{}
	if cs, _ := a.store.Contacts(); len(cs) > 0 {
		m, _ := a.store.Messages(cs[0].ID, 200)
		msgs[cs[0].ID] = nonNil(m)
	}
	return map[string]any{"state": st, "runs": nonNil(runs), "messages": msgs}
}

func (a *api) snapshot() (map[string]any, error) {
	contacts, err := a.store.Contacts()
	if err != nil {
		return nil, err
	}
	stats, err := a.store.Stats()
	if err != nil {
		return nil, err
	}
	persona, _ := os.ReadFile("data/persona/john.json")
	faq, _ := os.ReadFile("data/faq.md")
	waState := map[string]any{"enabled": a.wa != nil, "connected": false}
	if a.wa != nil {
		waState["connected"] = a.wa.WA.IsConnected() && a.wa.WA.IsLoggedIn()
		if id := a.wa.WA.Store.ID; id != nil {
			waState["phone"] = id.User
		}
		waState["push_name"] = a.wa.WA.Store.PushName
	}
	last := map[int64]store.Message{}
	for _, c := range contacts {
		if m, _ := a.store.Messages(c.ID, 1); len(m) == 1 {
			last[c.ID] = m[0]
		}
	}
	minPerReply := atoi(config.Get("MIN_SAVED_PER_REPLY", "15"))
	return map[string]any{
		"ambassador": map[string]any{
			"name":         "John Klensch",
			"voice_id":     a.voiceID,
			"tts_model":    a.core.TTSModel,
			"brain_model":  a.brain.Model,
			"referral_url": a.brain.ReferralURL,
			"persona":      json.RawMessage(orJSON(persona)),
			"faq":          string(faq),
			"mock_voice":   a.core.Voice.Mock,
		},
		"contacts":    nonNil(contacts),
		"last":        last,
		"stats":       stats,
		"hours_saved": float64(stats.RepliesWeek*minPerReply) / 60,
		"whatsapp":    waState,
	}, nil
}

func (a *api) messages(w http.ResponseWriter, r *http.Request) {
	msgs, err := a.store.Messages(pathID(r), 200)
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, nonNil(msgs))
}

func (a *api) runs(w http.ResponseWriter, r *http.Request) {
	runs, err := a.store.Runs(30)
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, nonNil(runs))
}

func (a *api) setStatus(w http.ResponseWriter, r *http.Request) {
	var body struct{ Status string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpErr(w, err)
		return
	}
	switch body.Status {
	case store.StatusAuto, store.StatusHuman, store.StatusNeedsJohn:
	default:
		http.Error(w, "status must be auto, human or needs_john", http.StatusBadRequest)
		return
	}
	if err := a.core.SetStatus(pathID(r), body.Status); err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *api) send(w http.ResponseWriter, r *http.Request) {
	var body struct{ Text string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Text) == "" {
		http.Error(w, "text required", http.StatusBadRequest)
		return
	}
	if err := a.core.SendAsJohn(r.Context(), pathID(r), body.Text); err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// addContact saves a phone number under a name (e.g. a teammate as "Raj").
func (a *api) addContact(w http.ResponseWriter, r *http.Request) {
	var body struct{ Name, Phone, Context string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpErr(w, err)
		return
	}
	phone := digits(body.Phone)
	if len(phone) < 8 || strings.TrimSpace(body.Name) == "" {
		http.Error(w, "name and phone (with country code) required", http.StatusBadRequest)
		return
	}
	c, err := a.store.UpsertByPhone(phone, strings.TrimSpace(body.Name), strings.TrimSpace(body.Context))
	if err != nil {
		httpErr(w, err)
		return
	}
	a.core.Hub.Publish("contact", c)
	writeJSON(w, c)
}

func (a *api) campaignDraft(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ContactIDs []int64 `json:"contact_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.ContactIDs) == 0 || len(body.ContactIDs) > 8 {
		http.Error(w, "pick 1 to 8 contacts", http.StatusBadRequest)
		return
	}
	writeJSON(w, a.core.DraftOpeners(r.Context(), body.ContactIDs))
}

func (a *api) campaignSend(w http.ResponseWriter, r *http.Request) {
	var body struct{ Drafts []core.Draft }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Drafts) == 0 {
		http.Error(w, "drafts required", http.StatusBadRequest)
		return
	}
	a.core.SendOpeners(body.Drafts)
	writeJSON(w, map[string]bool{"ok": true})
}

// simulate runs the real pipeline for a pretend lead without touching WhatsApp.
func (a *api) simulate(w http.ResponseWriter, r *http.Request) {
	var body struct{ Name, Phone, Text string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Text) == "" {
		http.Error(w, "text required", http.StatusBadRequest)
		return
	}
	phone := digits(body.Phone)
	if phone == "" {
		phone = "990000000001"
	}
	jid := wa.PhoneJID(phone)
	go a.core.Handle(core.Msg{
		WAID: "sim-" + strconv.FormatInt(time.Now().UnixNano(), 36), Chat: jid, From: jid, Phone: phone,
		PushName: body.Name, Text: body.Text, Simulated: true, At: time.Now(),
	})
	writeJSON(w, map[string]bool{"ok": true})
}

// spa serves the embedded dashboard, falling back to index.html for client
// routes. boot() is inlined as window.__BOOT__ so the first render has data.
func spa(boot func() any) http.Handler {
	dist, _ := fs.Sub(web.Dist, "dist")
	files := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if _, err := fs.Stat(dist, p); err == nil {
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(dist, "index.html")
		if err != nil {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("U-Reach API is running. Build the dashboard: cd web && npm install && npm run build, then rebuild the server.\n"))
			return
		}
		if b, err := json.Marshal(boot()); err == nil {
			// Escape "<" so data can never close the script tag.
			script := "<script>window.__BOOT__=" + strings.ReplaceAll(string(b), "<", "\\u003c") + "</script>"
			index = []byte(strings.Replace(string(index), "<div id=\"root\"></div>", script+"<div id=\"root\"></div>", 1))
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(index)
	})
}

func logRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || strings.HasPrefix(r.URL.Path, "/api/state") {
			log.Printf("%s %s", r.Method, r.URL.Path)
		}
		h.ServeHTTP(w, r)
	})
}

func pathID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func httpErr(w http.ResponseWriter, err error) {
	log.Printf("api error: %v", err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func orJSON(b []byte) []byte {
	if len(b) == 0 || !json.Valid(b) {
		return []byte("null")
	}
	return b
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func digits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}
