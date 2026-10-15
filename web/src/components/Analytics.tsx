import { Clock, Link2, Mic, TriangleAlert } from "lucide-react"
import { cn } from "@/lib/utils"
import { useCountUp, type State } from "@/lib/api"
import { Grain } from "./Grain"

/** John's week at a glance. */
export function Analytics({ state }: { state: State }) {
  return (
    <div className="grid auto-rows-min grid-cols-2 gap-4 xl:grid-cols-4">
      <Stat
        icon={Clock}
        label="Hours saved this week"
        value={state.hours_saved}
        decimals={1}
        accent
        hint="Each AI voice note replaces a ~15 minute call John would have made himself."
      />
      <Stat icon={Mic} label="Voice notes this week" value={state.stats.replies_week} hint="Replies sent in John's cloned voice." />
      <Stat icon={Link2} label="Referral links sent" value={state.stats.links_sent} hint="Leads who got John's sign-up link." />
      <Stat
        icon={TriangleAlert}
        label="Waiting for John"
        value={state.stats.needs_john}
        warn={state.stats.needs_john > 0}
        hint="Chats the AI handed to John, like money or legal questions."
      />
    </div>
  )
}

function Stat({ icon: Icon, label, value, decimals = 0, accent, warn, hint }: { icon: typeof Clock; label: string; value: number; decimals?: number; accent?: boolean; warn?: boolean; hint: string }) {
  const shown = useCountUp(value)
  const body = (
    <div>
      <div className="flex items-start justify-between gap-3">
        <div className={cn("flex items-center gap-1.5 truncate text-xs font-medium", accent ? "text-white/85" : "text-muted-foreground")}>
          {warn && <span className="size-1.5 shrink-0 rounded-full bg-amber" />}
          {label}
        </div>
        <Icon className={cn("size-4 shrink-0", accent ? "text-white/80" : "text-muted-foreground/70")} />
      </div>
      <div className="mt-3 text-4xl leading-none font-semibold tracking-tight tabular-nums">{shown.toFixed(decimals)}</div>
      <p className={cn("mt-3 text-xs", accent ? "text-white/80" : "text-muted-foreground")}>{hint}</p>
    </div>
  )
  // The headline number sits on a grainy gradient; the rest stay quiet.
  if (accent)
    return (
      <Grain seed="hours-saved" palette="mint" className="rounded-xl p-5 text-white">
        {body}
      </Grain>
    )
  return <div className="rounded-xl border bg-card p-5">{body}</div>
}
