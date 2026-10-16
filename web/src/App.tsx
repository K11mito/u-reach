import { useCallback, useEffect, useState } from "react"
import { Bot, ChartColumn, CircleHelp, Inbox as InboxIcon, Megaphone, MonitorPlay, Mic, QrCode, UserRound } from "lucide-react"
import { cn } from "@/lib/utils"
import { explainError, useLive, type BrainHealth, type WhatsAppStatus } from "@/lib/api"
import { Inbox } from "@/components/Inbox"
import { Stage } from "@/components/Stage"
import { Profile } from "@/components/Profile"
import { Campaign } from "@/components/Campaign"
import { Analytics, prefetchAnalytics } from "@/components/Analytics"
import { Help } from "@/components/Help"
import { LinkWhatsApp } from "@/components/LinkWhatsApp"
import { BrandHeader } from "@/components/BrandHeader"
import { JohnAvatar } from "@/components/bits"
import robot from "@/assets/nav-robot.webp"
import logo from "@/assets/logo.webp"
import { Toaster, toast } from "@/components/Toast"

type Page = "inbox" | "stage" | "campaign" | "analytics" | "profile" | "help"
const TITLES: Record<Page, string> = { inbox: "Inbox", stage: "Stage", campaign: "Campaign", analytics: "Analytics", profile: "John", help: "Help" }
const pageFromPath = (): Page => {
  const p = window.location.pathname.replace(/^\//, "")
  return (["stage", "campaign", "analytics", "profile", "help"].includes(p) ? p : "inbox") as Page
}

// Big screens at 100% browser zoom render the dashboard small. Scale it up
// there, like 120% zoom, so every page matches; laptops that already render
// larger (or a browser already zoomed in) get no extra scale.
const uiScale = () => Math.min(1.2, Math.max(1, window.innerWidth / 1600))

function useUiScale() {
  const [scale, setScale] = useState(uiScale)
  useEffect(() => {
    const resize = () => setScale(uiScale())
    window.addEventListener("resize", resize)
    return () => window.removeEventListener("resize", resize)
  }, [])
  return scale
}

export default function App() {
  const { state, runs, messages, loadMessages, connected, campaign } = useLive()
  const [page, setPage] = useState<Page>(pageFromPath)
  const [linkOpen, setLinkOpen] = useState(false)
  const scale = useUiScale()
  const go = useCallback((p: Page) => {
    window.history.pushState(null, "", p === "inbox" ? "/" : `/${p}`)
    setPage(p)
    // Lets a page with sub-views (John's editors) return to its top level.
    window.dispatchEvent(new PopStateEvent("popstate"))
  }, [])
  useEffect(() => {
    const pop = () => setPage(pageFromPath())
    window.addEventListener("popstate", pop)
    return () => window.removeEventListener("popstate", pop)
  }, [])
  useEffect(() => {
    document.title = `U-Reach · ${TITLES[page]}`
  }, [page])
  useEffect(() => {
    // A few seconds after opening, so it never competes with the first paint.
    const t = window.setTimeout(prefetchAnalytics, 3000)
    return () => window.clearTimeout(t)
  }, [])

  // Number keys switch pages; P presents (Stage view). Ignored while typing.
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return
      const map: Record<string, Page> = { "1": "inbox", "2": "stage", "3": "campaign", "4": "analytics", "5": "profile", "6": "help", p: "stage" }
      if (map[e.key]) go(map[e.key])
    }
    window.addEventListener("keydown", key)
    return () => window.removeEventListener("keydown", key)
  }, [go])

  // Tell John when WhatsApp drops or comes back.
  const wa = state?.whatsapp
  const [prevConnected, setPrevConnected] = useState(wa?.connected)
  if (wa && wa.connected !== prevConnected) {
    setPrevConnected(wa.connected)
    if (prevConnected !== undefined && wa.enabled) toast(wa.connected ? "WhatsApp connected" : "WhatsApp disconnected, reconnecting…", wa.connected ? "info" : "error")
  }
  // …and when Claude stops answering, before leads pile up in Needs John.
  const aiOK = state?.brain?.ok ?? true
  const [prevAiOK, setPrevAiOK] = useState(aiOK)
  if (aiOK !== prevAiOK) {
    setPrevAiOK(aiOK)
    toast(aiOK ? "John's AI is answering again" : `John's AI can't reply: ${explainError(state?.brain?.error)} Leads get a holding note.`, aiOK ? "info" : "error")
  }

  if (!state) {
    return (
      <div className="grid h-screen place-items-center text-muted-foreground">
        <div className="flex items-center gap-3">
          <img src={logo} alt="" className="size-9 animate-pulse" />
          {connected ? "Loading…" : "Connecting to U-Reach…"}
        </div>
      </div>
    )
  }
  if (page === "stage") {
    return (
      <>
        <Stage state={state} runs={runs} messages={messages} loadMessages={loadMessages} onExit={() => go("inbox")} />
        <Toaster />
      </>
    )
  }

  const nav = [
    { id: "inbox" as const, label: "Inbox", icon: InboxIcon, badge: state.stats.needs_john },
    { id: "stage" as const, label: "Stage view", icon: MonitorPlay },
    { id: "campaign" as const, label: "Campaign", icon: Megaphone },
    { id: "analytics" as const, label: "Analytics", icon: ChartColumn },
    { id: "profile" as const, label: "John", icon: UserRound },
    { id: "help" as const, label: "Help", icon: CircleHelp },
  ]

  return (
    // Laid out at 1/scale of the window, then scaled up to fill it. The transform also anchors the modal and toasts to this box.
    <div
      className="flex min-w-[960px] origin-top-left flex-col gap-4 bg-gradient-to-r from-[oklch(0.95_0.035_300)] to-[oklch(0.95_0.035_245)] p-4"
      style={{ width: `${100 / scale}vw`, height: `${100 / scale}vh`, transform: scale === 1 ? undefined : `scale(${scale})` }}
    >
      <BrandHeader />

      <div className="flex min-h-0 flex-1 gap-4">
        <aside className="-mt-8 ml-2 flex w-[clamp(220px,18vw,300px)] shrink-0 flex-col rounded-2xl bg-gradient-to-b from-indigo-600 to-purple-700 p-4 pt-12 text-white shadow-lg shadow-indigo-500/20">
          <nav className="space-y-1">
            {nav.map((n, i) => (
              <button
                key={n.id}
                onClick={() => go(n.id)}
                title={`${n.label} (${i + 1})`}
                className={cn(
                  "flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-semibold transition",
                  page === n.id ? "bg-white text-indigo-700 shadow" : "text-white/85 hover:bg-white/10",
                )}
              >
                <n.icon className="size-4" />
                <span className="flex-1 text-left">{n.label}</span>
                {!!n.badge && <span className="min-w-5 rounded-full bg-amber-100 px-1.5 text-center text-[11px] font-semibold text-amber-800 tabular-nums">{n.badge}</span>}
              </button>
            ))}
          </nav>
          {/* Fills the free space above John's card; on short screens the top is cropped, keeping the robot, and on very short ones (zoomed in) it hides. */}
          <div className="relative mt-4 mb-3 min-h-0 flex-1">
            <img src={robot} alt="" className="absolute bottom-0 -left-3 max-h-full w-[68%] object-cover [@media(max-height:680px)]:hidden object-[center_76%] opacity-80 [mask-image:linear-gradient(to_bottom,transparent,black_10%,black_94%,transparent)]" />
          </div>
          <div className="space-y-3 rounded-2xl bg-white/10 p-3">
            {/* John's photo opens his page: persona, presets and sales playbook. */}
            <button onClick={() => go("profile")} title="Edit how John's AI sells" className="-m-1 flex w-[calc(100%+0.5rem)] items-center gap-3 rounded-xl p-1 text-left transition hover:bg-white/10">
              <JohnAvatar size={40} src={state.ambassador.avatar} />
              <div className="min-w-0">
                <div className="truncate font-bold">John Klensch</div>
                <div className="text-xs text-white/70">U.Center ambassador</div>
              </div>
            </button>
            <div className="space-y-1.5 text-xs">
              {state.ambassador.mock_voice ? (
                // Leads would hear a tone, not John: easy to miss before a demo.
                <div
                  className="flex items-start gap-2 rounded-lg bg-amber-400/90 px-2 py-1.5 font-semibold text-amber-950"
                  title="No voice clone is set, so voice notes are a placeholder tone. Set ELEVENLABS_API_KEY and JOHN_VOICE_ID in .env and restart the server."
                >
                  <Mic className="mt-px size-3.5 shrink-0" /> Voice notes are a test tone: no voice clone set
                </div>
              ) : (
                <div className="flex items-center gap-2">
                  <Mic className="size-3.5" /> Your voice, ready
                </div>
              )}
              <WhatsAppPill wa={state.whatsapp} onLink={() => setLinkOpen(true)} />
              <AiPill health={state.brain} onCheck={() => go("profile")} />
              {!connected && <div className="text-amber">Live updates reconnecting…</div>}
            </div>
          </div>
        </aside>

        <main className="flex min-w-0 flex-1 flex-col gap-4">
          {page === "inbox" && (
            <Inbox contacts={state.contacts} last={state.last ?? {}} messages={messages} loadMessages={loadMessages} runs={runs} onCampaign={() => go("campaign")} />
          )}
          {page === "profile" && <Profile state={state} />}
          {page === "campaign" && <Campaign contacts={state.contacts} statuses={campaign} />}
          {page === "analytics" && <Analytics state={state} />}
          {page === "help" && <Help state={state} />}
        </main>
      </div>
      {linkOpen && <LinkWhatsApp wa={state.whatsapp} onClose={() => setLinkOpen(false)} />}
      <Toaster />
    </div>
  )
}

