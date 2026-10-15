import { useState } from "react"
import { BadgeCheck, Copy, Link2, Loader2, Mic, MonitorPlay, Quote, RotateCcw } from "lucide-react"
import { Button } from "@/components/ui/button"
import { api, type State } from "@/lib/api"
import { JohnAvatar, VoiceNote } from "./bits"
import { toast } from "./Toast"

/** John's voice and persona: Kyle's "Your voice, ready" step, already done. */
export function Profile({ state }: { state: State }) {
  const a = state.ambassador
  const p = a.persona
  const copy = () =>
    navigator.clipboard
      .writeText(a.referral_url)
      .then(() => toast("Referral link copied"))
      .catch(() => toast("Couldn't copy", "error"))

  return (
    <div className="grid min-h-0 flex-1 grid-cols-[minmax(320px,380px)_1fr] gap-4 overflow-hidden">
      <div className="space-y-4 overflow-y-auto pr-1">
        <div className="rounded-2xl bg-gradient-to-br from-indigo-600 to-purple-700 p-6 text-white shadow-lg">
          <JohnAvatar size={72} src={a.avatar} />
          <div className="mt-4 text-2xl font-extrabold">{p?.name ?? a.name}</div>
          <div className="text-sm text-white/80">{p?.role}</div>
          <div className="mt-5 space-y-2 rounded-xl bg-white/10 p-3 text-sm">
            <div className="flex items-center gap-2">
              <BadgeCheck className="size-4 shrink-0 text-wa" /> Intake: persona built from one YouTube talk
            </div>
            <div className="flex items-center gap-2">
              <Mic className="size-4 shrink-0 text-wa" /> {a.mock_voice ? "Voice clone not selected yet" : "Your voice, ready"}
            </div>
          </div>
          <div className="mt-3 flex items-center gap-2 rounded-xl bg-white/10 p-2 pl-3 text-sm">
            <Link2 className="size-4 shrink-0 text-wa" />
            <span className="min-w-0 flex-1 truncate">{a.referral_url || "No referral link set"}</span>
            {a.referral_url && (
              <button onClick={copy} className="rounded-lg bg-white/15 p-1.5 hover:bg-white/25" aria-label="Copy referral link" title="Copy link">
                <Copy className="size-3.5" />
              </button>
            )}
          </div>
          <div className="mt-3 text-[11px] text-white/60">
            Voice {a.voice_id || "none"} · {a.tts_model} · brain {a.brain_model}
          </div>
        </div>

        {(a.samples?.original || a.samples?.clone) && (
          <div className="rounded-2xl border bg-card p-5 shadow-sm">
            <div className="mb-3 font-bold">John's voice</div>
            {a.samples.original && (
              <div className="mb-3">
                <div className="mb-1.5 flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
                  <MonitorPlay className="size-3.5 text-red-500" /> The real John, from his YouTube talk
                </div>
                <div className="rounded-2xl bg-muted px-3 py-2">
                  <VoiceNote src={a.samples.original} durationMs={14000} seed={11} />
                </div>
              </div>
            )}
            {a.samples.clone && (
              <div>
                <div className="mb-1.5 flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
                  <Mic className="size-3.5 text-wa-dark" /> His AI voice, saying something new
                </div>
                <div className="rounded-2xl bg-wa-bubble px-3 py-2">
                  <VoiceNote src={a.samples.clone} durationMs={9000} seed={22} outgoing />
                </div>
                <p className="mt-1.5 text-xs text-muted-foreground italic">“{a.clone_line}”</p>
              </div>
            )}
          </div>
        )}

        {p?.sample_lines && (
          <div className="rounded-2xl border bg-card p-5 shadow-sm">
            <div className="mb-3 flex items-center gap-2 font-bold">
              <Quote className="size-4 text-primary" /> In John's own words
            </div>
            <div className="space-y-2">
              {p.sample_lines.slice(0, 6).map((l) => (
                <p key={l} className="rounded-xl bg-secondary/60 px-3 py-2 text-sm text-secondary-foreground italic">
                  “{l}”
                </p>
              ))}
            </div>
          </div>
        )}
      </div>
      <div className="grid min-h-0 auto-rows-min grid-cols-2 gap-4 overflow-y-auto pr-1">
        <Card title="How John sells" items={p?.how_he_sells} />
        <Card title="Signature phrases" items={p?.signature_phrases} chips />
        <Card title="Background" items={p?.background} />
        <Card title="Never" items={p?.never} />
        <DemoTools />
        <div className="col-span-2 rounded-2xl border bg-card p-5 shadow-sm">
          <div className="mb-1 font-bold">What the assistant may say</div>
          <p className="mb-3 text-xs text-muted-foreground">Anything not on this page, it hands to John. Money, legal and "is this a pyramid" questions always go to him.</p>
          <pre className="max-h-96 overflow-y-auto rounded-xl bg-muted/50 p-4 font-sans text-sm whitespace-pre-wrap text-foreground/80">{a.faq}</pre>
        </div>
      </div>
    </div>
  )
}

function Card({ title, items, chips }: { title: string; items?: string[]; chips?: boolean }) {
  if (!items?.length) return null
  return (
    <div className="rounded-2xl border bg-card p-5 shadow-sm">
      <div className="mb-3 font-bold">{title}</div>
      {chips ? (
        <div className="flex flex-wrap gap-1.5">
          {items.map((i) => (
            <span key={i} className="rounded-full bg-accent px-2.5 py-1 text-xs font-medium text-accent-foreground">
              {i}
            </span>
          ))}
        </div>
      ) : (
        <ul className="space-y-2 text-sm text-muted-foreground">
          {items.map((i) => (
            <li key={i} className="flex gap-2">
              <span className="mt-1.5 size-1.5 shrink-0 rounded-full bg-primary" />
              {i}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

/** Rehearsal helper: put the seeded demo back. The WhatsApp link is kept. */
function DemoTools() {
  const [busy, setBusy] = useState(false)
  const reset = async () => {
    if (!window.confirm("Reset to the demo leads? This deletes every conversation since the last reset. WhatsApp stays linked.")) return
    setBusy(true)
    try {
      await api.resetDemo() // every open dashboard reloads on the "reset" event
    } catch (e) {
      toast(`Reset failed: ${e}`, "error")
      setBusy(false)
    }
  }
  return (
    <div className="col-span-2 flex items-center gap-4 rounded-2xl border border-dashed bg-card/60 p-5">
      <div className="min-w-0 flex-1">
        <div className="font-bold">Demo tools</div>
        <p className="text-xs text-muted-foreground">Between rehearsals: restore the 10 seeded leads and clear live chats. Seeded voice notes come from the cache, so it's free.</p>
      </div>
      <Button variant="outline" onClick={reset} disabled={busy} className="rounded-full">
        {busy ? <Loader2 className="animate-spin" /> : <RotateCcw />} Reset demo
      </Button>
    </div>
  )
}
