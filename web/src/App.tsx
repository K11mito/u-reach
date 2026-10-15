import { useCallback, useEffect, useState } from "react"
import { Clock, Inbox as InboxIcon, Link2, Megaphone, MonitorPlay, Mic, TriangleAlert, UserRound } from "lucide-react"
import { cn } from "@/lib/utils"
import { useLive } from "@/lib/api"
import { Inbox } from "@/components/Inbox"
import { Stage } from "@/components/Stage"
import { Profile } from "@/components/Profile"
import { Campaign } from "@/components/Campaign"
import { JohnAvatar } from "@/components/bits"

type Page = "inbox" | "stage" | "campaign" | "profile"
const pageFromPath = (): Page => {
  const p = window.location.pathname.replace(/^\//, "")
  return (["stage", "campaign", "profile"].includes(p) ? p : "inbox") as Page
}

export default function App() {
  const { state, runs, messages, loadMessages, connected, campaign } = useLive()
  const [page, setPage] = useState<Page>(pageFromPath)
  const go = useCallback((p: Page) => {
    window.history.pushState(null, "", p === "inbox" ? "/" : `/${p}`)
    setPage(p)
  }, [])
  useEffect(() => {
    const pop = () => setPage(pageFromPath())
    window.addEventListener("popstate", pop)
    return () => window.removeEventListener("popstate", pop)
  }, [])

  if (!state) {
    return <div className="grid h-screen place-items-center text-muted-foreground">{connected ? "Loading…" : "Connecting to U-Reach…"}</div>
  }
  if (page === "stage") {
    return <Stage state={state} runs={runs} messages={messages} loadMessages={loadMessages} onExit={() => go("inbox")} />
  }

  const nav = [
    { id: "inbox" as const, label: "Inbox", icon: InboxIcon, badge: state.stats.needs_john },
    { id: "stage" as const, label: "Stage view", icon: MonitorPlay },
    { id: "campaign" as const, label: "Campaign", icon: Megaphone },
    { id: "profile" as const, label: "Profile", icon: UserRound },
  ]

  return (
    <div className="flex h-screen gap-4 bg-[radial-gradient(900px_500px_at_0%_0%,oklch(0.94_0.04_285),transparent)] p-4">
      <aside className="flex w-60 shrink-0 flex-col rounded-2xl bg-gradient-to-b from-indigo-600 to-purple-700 p-4 text-white shadow-lg shadow-indigo-500/20">
        <div className="flex items-center gap-2.5 px-2 py-1">
          <div className="grid size-9 place-items-center rounded-xl bg-white/20 text-lg font-extrabold">U</div>
          <div className="text-lg font-extrabold tracking-tight">U-Reach</div>
        </div>
        <nav className="mt-6 space-y-1">
          {nav.map((n) => (
            <button
              key={n.id}
              onClick={() => go(n.id)}
              className={cn("flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-semibold transition", page === n.id ? "bg-white text-indigo-700 shadow" : "text-white/85 hover:bg-white/10")}
            >
              <n.icon className="size-4" />
              <span className="flex-1 text-left">{n.label}</span>
              {!!n.badge && <span className="rounded-full bg-amber px-1.5 text-[11px] font-bold text-amber-950">{n.badge}</span>}
            </button>
          ))}
        </nav>
        <div className="mt-auto space-y-3 rounded-2xl bg-white/10 p-3">
          <div className="flex items-center gap-3">
            <JohnAvatar size={40} />
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
            <div className="flex items-center gap-2">
              <span className={cn("size-2 rounded-full", state.whatsapp.connected ? "bg-wa" : state.whatsapp.enabled ? "bg-amber" : "bg-white/40")} />
              {state.whatsapp.connected ? `WhatsApp +${state.whatsapp.phone}` : state.whatsapp.enabled ? "WhatsApp connecting…" : "WhatsApp off (simulate)"}
            </div>
            {!connected && <div className="text-amber">Live updates reconnecting…</div>}
          </div>
        </div>
      </aside>

      <main className="flex min-w-0 flex-1 flex-col gap-4">
        <div className="grid grid-cols-4 gap-4">
          <Stat icon={Clock} label="Hours saved this week" value={state.hours_saved.toFixed(1)} accent />
          <Stat icon={Mic} label="Voice notes this week" value={state.stats.replies_week} />
          <Stat icon={Link2} label="Referral links sent" value={state.stats.links_sent} />
          <Stat icon={TriangleAlert} label="Waiting for John" value={state.stats.needs_john} warn={state.stats.needs_john > 0} />
        </div>
        {page === "inbox" && <Inbox contacts={state.contacts} last={state.last ?? {}} messages={messages} loadMessages={loadMessages} runs={runs} />}
        {page === "profile" && <Profile state={state} />}
        {page === "campaign" && <Campaign contacts={state.contacts} statuses={campaign} />}
      </main>
    </div>
  )
}

function Stat({ icon: Icon, label, value, accent, warn }: { icon: typeof Clock; label: string; value: string | number; accent?: boolean; warn?: boolean }) {
  return (
    <div className={cn("flex items-center gap-4 rounded-2xl border bg-card p-4 shadow-sm", accent && "border-transparent bg-gradient-to-br from-wa to-wa-dark text-white")}>
      <div className={cn("grid size-11 place-items-center rounded-xl", accent ? "bg-white/20" : warn ? "bg-amber/25 text-amber-800" : "bg-secondary text-primary")}>
        <Icon className="size-5" />
      </div>
      <div>
        <div className="text-2xl font-extrabold tabular-nums">{value}</div>
        <div className={cn("text-xs", accent ? "text-white/85" : "text-muted-foreground")}>{label}</div>
      </div>
    </div>
  )
}
