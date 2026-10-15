# U-Reach

John's WhatsApp voice-note assistant. A lead messages John's number. A minute later they get a voice note in John's cloned voice that greets them by name and answers their question plus the next one they'd ask. Then they get his referral link. John's control room (Inbox, Stage view, Campaign, Profile) shows it all live.

One Go binary: WhatsApp (whatsmeow), reply pipeline (Scribe, then Claude, then ElevenLabs TTS), SQLite, SSE, with the React dashboard embedded.

## Setup
```sh
cp .env.example .env        # fill in ELEVENLABS_API_KEY, ANTHROPIC_API_KEY, JOHN_VOICE_ID, REFERRAL_URL
make build                  # builds web/dist and bin/ureach
go run ./cmd/seed           # 10 fake leads with history (add -render-audio once the voice is chosen)
```
Needs Go 1.26+, Node 20+, ffmpeg with libopus, and yt-dlp (only for pulling John's audio).

## Run
```sh
bin/ureach                  # first run prints a QR: WhatsApp > Linked devices > Link a device
bin/ureach -wa=false        # no WhatsApp; use "Simulate a lead message" in the Inbox
```
Dashboard: http://localhost:8080. If WhatsApp isn't linked yet, click **Link WhatsApp** in the sidebar to scan the QR from the browser.

- **Inbox:** live chats with search (`/`), filters, voice notes, and delivered/read/played ticks. **Take over** pauses the AI for a chat; replying from the demo phone itself does the same.
- **Simulate** (bottom of the chat list): type a message, or press the mic to record a voice note. It runs the real pipeline but sends nothing to WhatsApp.
- **Stage view** (`/stage`, or press `P`): the projector screen. `F` toggles fullscreen and `Esc` exits.
- **Campaign:** add teammates' numbers under demo names, generate personal openers, listen and edit them, then send.
- **Profile:** John's real voice next to his clone, the persona, the FAQ, and **Reset demo** for between rehearsals.
- Keys `1` to `4` switch pages.

`TTS_MOCK=1` replaces ElevenLabs with a tone, so no credits are spent.

Only one process can hold the WhatsApp session (`data/wa-session.db`). Stop `wa-spike` before starting the server; it reuses the same link.

## Tools
| Command | What it does |
|---|---|
| `go run ./cmd/wa-spike` | WhatsApp go/no-go test: links, echoes texts, and replies to "voice" with a voice note |
| `scripts/pull-audio.sh`, `scripts/cut-clip.sh` | Download John's video audio and cut clips |
| `go run ./cmd/clone-voice clip.mp3 ...` | Make an Instant Voice Clone and render a test line (`-voice <id>` re-renders) |
| `go run ./cmd/brain-cli -samples data/brain-samples.txt` | Print the brain's replies without WhatsApp or TTS |
| `go run ./cmd/seed` | Reset app data to the seeded demo leads |

## Where things live
- `data/persona/john.json`, `data/faq.md`: what John sounds like and what the assistant may say. Money, legal and "is it a pyramid" questions always hand off to John.
- `internal/brain`: one Claude call per reply; code enforces the word cap, no URLs aloud, the handoff net, and sending the link once.
- `internal/core`: debounce, per-chat lock, hear, think, speak, send, plus fallbacks and the campaign.
- `docs/context.md`: what U-topia and U.Center are, and how ambassadors sell.

ElevenLabs credits are finite. Every TTS result is cached by voice, model, settings and text, and nothing calls TTS in a loop.
