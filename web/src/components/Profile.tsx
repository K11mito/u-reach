import { BadgeCheck, Link2, Mic, Quote } from "lucide-react"
import type { State } from "@/lib/api"
import { JohnAvatar } from "./bits"

/** John's voice and persona: Kyle's "Your voice, ready" step, already done. */
export function Profile({ state }: { state: State }) {
  const a = state.ambassador
  const p = a.persona
  return (
    <div className="grid min-h-0 flex-1 grid-cols-[380px_1fr] gap-4 overflow-hidden">
      <div className="space-y-4 overflow-y-auto">
        <div className="rounded-2xl bg-gradient-to-br from-indigo-600 to-purple-700 p-6 text-white shadow-lg">
          <JohnAvatar size={72} />
          <div className="mt-4 text-2xl font-extrabold">{p?.name ?? a.name}</div>
          <div className="text-sm text-white/80">{p?.role}</div>
          <div className="mt-5 space-y-2 rounded-xl bg-white/10 p-3 text-sm">
            <div className="flex items-center gap-2">
              <BadgeCheck className="size-4 text-wa" /> Intake interview · from one YouTube video
            </div>
            <div className="flex items-center gap-2">
              <Mic className="size-4 text-wa" /> {a.mock_voice ? "Voice clone not selected yet" : "Your voice, ready"}
            </div>
            <div className="flex items-center gap-2 break-all">
              <Link2 className="size-4 shrink-0 text-wa" /> {a.referral_url || "No referral link set"}
            </div>
          </div>
          <div className="mt-3 text-[11px] text-white/60">
            Voice {a.voice_id || "—"} · {a.tts_model} · brain {a.brain_model}
          </div>
        </div>
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
      <div className="grid min-h-0 grid-cols-2 gap-4 overflow-y-auto">
        <Card title="How John sells" items={p?.how_he_sells} />
        <Card title="Signature phrases" items={p?.signature_phrases} chips />
        <Card title="Background" items={p?.background} />
        <Card title="Never" items={p?.never} />
        <div className="col-span-2 rounded-2xl border bg-card p-5 shadow-sm">
          <div className="mb-2 font-bold">What the assistant may say (FAQ)</div>
          <pre className="max-h-80 overflow-y-auto font-sans text-sm whitespace-pre-wrap text-muted-foreground">{a.faq}</pre>
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