function AiPill({ health, onCheck }: { health?: BrainHealth; onCheck: () => void }) {
  if (!health || health.ok)
    return (
      <div className="flex items-center gap-2" title="Claude answered the last request">
        <Bot className="size-3.5" /> AI replies on
      </div>
    )
  return (
    // It clears on the next request that works: Try John on the John page sends one.
    <button onClick={onCheck} className="flex w-full items-start gap-2 rounded-lg bg-red-500/90 px-2 py-1.5 text-left font-semibold hover:bg-red-500" title={`${health.error}\n\nUse Try John on the John page to check again.`}>
      <Bot className="mt-px size-3.5 shrink-0" />
      <span>AI can't reply: {explainError(health.error)}</span>
    </button>
  )
}

function WhatsAppPill({ wa, onLink }: { wa: WhatsAppStatus; onLink: () => void }) {
  if (!wa.enabled)
    return (
      <div className="flex items-center gap-2" title="Started with -wa=false: use Simulate in the Inbox">
        <span className="size-2 rounded-full bg-white/40" /> WhatsApp off (simulate)
      </div>
    )
  if (wa.connected)
    return (
      <div className="flex items-center gap-2">
        <span className="size-2 rounded-full bg-wa shadow-[0_0_8px] shadow-wa" /> WhatsApp +{wa.phone}
      </div>
    )
  if (!wa.linked || wa.logged_out)
    return (
      <button onClick={onLink} className="flex w-full items-center gap-2 rounded-lg bg-white px-2 py-1.5 font-semibold text-indigo-700 hover:bg-white/90">
        <QrCode className="size-3.5" /> Link WhatsApp
      </button>
    )
  return (
    <div className="flex items-center gap-2">
      <span className="size-2 animate-pulse rounded-full bg-amber" /> WhatsApp connecting…
    </div>
  )
}
