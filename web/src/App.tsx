import { useCallback, useEffect, useState } from "react"
import { Clock, Inbox as InboxIcon, Link2, Megaphone, MonitorPlay, Mic, QrCode, TriangleAlert, UserRound } from "lucide-react"
import { cn } from "@/lib/utils"
import { useCountUp, useLive, type WhatsAppStatus } from "@/lib/api"
import { Inbox } from "@/components/Inbox"
import { Stage } from "@/components/Stage"
import { Profile } from "@/components/Profile"
import { Campaign } from "@/components/Campaign"
import { LinkWhatsApp } from "@/components/LinkWhatsApp"
import { JohnAvatar } from "@/components/bits"
import { Toaster, toast } from "@/components/Toast"

type Page = "inbox" | "stage" | "campaign" | "profile"
const TITLES: Record<Page, string> = { inbox: "Inbox", stage: "Stage", campaign: "Campaign", profile: "Profile" }
const pageFromPath = (): Page => {
  const p = window.location.pathname.replace(/^\//, "")
  return (["stage", "campaign", "profile"].includes(p) ? p : "inbox") as Page
}

export default function App() {
  const { state, runs, messages, loadMessages, connected, campaign } = useLive()
  const [page, setPage] = useState<Page>(pageFromPath)
  const [linkOpen, setLinkOpen] = useState(false)
  const go = useCallback((p: Page) => {
    window.history.pushState(null, "", p === "inbox" ? "/" : `/${p}`)
    setPage(p)
  }, [])
  useEffect(() => {
    const pop = () => setPage(pageFromPath())
    window.addEventListener("popstate", pop)
    return () => window.removeEventListener("popstate", pop)
  }, [])
  useEffect(() => {
    document.title = `U-Reach · ${TITLES[page]}`
  }, [page])

  // Number keys switch pages; P presents (Stage view). Ignored while typing.
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return
      const map: Record<string, Page> = { "1": "inbox", "2": "stage", "3": "campaign", "4": "profile", p: "stage" }
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

  if (!state) {
    return (
      <div className="grid h-screen place-items-center text-muted-foreground">
        <div className="flex items-center gap-3">
          <div className="grid size-9 animate-pulse place-items-center rounded-xl bg-gradient-to-br from-indigo-500 to-purple-600 font-extrabold text-white">U</div>
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
    { id: "profile" as const, label: "Profile", icon: UserRound },
  ]

  return (
    <div className="flex h-screen min-w-[1100px] gap-4 bg-[radial-gradient(900px_500px_at_0%_0%,oklch(0.94_0.04_285),transparent)] p-4">
      <aside className="flex w-56 shrink-0 flex-col rounded-2xl bg-gradient-to-b from-indigo-600 to-purple-700 p-4 text-white shadow-lg shadow-indigo-500/20 xl:w-60">
        <div className="flex items-center gap-2.5 px-2 py-1">
          <div className="grid size-9 place-items-center rounded-xl bg-white/20 text-lg font-extrabold">U</div>
          <div>
            <div className="text-lg leading-tight font-extrabold tracking-tight">U-Reach</div>
            <div className="text-[11px] whitespace-nowrap text-white/65">for U.Center ambassadors</div>
          </div>
        </div>
        <nav className="mt-6 space-y-1">
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
              {!!n.badge && <span className="rounded-full bg-amber px-1.5 text-[11px] font-bold text-amber-950">{n.badge}</span>}
            </button>
          ))}
        </nav>
        <div className="mt-auto space-y-3 rounded-2xl bg-white/10 p-3">
          <div className="flex items-center gap-3">
            <JohnAvatar size={40} src={state.ambassador.avatar} />
            <div className="min-w-0">
              <div className="truncate font-bold">John Klensch</div>
              <div className="text-xs text-white/70">U.Center ambassador</div>
            </div>
          </div>
          <div className="space-y-1.5 text-xs">
            <div className="flex items-center gap-2">
              <Mic className="size-3.5" />
              {state.ambassador.mock_voice ? "Voice: mock (no clone set)" : "Your voice, ready"}
            </div>
            <WhatsAppPill wa={state.whatsapp} onLink={() => setLinkOpen(true)} />
            {!connected && <div className="text-amber">Live updates reconnecting…</div>}
          </div>
        </div>
      </aside>

      <main className="flex min-w-0 flex-1 flex-col gap-4">
        <div className="grid grid-cols-4 gap-4">
          <Stat icon={Clock} label="Hours saved this week" value={state.hours_saved} decimals={1} accent hint="Each AI voice note replaces a ~15 minute call John would have made himself" />
          <Stat icon={Mic} label="Voice notes this week" value={state.stats.replies_week} />
          <Stat icon={Link2} label="Referral links sent" value={state.stats.links_sent} />
          <Stat icon={TriangleAlert} label="Waiting for John" value={state.stats.needs_john} warn={state.stats.needs_john > 0} />
        </div>
        {page === "inbox" && (
          <Inbox contacts={state.contacts} last={state.last ?? {}} messages={messages} loadMessages={loadMessages} runs={runs} onCampaign={() => go("campaign")} />
        )}
        {page === "profile" && <Profile state={state} />}
        {page === "campaign" && <Campaign contacts={state.contacts} statuses={campaign} />}
      </main>
      {linkOpen && <LinkWhatsApp wa={state.whatsapp} onClose={() => setLinkOpen(false)} />}
      <Toaster />
    </div>
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

function Stat({ icon: Icon, label, value, decimals = 0, accent, warn, hint }: { icon: typeof Clock; label: string; value: number; decimals?: number; accent?: boolean; warn?: boolean; hint?: string }) {
  const shown = useCountUp(value)
  return (
    <div title={hint} className={cn("flex items-center gap-4 rounded-2xl border bg-card p-4 shadow-sm", accent && "border-transparent bg-gradient-to-br from-wa to-wa-dark text-white")}>
      <div className={cn("grid size-11 shrink-0 place-items-center rounded-xl", accent ? "bg-white/20" : warn ? "bg-amber/25 text-amber-800" : "bg-secondary text-primary")}>
        <Icon className="size-5" />
      </div>
      <div className="min-w-0">
        <div className="text-2xl font-extrabold tabular-nums">{shown.toFixed(decimals)}</div>
        <div className={cn("truncate text-xs", accent ? "text-white/85" : "text-muted-foreground")}>{label}</div>
      </div>
    </div>
  )
}
