import { useCallback, useEffect, useState } from "react"
import { AudioLines, ChevronDown, Copy, Link2, Loader2, Mic, MonitorPlay, RefreshCw, RotateCcw, ScrollText, Upload } from "lucide-react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { api, errText, harnessApi, type HarnessPreset, type HarnessSnapshot, type State } from "@/lib/api"
import { VoiceNote } from "./bits"
import { toast } from "./Toast"
import { Section, TextField } from "./john/editors"
import { Hero } from "./john/Hero"
import { Playbook } from "./john/Playbook"
import { PresetEditor } from "./john/PresetEditor"
import { ProfileEditor } from "./john/ProfileEditor"
import { TryIt } from "./john/TryIt"

// Sub-pages live in the URL hash so the browser's back button works.
type View = { kind: "home" } | { kind: "preset"; id: number } | { kind: "about" }
const viewFromHash = (): View => {
  const m = window.location.hash.match(/^#preset\/(\d+)$/)
  if (m) return { kind: "preset", id: Number(m[1]) }
  return window.location.hash === "#about" ? { kind: "about" } : { kind: "home" }
}
const hashOf = (v: View) => (v.kind === "preset" ? `#preset/${v.id}` : v.kind === "about" ? "#about" : "")

/**
 * John's page. The top row is who he is, which preset he sells with and how
 * much his AI handles alone; presets and his profile open their own editors.
 * Below, in three columns: what he knows (playbook, knowledge base), how he
 * sounds (his voice), and trying him out (Try John, his link).
 */
export function Profile({ state }: { state: State }) {
  const [snap, setSnap] = useState<HarnessSnapshot | null>(null)
  const [view, setView] = useState<View>(viewFromHash)

  const load = useCallback(() => harnessApi.get().then(setSnap).catch((e) => toast(errText(e), "error")), [])
  useEffect(() => {
    load()
    const on = () => load()
    const pop = () => setView(viewFromHash())
    window.addEventListener("ureach:harness", on)
    window.addEventListener("popstate", pop)
    return () => {
      window.removeEventListener("ureach:harness", on)
      window.removeEventListener("popstate", pop)
    }
  }, [load])

  const open = (v: View) => {
    window.history.pushState(null, "", `/profile${hashOf(v)}`)
    setView(v)
  }
  const run = async (p: Promise<HarnessSnapshot>, ok?: string) => {
    try {
      setSnap(await p)
      if (ok) toast(ok)
      return true
    } catch (e) {
      toast(errText(e), "error")
      return false
    }
  }

  if (!snap) {
    return (
      <div className="grid flex-1 place-items-center text-sm text-muted-foreground">
        <Loader2 className="size-5 animate-spin" />
      </div>
    )
  }

  const editing = view.kind === "preset" ? snap.presets.find((p) => p.id === view.id) : undefined
  if (editing) {
    // key: a different preset starts a fresh form.
    return <PresetEditor key={editing.id} snap={snap} preset={editing} run={run} onBack={() => open({ kind: "home" })} onEditProfile={() => open({ kind: "about" })} />
  }
  if (view.kind === "about") return <ProfileEditor snap={snap} run={run} onBack={() => open({ kind: "home" })} />

  const activate = (p: HarnessPreset) => p.id !== snap.active_id && run(harnessApi.activate(p.id), `John now sells with “${p.name}”`)
  const setAutonomy = (level: HarnessSnapshot["autonomy"]) => {
    const l = snap.levels.find((x) => x.key === level)
    return run(harnessApi.setAutonomy(level), l ? `${l.label}: ${l.summary}` : undefined)
  }
  const newPreset = async (name: string) => {
    const active = snap.presets.find((p) => p.id === snap.active_id) ?? snap.presets[0]
    try {
      const s = await harnessApi.saveAs(name, `Started from “${active.name}”`, active.harness)
      setSnap(s)
      toast(`Created “${name}”. Make it yours.`)
      open({ kind: "preset", id: s.active_id })
    } catch (e) {
      toast(errText(e), "error")
    }
  }

  // Fills the window like the Inbox: the bottom row takes the rest of the
  // height and its flexible cards (knowledge base, voice training, Try John)
  // stretch or scroll inside. On a very short window the page scrolls.
  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <div className="flex h-full min-h-[46rem] flex-col gap-4">
        <Hero state={state} snap={snap} onActivate={activate} onEditPreset={(p) => open({ kind: "preset", id: p.id })} onEditProfile={() => open({ kind: "about" })} onNewPreset={newPreset} onAutonomy={setAutonomy} />
        <div className="grid min-h-0 flex-1 gap-4 lg:grid-cols-3 lg:grid-rows-[minmax(0,1fr)]">
          <div className="flex min-h-0 min-w-0 flex-col gap-4">
            <Playbook snap={snap} onChange={setSnap} className="shrink-0" />
            <Facts snap={snap} run={run} className="flex min-h-0 flex-1 flex-col" />
            <DemoTools />
          </div>
          <div className="flex min-h-0 min-w-0 flex-col gap-4">
            <Voice state={state} />
            <TrainVoice className="min-h-0 flex-1" />
          </div>
          <div className="flex min-h-0 min-w-0 flex-col gap-4">
            <TryIt className="flex min-h-0 flex-1 flex-col" />
            <Referral snap={snap} run={run} />
          </div>
        </div>
      </div>
    </div>
  )
}

