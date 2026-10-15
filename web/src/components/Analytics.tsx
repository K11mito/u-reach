import { useEffect, useState, type ReactNode } from "react"
import { ArrowDownRight, ArrowUpRight, AudioLines, Clock, Link2, Loader2, PartyPopper, Timer } from "lucide-react"
import { cn } from "@/lib/utils"
import { api, useCountUp, type Analytics as Data, type State } from "@/lib/api"
import { LineChart, Sparkline } from "./Chart"

const C = { leads: "#6366f1", voice: "#a855f7", links: "#0ea5e9", joined: "#10b981", eleven: "#7c3aed", claude: "#d97757" }

const day = (d: string, long = false) =>
  new Date(`${d}T12:00:00`).toLocaleDateString("en-US", long ? { weekday: "short", month: "short", day: "numeric" } : { month: "short", day: "numeric" })
const hour = (t: number, long = false) => new Date(t).toLocaleString("en-US", long ? { weekday: "short", hour: "numeric", minute: "2-digit" } : { hour: "numeric" })
const usd = (v: number) => (v >= 0.01 ? `$${v.toFixed(2)}` : v > 0 ? `$${v.toFixed(3)}` : "$0")
const sum = (xs: number[]) => xs.reduce((a, b) => a + b, 0)
const running = (xs: number[]) => xs.map((_, i) => sum(xs.slice(0, i + 1)))

// The last numbers fetched, kept across visits: the page shows them at once
// and refreshes quietly in the background instead of flashing a spinner.
let cached: Data | null = null
let inflight: Promise<Data> | null = null
function fetchAnalytics() {
  inflight ??= api
    .analytics()
    .then((d) => (cached = d))
    .finally(() => (inflight = null))
  return inflight
}

/** Warms the cache so the first visit to the page is instant too. */
export function prefetchAnalytics() {
  fetchAnalytics().catch(() => {})
}

/** John's last two weeks: the funnel, and what U-Reach costs to run. */
export function Analytics({ state }: { state: State }) {
  const [data, setData] = useState<Data | null>(cached)
  const [error, setError] = useState("")

  // Spend is live, so keep it fresh while the page is open.
  useEffect(() => {
    let alive = true
    const load = () =>
      fetchAnalytics()
        .then((d) => alive && (setData(d), setError("")))
        .catch((e) => alive && setError(String(e)))
    load()
    const t = window.setInterval(load, 30_000)
    return () => {
      alive = false
      window.clearInterval(t)
    }
  }, [state.stats.replies_week])

  if (!data)
    return (
      <div className="grid flex-1 place-items-center text-sm text-muted-foreground">
        {error ? `Couldn't load analytics: ${error}` : <Loader2 className="size-5 animate-spin" />}
      </div>
    )

  const days = data.days
  const week = days.slice(7)
  const prev = days.slice(0, 7)
  const total = (k: "leads" | "voice" | "links" | "purchased", ds = week) => sum(ds.map((d) => d[k]))
  const change = (k: "leads" | "voice" | "links" | "purchased") => {
    const before = total(k, prev)
    return before ? (total(k) - before) / before : 0
  }

  return (
    <div className="min-h-0 flex-1 space-y-4 overflow-y-auto pr-1 pb-2">
      <div className="grid grid-cols-2 gap-4 xl:grid-cols-4">
        <Kpi hero icon={Clock} label="Hours saved this week" value={state.hours_saved} decimals={1} delta={change("voice")} trend={days.map((d) => d.voice / 4)} />
        <Kpi icon={AudioLines} label="Voice notes this week" value={state.stats.replies_week} delta={change("voice")} trend={days.map((d) => d.voice)} color={C.voice} />
        <Kpi icon={Link2} label="Referral links sent" value={total("links")} delta={change("links")} trend={days.map((d) => d.links)} color={C.links} />
        <Kpi icon={PartyPopper} label="Leads who joined" value={total("purchased")} delta={change("purchased")} trend={days.map((d) => d.purchased)} color={C.joined} />
      </div>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_340px]">
        <Card title="Your funnel" hint="Last 14 days, every lead John's AI talked to">
          <div className="mb-3 flex flex-wrap gap-x-5 gap-y-1">
            <Legend color={C.leads} label="Leads reached" value={total("leads")} />
            <Legend color={C.voice} label="Voice notes" value={total("voice")} />
            <Legend color={C.links} label="Links sent" value={total("links")} />
            <Legend color={C.joined} label="Joined" value={total("purchased")} />
          </div>
          <LineChart
            labels={days.map((d) => d.date)}
            height={260}
            tick={(d, i) => day(d, i < 0)}
            series={[
              { name: "Leads reached", color: C.leads, values: days.map((d) => d.leads), area: true },
              { name: "Voice notes", color: C.voice, values: days.map((d) => d.voice), area: true },
              { name: "Links sent", color: C.links, values: days.map((d) => d.links) },
              { name: "Joined", color: C.joined, values: days.map((d) => d.purchased) },
            ]}
          />
        </Card>

        <div className="space-y-4">
          <Card title="Conversion this week" hint="From first message to membership">
            <Funnel
              steps={[
                { label: "Leads reached", value: total("leads") },
                { label: "Got the link", value: total("links") },
                { label: "Joined", value: total("purchased") },
              ]}
            />
          </Card>
          <Card title="Reply time" hint="Message in to voice note out, measured live">
            {data.response.avg_ms ? (
              <div className="flex items-end gap-3">
                <div className="grid size-11 place-items-center rounded-xl bg-gradient-to-br from-indigo-600 to-purple-700 text-white shadow-md shadow-indigo-500/25">
                  <Timer className="size-5" />
                </div>
                <div>
                  <div className="text-3xl leading-none font-extrabold tracking-tight tabular-nums">{(data.response.avg_ms / 1000).toFixed(1)}s</div>
                  <div className="mt-1 text-xs text-muted-foreground">
                    average over {data.response.replies} real {data.response.replies === 1 ? "reply" : "replies"}
                  </div>
                </div>
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">Timed from the first real reply.</p>
            )}
          </Card>
        </div>
      </div>

      <Spend data={data} />
    </div>
  )
}

