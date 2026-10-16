import { useCallback, useEffect, useState } from "react"
import { ChevronDown, Copy, Link2, Loader2, Mic, MonitorPlay, RotateCcw, ScrollText } from "lucide-react"
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
 * John's page. The card at the top is who he is and which preset he sells
 * with; presets and his profile open their own editors. Below are the things
 * shared by every preset: the sales playbook, his facts and his link.
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

  return (
    <div className="min-h-0 flex-1 space-y-4 overflow-y-auto pr-1 pb-6">
      <Hero state={state} snap={snap} onActivate={activate} onEditPreset={(p) => open({ kind: "preset", id: p.id })} onEditProfile={() => open({ kind: "about" })} onNewPreset={newPreset} onAutonomy={setAutonomy} />
      <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(340px,420px)]">
        <div className="space-y-4">
          <Playbook snap={snap} onChange={setSnap} />
          <Facts snap={snap} run={run} />
        </div>
        <div className="space-y-4">
          <TryIt />
          <Referral snap={snap} run={run} />
          <Voice state={state} />
          <DemoTools />
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
    <Section title="Referral link" hint="Sent as a text after his voice note. Leads who join through it get John as their sponsor." icon={Link2}>
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
function Facts({ snap, run }: { snap: HarnessSnapshot; run: Run }) {
  const [open, setOpen] = useState(false)
  const [text, setText] = useState(snap.faq)
  useEffect(() => setText(snap.faq), [snap.faq])
  const changed = text !== snap.faq
  const topics = snap.faq.split("\n").filter((l) => l.startsWith("## ")).map((l) => l.slice(3))
  return (
    <Section
      title="Knowledge base"
      hint="What the AI knows about U-topia and U.Center, built from U.Center's calls, u.center and u-topia.com. Every preset answers from it, and only hands to John when truly stuck."
      icon={ScrollText}
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
          <div className="max-h-[32rem] overflow-y-auto rounded-lg">
            <TextField value={text} onChange={setText} rows={14} mono />
          </div>
          <div className="mt-1.5 text-right text-[11px] text-muted-foreground tabular-nums">{text.split(/\s+/).filter(Boolean).length.toLocaleString()} words · Markdown</div>
        </>
      ) : (
        <button onClick={() => setOpen(true)} className="group w-full text-left">
          <div className="flex flex-wrap gap-1.5">
            {topics.map((t) => (
              <span key={t} className="rounded-md bg-secondary px-2 py-1 text-xs text-foreground/80">
                {t}
              </span>
            ))}
          </div>
          <div className="mt-2 flex items-center gap-1 text-[11px] text-muted-foreground group-hover:text-foreground">
            {snap.faq.split(/\s+/).filter(Boolean).length.toLocaleString()} words across {topics.length} topics <ChevronDown className="size-3" />
          </div>
        </button>
      )}
    </Section>
  )
}

function Voice({ state }: { state: State }) {
  const a = state.ambassador
  if (!a.samples?.original && !a.samples?.clone) return null
  return (
    <Section title="His voice" hint="The real John next to his clone." icon={Mic}>
      <div className="space-y-2">
        {a.samples.original && (
          <div className="rounded-xl bg-secondary px-3 py-2">
            <div className="mb-1 flex items-center gap-1.5 text-[11px] font-semibold text-muted-foreground">
              <MonitorPlay className="size-3.5 text-red-500" /> The real John
            </div>
            <VoiceNote src={a.samples.original} durationMs={14000} seed={11} />
          </div>
        )}
        {a.samples.clone && (
          <div className="rounded-xl bg-wa-bubble px-3 py-2">
            <div className="mb-1 flex items-center gap-1.5 text-[11px] font-semibold text-wa-dark">
              <Mic className="size-3.5" /> His AI voice
            </div>
            <VoiceNote src={a.samples.clone} durationMs={9000} seed={22} outgoing />
          </div>
        )}
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
    <div className={cn("flex items-center gap-4 rounded-xl border border-dashed p-4")}>
      <div className="min-w-0 flex-1">
        <div className="text-sm font-semibold">Demo tools</div>
        <p className="text-xs text-muted-foreground">Restore the 10 seeded leads between rehearsals. John's settings stay.</p>
      </div>
      <Button variant="outline" size="sm" onClick={reset} disabled={busy}>
        {busy ? <Loader2 className="animate-spin" /> : <RotateCcw />} Reset demo
      </Button>
    </div>
  )
}
