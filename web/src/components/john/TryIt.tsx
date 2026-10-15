import { useState } from "react"
import { Link2, Loader2, Send, Sparkles, TriangleAlert } from "lucide-react"
import { Button } from "@/components/ui/button"
import { errText, harnessApi, type Harness, type HarnessProfile, type TryResult } from "@/lib/api"
import { Section } from "./editors"

const SAMPLES = ["Hey John, what is U.Center?", "Is this safe? I lost money on an exchange before", "How much does it cost to join?", "Can I use it to send money home to the Philippines?"]

/**
 * Ask John something. With a draft it answers with those unsaved edits;
 * without one, with what's live. Nothing is saved or sent.
 */
export function TryIt({ draft, title = "Try John", hint = "Hear how he'd answer right now. Nothing is sent to WhatsApp." }: { draft?: { profile?: HarnessProfile; preset?: Harness }; title?: string; hint?: string }) {
  const [name, setName] = useState("Raj")
  const [msg, setMsg] = useState("")
  const [busy, setBusy] = useState(false)
  const [res, setRes] = useState<TryResult | null>(null)
  const [err, setErr] = useState("")

  const ask = async (text = msg) => {
    if (!text.trim()) return
    setMsg(text)
    setBusy(true)
    setErr("")
    try {
      setRes(await harnessApi.tryIt(name, text, draft))
    } catch (e) {
      setRes(null)
      setErr(errText(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Section title={title} hint={hint} icon={Sparkles}>
      <div className="flex gap-2">
        <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Lead name" className="w-24 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:border-ring" />
        <input
          value={msg}
          onChange={(e) => setMsg(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && ask()}
          placeholder="Type what a lead would say…"
          className="h-9 min-w-0 flex-1 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:border-ring"
        />
        <Button onClick={() => ask()} disabled={busy || !msg.trim()} size="icon" aria-label="Ask John">
          {busy ? <Loader2 className="animate-spin" /> : <Send />}
        </Button>
      </div>
      <div className="mt-2 flex flex-wrap gap-1.5">
        {SAMPLES.map((s) => (
          <button key={s} onClick={() => ask(s)} disabled={busy} className="rounded-md bg-secondary px-2 py-1 text-[11px] text-foreground/70 hover:bg-foreground/10 hover:text-foreground">
            {s}
          </button>
        ))}
      </div>

      {err && (
        <div className="mt-4 flex gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2.5 text-xs text-red-800">
          <TriangleAlert className="mt-px size-4 shrink-0" />
          <span className="min-w-0 [overflow-wrap:anywhere]">{err}</span>
        </div>
      )}
      {res && (
        <div className="mt-4 space-y-2">
          <div className="ml-6 rounded-2xl rounded-br-md bg-wa-bubble px-4 py-3 text-sm leading-relaxed">
            <span className="mb-1 block text-[11px] font-semibold text-wa-dark">
              🎤 Voice note · {res.words} words · ~{Math.round(res.words / 2.5)}s
            </span>
            {res.reply.spoken_script}
          </div>
          {res.reply.followup_text && <div className="ml-6 rounded-2xl rounded-br-md bg-wa-bubble px-4 py-2.5 text-sm whitespace-pre-wrap [overflow-wrap:anywhere]">{res.reply.followup_text}</div>}
          <div className="flex flex-wrap gap-1.5 pt-1 text-[11px]">
            {res.reply.handoff && <Flag tone="amber">Hands to John: {res.reply.handoff_reason || res.forced_handoff}</Flag>}
            {res.reply.opt_out && <Flag tone="amber">Lead opted out</Flag>}
            {res.reply.include_referral ? (
              <Flag tone="green">
                <Link2 className="size-3" /> Sends the link
              </Flag>
            ) : (
              <Flag>No link yet</Flag>
            )}
            {res.trimmed && <Flag>Trimmed to length</Flag>}
            <Flag>
              {(res.latency_ms / 1000).toFixed(1)}s · {res.model}
            </Flag>
          </div>
        </div>
      )}
    </Section>
  )
}

function Flag({ children, tone }: { children: React.ReactNode; tone?: "amber" | "green" }) {
  const cls = tone === "amber" ? "bg-amber/20 text-amber-900" : tone === "green" ? "bg-wa/15 text-wa-dark" : "bg-secondary text-muted-foreground"
  return <span className={`inline-flex items-center gap-1 rounded-md px-2 py-0.5 font-medium ${cls}`}>{children}</span>
}