type Run = (p: Promise<HarnessSnapshot>, ok?: string) => Promise<boolean>

function Referral({ snap, run }: { snap: HarnessSnapshot; run: Run }) {
  const [url, setUrl] = useState(snap.referral_url)
  useEffect(() => setUrl(snap.referral_url), [snap.referral_url])
  const changed = url.trim() !== snap.referral_url
  const save = () => run(harnessApi.setReferral(url), "Referral link saved")
  return (
    <Section title="Referral link" hint="Sent after his voice note; joiners get John as sponsor." icon={Link2} className="shrink-0">
      <div className="flex gap-2">
        <input
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && changed && save()}
          placeholder="https://u.center/?ref=…"
          className="h-9 min-w-0 flex-1 rounded-lg border border-input bg-background px-3 font-mono text-[13px] outline-none focus:border-ring focus:ring-[3px] focus:ring-ring/30"
        />
        {changed ? (
          <Button onClick={save}>Save</Button>
        ) : (
          <Button
            variant="outline"
            size="icon"
            disabled={!url}
            onClick={() =>
              navigator.clipboard
                .writeText(url)
                .then(() => toast("Referral link copied"))
                .catch(() => toast("Couldn't copy", "error"))
            }
            aria-label="Copy link"
            title="Copy link"
          >
            <Copy />
          </Button>
        )}
      </div>
    </Section>
  )
}

/** The knowledge base: a short preview of its topics that opens into an editor. */
function Facts({ snap, run, className }: { snap: HarnessSnapshot; run: Run; className?: string }) {
  const [open, setOpen] = useState(false)
  const [text, setText] = useState(snap.faq)
  useEffect(() => setText(snap.faq), [snap.faq])
  const changed = text !== snap.faq
  const topics = snap.faq.split("\n").filter((l) => l.startsWith("## ")).map((l) => l.slice(3))
  return (
    <Section
      title="Knowledge base"
      hint="What the AI knows about U-topia and U.Center."
      icon={ScrollText}
      className={className}
      action={
        open ? (
          <div className="flex gap-1">
            {changed && (
              <Button variant="ghost" size="sm" onClick={() => setText(snap.faq)}>
                Discard
              </Button>
            )}
            <Button size="sm" variant={changed ? "default" : "outline"} onClick={async () => (!changed || (await run(harnessApi.setFAQ(text), "Knowledge base saved"))) && setOpen(false)}>
              {changed ? "Save" : "Done"}
            </Button>
          </div>
        ) : (
          <Button size="sm" variant="outline" onClick={() => setOpen(true)}>
            Edit
          </Button>
        )
      }
    >
      {open ? (
        <>
          <div className="min-h-0 flex-1 overflow-y-auto rounded-lg">
            <TextField value={text} onChange={setText} rows={14} mono />
          </div>
          <div className="mt-1.5 text-right text-[11px] text-muted-foreground tabular-nums">{text.split(/\s+/).filter(Boolean).length.toLocaleString()} words · Markdown</div>
        </>
      ) : (
        <button onClick={() => setOpen(true)} className="group flex min-h-0 w-full flex-1 flex-col text-left">
          {/* As many topics as fit; the rest fade out under the edge. */}
          <div className="relative min-h-0 flex-1 overflow-hidden [mask-image:linear-gradient(to_bottom,black_calc(100%-1.5rem),transparent)]">
            <div className="flex flex-wrap gap-1.5">
              {topics.map((t) => (
                <span key={t} className="rounded-md bg-secondary px-2 py-1 text-xs text-foreground/80">
                  {t}
                </span>
              ))}
            </div>
          </div>
          <div className="mt-2 flex shrink-0 items-center gap-1 text-[11px] text-muted-foreground group-hover:text-foreground">
            {snap.faq.split(/\s+/).filter(Boolean).length.toLocaleString()} words across {topics.length} topics <ChevronDown className="size-3" />
          </div>
        </button>
      )}
    </Section>
  )
}

