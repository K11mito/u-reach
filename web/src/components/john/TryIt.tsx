import { useState } from "react"
import { Link2, Loader2, MessageSquareText, Mic, Send, TriangleAlert } from "lucide-react"
import { Button } from "@/components/ui/button"
import { errText, explainError, harnessApi, type Harness, type HarnessProfile, type TryResult } from "@/lib/api"
import { Section } from "./editors"

const SAMPLES = ["Hey John, what is U.Center?", "Is this safe? I lost money on an exchange before", "How much does it cost to join?", "Can I use it to send money home to the Philippines?"]

/**
 * Ask John something. With a draft it answers with those unsaved edits;
 * without one, with what's live. Nothing is saved or sent.
 */
export function TryIt({
  draft,
  title = "Try John",
  hint = "Ask anything; nothing is sent to WhatsApp.",
  className,
}: {
  draft?: { profile?: HarnessProfile; preset?: Harness }
  title?: string
  hint?: string
  className?: string
}) {
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
    <Section title={title} hint={hint} icon={MessageSquareText} className={className}>
      <div className="flex gap-2">
        <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Lead name" className="w-24 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:border-ring" />
        <input
          value={msg}
          onChange={(e) => setMsg(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && ask()}
          placeholder="Type what a lead would say…"
          className="h-9 min-w-0 flex-1 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:border-ring"
        />
        <Button onClick={() => ask()} disabled={busy || !msg.trim()} size="icon" aria-label="Ask John" className="bg-indigo-600 hover:bg-indigo-700">
          {busy ? <Loader2 className="animate-spin" /> : <Send />}
        </Button>
      </div>
      {!res && (
        <div className="mt-2 flex flex-wrap gap-1.5">
          {SAMPLES.map((s) => (
            <button key={s} onClick={() => ask(s)} disabled={busy} className="rounded-md bg-secondary px-2 py-1 text-[11px] text-foreground/70 hover:bg-indigo-50 hover:text-indigo-800">
              {s}
            </button>
          ))}
        </div>
      )}

      {err && (
        <div className="mt-4 flex gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2.5 text-xs text-red-800">
          <TriangleAlert className="mt-px size-4 shrink-0" />
          <span className="min-w-0 [overflow-wrap:anywhere]">
            {explainError(err.replace(/^John's AI couldn't answer: /, ""))}
            {/^John's AI couldn't answer: /.test(err) && (
              <span className="mt-0.5 block truncate font-mono text-[11px] text-red-900/55" title={err}>
                {err}
              </span>
            )}
          </span>
        </div>
      )}
      {res && (
        <div className="mt-3 min-h-0 flex-1 space-y-2 overflow-y-auto">
          <div className="ml-6 rounded-xl rounded-br-sm bg-wa-bubble px-4 py-3 ring-1 ring-wa/15 text-sm leading-relaxed">
            <span className="mb-1 flex items-center gap-1 text-[11px] font-medium text-wa-dark">
              <Mic className="size-3" /> Voice note · {res.words} words · ~{Math.round(res.words / 2.5)}s
            </span>
            {res.reply.spoken_script}
          </div>
          {res.reply.followup_text && <div className="ml-6 rounded-xl rounded-br-sm bg-wa-bubble px-4 py-2.5 ring-1 ring-wa/15 text-sm whitespace-pre-wrap [overflow-wrap:anywhere]">{res.reply.followup_text}</div>}
          <div className="flex flex-wrap gap-1.5 pt-1 text-[11px]">
            {res.reply.handoff && <Flag tone="amber">Hands to John: {res.reply.handoff_reason || res.forced_handoff}</Flag>}
            {res.reply.opt_out && <Flag tone="amber">Lead opted out</Flag>}
            {res.reply.include_referral ? (
              <Flag tone="green">
                <Link2 className="size-3" /> Sends his referral link
              </Flag>
            ) : (
              <Flag>Referral link held until they're ready</Flag>
            )}
            {res.reply.info_link && (
              <Flag tone="green">
                <Link2 className="size-3" /> Shares {res.reply.info_link}
              </Flag>
            )}
            {res.reply.interest && <Flag>Rates them {res.reply.interest}</Flag>}
            {res.trimmed && <Flag>Trimmed to length</Flag>}
            <Flag>
              {(res.latency_ms / 1000).toFixed(1)}s · {res.model}
            </Flag>
            <button onClick={() => (setRes(null), setMsg(""))} className="ml-auto font-medium text-indigo-700 hover:underline">
              Try another
            </button>
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
