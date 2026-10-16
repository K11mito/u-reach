<p align="center">
  <img src="docs/logo.png" width="128" alt="U-Reach logo">
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white" alt="Go 1.26">
  <img src="https://img.shields.io/badge/binary-single%2C%20no%20CGO-3fb950" alt="single binary, no CGO">
  <img src="https://img.shields.io/badge/platform-Linux-333?logo=linux&logoColor=white" alt="Linux">
  <img src="https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black" alt="React 19">
  <img src="https://img.shields.io/badge/Claude-Opus%205.5-D97757?logo=anthropic&logoColor=white" alt="Claude Opus 5.5">
  <img src="https://img.shields.io/badge/voice-ElevenLabs-000000?logo=elevenlabs&logoColor=white" alt="ElevenLabs voice">
  <img src="https://img.shields.io/badge/WhatsApp-whatsmeow-25D366?logo=whatsapp&logoColor=white" alt="WhatsApp via whatsmeow">
  <img src="https://img.shields.io/badge/SQLite-pure%20Go-003B57?logo=sqlite&logoColor=white" alt="SQLite, pure Go">
  <img src="https://img.shields.io/badge/U--Hackathon-2026-8b5cf6" alt="U-Hackathon 2026">
</p>

# U-Reach

Live Demo: https://jacob-backed-updating-stream.trycloudflare.com/

John's WhatsApp voice-note assistant. A lead messages John's number. About ten seconds later they get a voice note in John's cloned voice that greets them by name and answers their question, plus the next one they'd ask. Then they get his referral link. John's control room (Inbox, Campaign, Analytics, Profile, Help) shows it all live.

One Go binary: WhatsApp (whatsmeow), the reply pipeline (Scribe, then Claude, then ElevenLabs TTS), SQLite and SSE, with the React dashboard embedded.

## Setup
```sh
cp .env.example .env        # fill in ELEVENLABS_API_KEY, ANTHROPIC_API_KEY, JOHN_VOICE_ID, REFERRAL_URL
make build                  # builds web/dist and bin/ureach
go run ./cmd/seed           # 27 demo leads with history (add -render-audio once the voice is chosen)
```
Needs Go 1.26+, Node 20+, ffmpeg with libopus, and yt-dlp (only for pulling John's audio).

## Run
```sh
bin/ureach                  # first run prints a QR: WhatsApp > Linked devices > Link a device
bin/ureach -wa=false        # no WhatsApp; use "Simulate a lead message" in the Inbox
```
Dashboard: http://localhost:8080. If WhatsApp isn't linked yet, click **Link WhatsApp** in the sidebar to scan the QR from the browser.

- **Inbox:** live chats with search (`/`), filters, voice notes, and delivered/read/played ticks. **Take over** pauses the AI for a chat; replying from the demo phone itself does the same.
  - **Triage:** on every reply the AI rates the lead cold, warm or hot, with the clue ("asked how to pay with USDT"). Flames next to the name show it (two for hot, one for warm), and **Ready to call** (`/?ready=1`) shows only the hot leads: the people to call today.
- **Simulate** (bottom of the chat list): type a message, or press the mic to record a voice note. It runs the real pipeline but sends nothing to WhatsApp.
- **Stage view** (`/stage`): the projector screen, out of the nav for now. `F` toggles fullscreen and `Esc` exits.
- **Campaign:** add teammates' numbers under demo names, generate personal openers, listen and edit them, then send.
  - **Follow-ups** (`/campaign?view=followups`): when a lead hasn't answered John for 2 days (`NUDGE_AFTER_HOURS`), the AI drafts a short personal follow-up every 10 minutes (`NUDGE_AUTO=0` turns that off; **Draft now** works either way). Nothing goes out until John approves it. Drafts are text until he listens or sends, so skipped ones cost no voice credits. One follow-up per silence: a lead who writes back, or whose chat John takes over, drops out, and a draft is checked again just before it's sent. Demo leads (+99 numbers) get it added to their chat, never sent on WhatsApp.
- **Analytics:** the two-week funnel (leads reached, voice notes, links, joined) and what the APIs really cost over the last 24 hours.
- **John:** everything that shapes his replies, live with no restart:
  - **Presets** (the pills on his card): how he sells and his reply style (length, when the link goes out). Hover a preset to edit it, or start a new one.
  - **Autonomy slider:** Cautious hands money, safety and doubts to John; Balanced (the default) answers almost everything and hands off only what it can't fix or answer safely; Full autonomy only hands off money gone wrong, legal threats and emergencies. A reply that promises income is never sent at any level.
  - **Edit John:** his background, voice, phrases, openers and closers, and the lines he never crosses.
  - **Sales playbook:** upload Markdown, text or PDF strategy for him to read before every reply.
  - **Facts John can share**, his **referral link**, **Try John** (test a message without sending anything) and **Reset demo** for between rehearsals.
- **Help:** a guide for judges trying it from their own phone.
- **Sidebar status:** whether WhatsApp is connected, whether voice notes use John's clone or a test tone, and whether Claude answered the last request (it turns red with the reason, such as credit running out).
- Keys `1` to `6` switch pages.

`TTS_MOCK=1` replaces ElevenLabs with a tone, so no credits are spent.

Only one process can hold the WhatsApp session (`data/wa-session.db`). Stop `wa-spike` before starting the server; it reuses the same link.

### Working on it
```sh
make dev                                        # dashboard with hot reload on :5173, proxied to the API on :8080
API_URL=http://localhost:8090 npm --prefix web run dev -- --port 5174   # proxy to a second server instead
go test ./...                                   # needs ffmpeg; nothing calls Claude, ElevenLabs or WhatsApp
```
CI (`.github/workflows/ci.yml`) runs gofmt, vet, the tests with `-race` and the dashboard build on every pull request.

## Tools
| Command | What it does |
|---|---|
| `go run ./cmd/wa-spike` | WhatsApp go/no-go test: links, echoes texts, and replies to "voice" with a voice note |
| `scripts/pull-audio.sh`, `scripts/cut-clip.sh` | Download John's video audio and cut clips |
| `go run ./cmd/clone-voice clip.mp3 ...` | Make an Instant Voice Clone and render a test line (`-voice <id>` re-renders) |
| `go run ./cmd/brain-cli -samples data/brain-samples.txt` | Print the brain's replies without WhatsApp or TTS (`data/brain-eval.txt` has the hard questions) |
| `go run ./cmd/seed` | Reset app data to the seeded demo leads (`-add` only adds missing ones and keeps live chats) |

## Where things live
- `data/persona/john.json`, `data/faq.md`: John's default persona and the knowledge base the AI answers from. Edits made on the John page are saved in the database and win over these files.
- `internal/brain`: one Claude call per reply. Code enforces the word cap, no URLs aloud, the handoff net for the autonomy level (`autonomy.go`), the income-claim block, and sending the link once.
- `internal/harness`: presets, John's profile, the playbook documents and the autonomy setting behind the John page.
- `internal/core`: debounce, per-chat lock, hear, think, speak, send, plus fallbacks and the campaign.
- `docs/context.md`: what U-topia and U.Center are, and how ambassadors sell.

ElevenLabs credits are finite. Every TTS result is cached by voice, model, settings and text, and nothing calls TTS in a loop.