const MODELS: Record<string, string> = {
  eleven_flash_v2_5: "ElevenLabs Flash v2.5",
  eleven_multilingual_v2: "ElevenLabs Multilingual v2",
  eleven_turbo_v2_5: "ElevenLabs Turbo v2.5",
}

/** The real John next to his clone, so anyone can hear how close it is. */
function Voice({ state }: { state: State }) {
  const a = state.ambassador
  const ready = !a.mock_voice
  return (
    <Section
      title="His voice"
      hint="The real John next to his AI clone."
      icon={AudioLines}
      action={
        <span
          title={ready ? `Leads hear the clone, made with ${MODELS[a.tts_model] ?? a.tts_model}` : "No voice clone set: notes are a placeholder tone"}
          className={cn("inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-semibold ring-1", ready ? "bg-emerald-50 text-emerald-700 ring-emerald-200" : "bg-amber-50 text-amber-800 ring-amber-200")}
        >
          <span className={cn("size-1.5 rounded-full", ready ? "bg-emerald-500" : "bg-amber-500")} />
          {ready ? "Clone live" : "Test tone"}
        </span>
      }
    >
      {a.samples?.original || a.samples?.clone ? (
        <div className="space-y-2">
          {a.samples.original && (
            <div className="rounded-xl bg-secondary px-3.5 py-2">
              <div className="mb-1 flex items-center gap-1.5 text-[11px] font-semibold text-muted-foreground">
                <MonitorPlay className="size-3.5 text-red-500" /> The real John, from his talk
              </div>
              <VoiceNote src={a.samples.original} durationMs={14000} seed={11} />
            </div>
          )}
          {a.samples.clone && (
            <div className="rounded-xl bg-wa-bubble px-3.5 py-2 ring-1 ring-wa/15">
              <div className="mb-1 flex items-center gap-1.5 text-[11px] font-semibold text-wa-dark">
                <Mic className="size-3.5" /> His AI voice
              </div>
              <VoiceNote src={a.samples.clone} durationMs={9000} seed={22} outgoing />
            </div>
          )}
        </div>
      ) : (
        <p className="rounded-xl bg-secondary px-3.5 py-3 text-sm text-muted-foreground">The samples appear here once his voice clone is set up.</p>
      )}
    </Section>
  )
}

/** Not built yet: where John would add recordings to improve his clone. Greyed out on purpose. */
function TrainVoice({ className }: { className?: string }) {
  return (
    <Section
      title="Train his voice"
      hint="Add recordings to make the clone even closer."
      icon={Upload}
      className={cn("flex flex-col bg-muted/40", className)}
      action={<span className="rounded-full bg-secondary px-2.5 py-1 text-[11px] font-semibold text-muted-foreground ring-1 ring-border">Coming soon</span>}
    >
      <div aria-disabled className="pointer-events-none flex min-h-0 flex-1 flex-col gap-3 opacity-55 select-none">
        <div className="flex min-h-14 flex-1 flex-col items-center justify-center gap-1 overflow-hidden rounded-xl border-2 border-dashed border-foreground/15 px-4 py-3 text-center">
          <span className="flex items-center gap-2 text-sm font-medium">
            <Upload className="size-4 text-muted-foreground" /> Upload audio
          </span>
          <span className="text-xs text-muted-foreground">MP3, WAV or M4A · 1 to 3 minutes of just John, no music</span>
        </div>
        <Button variant="outline" disabled className="w-full shrink-0">
          <RefreshCw /> Retrain the voice clone
        </Button>
      </div>
    </Section>
  )
}

/** Rehearsal helper: put the seeded demo back. The WhatsApp link is kept. */
function DemoTools() {
  const [busy, setBusy] = useState(false)
  const reset = async () => {
    if (!window.confirm("Reset to the demo leads? This deletes every conversation since the last reset. WhatsApp stays linked, and John's settings are kept.")) return
    setBusy(true)
    try {
      await api.resetDemo() // every open dashboard reloads on the "reset" event
    } catch (e) {
      toast(`Reset failed: ${errText(e)}`, "error")
      setBusy(false)
    }
  }
  return (
    <div className="flex shrink-0 items-center gap-4 rounded-xl border border-dashed bg-card/60 px-4 py-3">
      <div className="min-w-0 flex-1">
        <div className="text-sm font-semibold">Demo tools</div>
        <p className="text-xs text-muted-foreground">Restore the demo leads between rehearsals.</p>
      </div>
      <Button variant="outline" size="sm" onClick={reset} disabled={busy}>
        {busy ? <Loader2 className="animate-spin" /> : <RotateCcw />} Reset demo
      </Button>
    </div>
  )
}