function Card({ title, hint, right, children, className }: { title: string; hint?: string; right?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={cn("rounded-xl border bg-card p-5 shadow-sm", className)}>
      <div className="mb-4 flex items-start justify-between gap-3">
        <div>
          <h2 className="text-[15px] font-semibold tracking-tight">{title}</h2>
          {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
        </div>
        {right}
      </div>
      {children}
    </section>
  )
}

function Delta({ value, light }: { value: number; light?: boolean }) {
  if (!Number.isFinite(value) || value === 0) return null
  const up = value > 0
  const Icon = up ? ArrowUpRight : ArrowDownRight
  return (
    <span
      className={cn(
        "inline-flex items-center gap-0.5 rounded-full px-1.5 py-0.5 text-[11px] font-semibold tabular-nums",
        light ? "bg-white/20 text-white" : up ? "bg-emerald-50 text-emerald-700" : "bg-red-50 text-red-700",
      )}
    >
      <Icon className="size-3" />
      {Math.round(Math.abs(value) * 100)}%
    </span>
  )
}

function Kpi({
  icon: Icon,
  label,
  value,
  decimals = 0,
  delta,
  trend,
  color,
  hero,
}: {
  icon: typeof Clock
  label: string
  value: number
  decimals?: number
  delta: number
  trend: number[]
  color?: string
  hero?: boolean
}) {
  const shown = useCountUp(value)
  return (
    <div
      className={cn(
        "flex flex-col rounded-xl p-5",
        hero ? "bg-gradient-to-br from-indigo-600 to-purple-700 text-white shadow-lg shadow-indigo-500/25" : "border bg-card shadow-sm",
      )}
    >
      <div className="flex items-center justify-between gap-2">
        <span className={cn("text-xs font-medium", hero ? "text-white/80" : "text-muted-foreground")}>{label}</span>
        <Icon className={cn("size-4", hero ? "text-white/80" : "text-muted-foreground/70")} />
      </div>
      <div className="mt-2 flex items-baseline gap-2">
        <span className="text-3xl font-extrabold tracking-tight tabular-nums">{shown.toFixed(decimals)}</span>
        <Delta value={delta} light={hero} />
      </div>
      <span className={cn("text-[11px]", hero ? "text-white/65" : "text-muted-foreground")}>vs the week before</span>
      <Sparkline values={trend} color={hero ? "#ffffff" : color} className="mt-3" />
    </div>
  )
}

function Legend({ color, label, value }: { color: string; label: string; value: number }) {
  return (
    <div className="flex items-center gap-2 text-xs">
      <span className="size-2.5 rounded-full" style={{ background: color }} />
      <span className="text-muted-foreground">{label}</span>
      <span className="font-semibold tabular-nums">{value}</span>
      <span className="text-muted-foreground/70">this week</span>
    </div>
  )
}

function Funnel({ steps }: { steps: { label: string; value: number }[] }) {
  const top = Math.max(1, steps[0]?.value ?? 1)
  return (
    <div className="space-y-3">
      {steps.map((s, i) => {
        const rate = i > 0 && steps[i - 1].value ? s.value / steps[i - 1].value : null
        return (
          <div key={s.label}>
            <div className="mb-1 flex items-baseline justify-between text-xs">
              <span className="font-medium">{s.label}</span>
              <span className="text-muted-foreground tabular-nums">
                <b className="text-sm font-semibold text-foreground">{s.value}</b>
                {rate !== null && ` · ${Math.round(rate * 100)}%`}
              </span>
            </div>
            <div className="h-2.5 overflow-hidden rounded-full bg-muted">
              <div className="h-full rounded-full bg-gradient-to-r from-indigo-500 to-purple-500" style={{ width: `${Math.max(3, (s.value / top) * 100)}%` }} />
            </div>
          </div>
        )
      })}
    </div>
  )
}

/** Real spend: ElevenLabs' own hourly usage, and every Claude call priced as it happens. */
function Spend({ data }: { data: Data }) {
  const { elevenlabs: el, claude } = data.spend
  const hours = data.hours
  const elCum = running(hours.map((h) => h.elevenlabs))
  const clCum = running(hours.map((h) => h.claude))
  const since = claude.tracked_since ? new Date(claude.tracked_since).toLocaleString("en-US", { weekday: "short", hour: "numeric", minute: "2-digit" }) : ""
  return (
    <Card
      title="What U-Reach costs to run"
      hint="Real API spend over the last 24 hours"
      right={
        <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-50 px-2 py-0.5 text-[11px] font-semibold text-emerald-700 ring-1 ring-emerald-200">
          <span className="relative flex size-1.5">
            <span className="absolute inset-0 animate-ping rounded-full bg-emerald-500 opacity-60" />
            <span className="size-1.5 rounded-full bg-emerald-500" />
          </span>
          Live
        </span>
      }
    >
      <div className="grid gap-6 lg:grid-cols-[260px_minmax(0,1fr)]">
        <div className="space-y-4">
          <div className="rounded-xl bg-gradient-to-br from-indigo-600 to-purple-700 p-4 text-white shadow-md shadow-indigo-500/25">
            <div className="text-xs text-white/80">Total, last 24 hours</div>
            <div className="mt-1 text-3xl font-extrabold tracking-tight tabular-nums">{usd((el.usd_window ?? 0) + (claude.usd_window ?? 0))}</div>
          </div>
          <Provider color={C.eleven} name="ElevenLabs" sub="John's cloned voice" value={el.error ? "—" : usd(el.usd_window ?? 0)}>
            {el.error
              ? "Usage unavailable right now."
              : `${Math.round(el.credits_used ?? 0).toLocaleString()} of ${(el.credits_limit ?? 0).toLocaleString()} credits used this month, at ${usd((el.usd_per_credit ?? 0) * 1000)} per 1,000.`}
          </Provider>
          <Provider color={C.claude} name="Claude" sub="Writes each reply" value={usd(claude.usd_window ?? 0)}>
            <ClaudeNote c={claude} since={since} />
          </Provider>
        </div>
        <div>
          <div className="mb-2 flex gap-5">
            <Legend2 color={C.eleven} label="ElevenLabs" />
            <Legend2 color={C.claude} label="Claude" />
          </div>
          <LineChart
            labels={hours.map((h) => String(h.t))}
            height={230}
            ticks={8}
            format={(v) => (v === 0 ? "$0" : v < 0.1 ? `$${v.toFixed(3)}` : `$${v.toFixed(2)}`)}
            tick={(t, i) => hour(Number(t), i < 0)}
            series={[
              { name: "ElevenLabs", color: C.eleven, values: elCum, area: true },
              { name: "Claude", color: C.claude, values: clCum, area: true },
            ]}
          />
          <p className="mt-2 text-[11px] text-muted-foreground">Running totals. ElevenLabs reports credits per hour; Claude calls are priced from their token counts at Anthropic's list prices.</p>
        </div>
      </div>
    </Card>
  )
}

/** Says exactly what the Claude number covers: measured calls, and estimates for replies from before tracking. */
function ClaudeNote({ c, since }: { c: Data["spend"]["claude"]; since: string }) {
  const earlier = c.earlier_replies ?? 0
  const replies = (n: number) => `${n} earlier ${n === 1 ? "reply" : "replies"}`
  if (c.calls)
    return (
      <>
        {c.calls} {c.calls === 1 ? "call" : "calls"} measured since {since}, about {usd(c.usd_per_call ?? 0)} each.
        {earlier > 0 && ` Includes ${replies(earlier)} from before then, estimated at that rate.`}
      </>
    )
  if (earlier > 0) return <>Measured per call since {since}. {replies(earlier)} from before then get estimated once the first call is measured.</>
  return <>Measured per call from its token usage at Anthropic's prices.</>
}

function Provider({ color, name, sub, value, children }: { color: string; name: string; sub: string; value: string; children: ReactNode }) {
  return (
    <div className="border-l-2 pl-3" style={{ borderColor: color }}>
      <div className="flex items-baseline justify-between gap-2">
        <div>
          <span className="text-sm font-semibold">{name}</span> <span className="text-xs text-muted-foreground">{sub}</span>
        </div>
        <span className="text-sm font-semibold tabular-nums">{value}</span>
      </div>
      <p className="mt-0.5 text-xs text-muted-foreground">{children}</p>
    </div>
  )
}

function Legend2({ color, label }: { color: string; label: string }) {
  return (
    <div className="flex items-center gap-2 text-xs text-muted-foreground">
      <span className="size-2.5 rounded-full" style={{ background: color }} />
      {label}
    </div>
  )
}
