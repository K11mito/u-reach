import { useCallback, useEffect, useRef, useState } from "react"

export type Status = "new" | "auto" | "needs_john" | "human"

export interface Contact {
  id: number
  jid: string
  chat: string
  phone: string
  name: string
  push_name: string
  context: string
  memory: string[] | null
  status: Status
  handoff_reason: string
  link_sent_at: number
  avatar?: string
  /** How close they are to joining, as the AI rated them on its last reply. */
  interest?: Interest
  interest_reason?: string
  interest_at?: number
  created_at: number
  last_message_at: number
}

export type Interest = "cold" | "warm" | "hot" | ""

export interface Message {
  id: number
  contact_id: number
  wa_id?: string
  direction: "in" | "out"
  kind: "text" | "voice"
  source: "lead" | "ai" | "john" | "seed"
  text: string
  audio_path?: string
  duration_ms?: number
  run_id?: number
  status?: "sent" | "delivered" | "read" | "played"
  created_at: number
}

export interface Run {
  id: number
  contact_id: number
  received_at: number
  heard_at: number
  thought_at: number
  spoken_at: number
  sent_at: number
  status: "running" | "sent" | "handoff" | "failed"
  error?: string
  model?: string
  brain?: string
}

export interface Stats {
  contacts: number
  replies_week: number
  replies_today: number
  needs_john: number
  links_sent: number
  seconds_spoken: number
}

export interface Persona {
  name: string
  short_name: string
  role: string
  background: string[]
  voice: Record<string, string>
  signature_phrases: string[]
  how_he_sells?: string
  values: string[]
  never: string[]
  sample_lines: string[]
}

export interface State {
  ambassador: {
    name: string
    voice_id: string
    tts_model: string
    brain_model: string
    referral_url: string
    persona: Persona | null
    faq: string
    mock_voice: boolean
    samples?: { original?: string; clone?: string }
    avatar?: string
    clone_line?: string
  }
  contacts: Contact[]
  last: Record<number, Message>
  stats: Stats
  hours_saved: number
  whatsapp: WhatsAppStatus
  brain?: BrainHealth
}

/** Whether the last request reached Claude. */
export interface BrainHealth {
  ok: boolean
  error?: string
  at?: number
}

export interface WhatsAppStatus {
  enabled: boolean
  connected: boolean
  linked: boolean
  phone?: string
  push_name?: string
  has_qr?: boolean
  logged_out?: boolean
}

async function json<T>(r: Response): Promise<T> {
  if (!r.ok) throw new Error(`${r.status} ${await r.text()}`)
  return r.json()
}

/** The server's message for a failed request, without the status code. */
export const errText = (e: unknown) => String(e).replace(/^Error: \d+ /, "").trim()

/** A pipeline or Claude error in plain words, for John (the raw text is for debugging). */
export function explainError(error = "") {
  const e = error.toLowerCase()
  if (e.includes("credit balance")) return "Claude's API credit has run out."
  if (/claude 40[13]|authentication|api key/.test(e)) return "Claude rejected the API key."
  if (/claude 429|rate limit/.test(e)) return "Claude is rate-limiting requests right now."
  if (/claude 5\d\d|overloaded|timeout|deadline exceeded/.test(e)) return "Claude didn't answer in time."
  if (/brain json|max_tokens|refused/.test(e)) return "Claude's answer couldn't be used."
  if (e.startsWith("brain:") || e.startsWith("claude")) return "Claude couldn't be reached."
  if (e.includes("isn't linked") || e.includes("not linked")) return "WhatsApp isn't linked."
  if (e.startsWith("send ")) return "WhatsApp didn't accept the message."
  const s = error.trim() || "Unknown error"
  return /[.!?]$/.test(s) ? s : `${s}.`
}

// John's editable harness (the John page). The profile is who John is;
// presets are how he sells; FAQ, link and playbook are shared.
export interface HarnessProfile {
  name: string
  short_name: string
  role: string
  background: string[]
  values: string[]
  voice: { tone: string; rhythm: string; english: string; brand_words: string }
  signature_phrases: string[]
  sample_lines: string[]
  metaphors: string[]
  never: string[]
  openers: string[]
  closers: string[]
}
export type LinkTiming = "first_reply" | "when_interested" | "when_asked"
export interface HarnessStyle {
  min_words: number
  max_words: number
  link_timing: LinkTiming
}
/** A preset: how John sells with it (free text) and his reply style. */
export interface Harness {
  approach: string
  style: HarnessStyle
}
export interface HarnessPreset {
  id: number
  key?: string
  name: string
  description: string
  harness: Harness
  updated_at: number
}
export interface PlaybookDoc {
  id: number
  name: string
  kind: "md" | "txt" | "pdf"
  bytes: number
  words: number
  chars: number
  active: boolean
  preview: string
  created_at: number
}
export interface HarnessSnapshot {
  profile: HarnessProfile
  active_id: number
  presets: HarnessPreset[]
  faq: string
  referral_url: string
  documents: PlaybookDoc[]
  playbook_max_chars: number
  always_handoff: string[] // at the current autonomy
  autonomy: Autonomy
  levels: AutonomyLevel[]
}
export type Autonomy = "low" | "medium" | "high"
export interface AutonomyLevel {
  key: Autonomy
  label: string
  summary: string
  ai_handles: string[]
  hands_off: string[]
}
export interface TryResult {
  reply: { spoken_script: string; followup_text: string; include_referral: boolean; handoff: boolean; handoff_reason: string; opt_out: boolean; remember: string[] }
  words: number
  trimmed: boolean
  forced_handoff: string
  model: string
  latency_ms: number
}

const send = (method: string, url: string, body?: unknown) =>
  fetch(url, { method, body: body === undefined ? undefined : JSON.stringify(body) }).then(json<HarnessSnapshot>)

export const harnessApi = {
  get: () => fetch("/api/harness").then(json<HarnessSnapshot>),
  saveProfile: (p: HarnessProfile) => send("PUT", "/api/harness/profile", p),
  savePreset: (id: number, h: Harness) => send("PUT", `/api/harness/presets/${id}`, h),
  saveAs: (name: string, description: string, harness: Harness) => send("POST", "/api/harness/presets", { name, description, harness }),
  activate: (id: number) => send("POST", `/api/harness/presets/${id}/activate`),
  reset: (id: number) => send("POST", `/api/harness/presets/${id}/reset`),
  remove: (id: number) => send("DELETE", `/api/harness/presets/${id}`),
  rename: (id: number, name: string, description: string) => send("PATCH", `/api/harness/presets/${id}`, { name, description }),
  setFAQ: (text: string) => send("PUT", "/api/harness/faq", { text }),
  setAutonomy: (level: Autonomy) => send("PUT", "/api/harness/autonomy", { level }),
  setReferral: (url: string) => send("PUT", "/api/harness/referral", { url }),
  upload: (file: File) => {
    const f = new FormData()
    f.append("file", file, file.name)
    return fetch("/api/harness/documents", { method: "POST", body: f }).then(json<HarnessSnapshot>)
  },
  docText: (id: number) => fetch(`/api/harness/documents/${id}`).then(json<{ text: string }>),
  setDocActive: (id: number, active: boolean) => send("PATCH", `/api/harness/documents/${id}`, { active }),
  removeDoc: (id: number) => send("DELETE", `/api/harness/documents/${id}`),
  /** John's reply to a pretend message; pass unsaved drafts to try them first. */
  tryIt: (name: string, message: string, draft?: { profile?: HarnessProfile; preset?: Harness }) =>
    fetch("/api/harness/try", { method: "POST", body: JSON.stringify({ name, message, ...draft }) }).then(json<TryResult>),
}

export interface FunnelDay {
  date: string // YYYY-MM-DD
  leads: number
  voice: number
  links: number
  purchased: number
}
export interface Analytics {
  days: FunnelDay[]
  week?: { leads: number } // each lead counted once over the last 7 days
  hours: { t: number; elevenlabs: number; claude: number }[] // USD per hour
  spend: {
    elevenlabs: { usd_window?: number; credits_window?: number; credits_used?: number; credits_limit?: number; usd_per_credit?: number; plan?: string; error?: string }
    claude: { usd_window?: number; usd_total?: number; calls?: number; usd_per_call?: number; tracked_since?: number; earlier_replies?: number; usd_estimated?: number; error?: string }
  }
  response: { replies?: number; avg_ms?: number }
}

export const api = {
  state: () => fetch("/api/state").then(json<State>),
  analytics: () => fetch("/api/analytics").then(json<Analytics>),
  messages: (id: number) => fetch(`/api/contacts/${id}/messages`).then(json<Message[]>),
  runs: () => fetch("/api/runs").then(json<Run[]>),
  setStatus: (id: number, status: Status) =>
    fetch(`/api/contacts/${id}/status`, { method: "POST", body: JSON.stringify({ status }) }).then(json),
  send: (id: number, text: string) =>
    fetch(`/api/contacts/${id}/send`, { method: "POST", body: JSON.stringify({ text }) }).then(json),
  simulate: (name: string, phone: string, text: string) =>
    fetch("/api/simulate", { method: "POST", body: JSON.stringify({ name, phone, text }) }).then(json),
  simulateVoice: (name: string, phone: string, audio: Blob) => {
    const f = new FormData()
    f.append("name", name)
    f.append("phone", phone)
    f.append("audio", audio, "voice-note")
    return fetch("/api/simulate", { method: "POST", body: f }).then(json)
  },
  addContact: (name: string, phone: string, context: string) =>
    fetch("/api/contacts", { method: "POST", body: JSON.stringify({ name, phone, context }) }).then(json<Contact>),
  draft: (contact_ids: number[]) =>
    fetch("/api/campaign/draft", { method: "POST", body: JSON.stringify({ contact_ids }) }).then(json<Draft[]>),
  resetChat: (id: number) => fetch(`/api/contacts/${id}/reset`, { method: "POST" }).then(json),
  resetDemo: () => fetch("/api/demo/reset", { method: "POST" }).then(json<{ leads: number; messages: number }>),
  sendCampaign: (drafts: Draft[]) => fetch("/api/campaign/send", { method: "POST", body: JSON.stringify({ drafts }) }).then(json),
}

export interface Draft {
  contact_id: number
  name: string
  script: string
  followup: string
  audio_path: string
  duration_ms: number
  error?: string
}

export type CampaignStatus = { status: "sending" | "sent" | "failed"; error?: string }

export type NudgeStatus = "draft" | "sending" | "sent" | "skipped" | "failed" | "stale"
/** A follow-up for a lead who went quiet, waiting for John's OK. */
export interface Nudge {
  id: number
  contact_id: number
  script: string
  followup: string
  link: boolean
  status: NudgeStatus
  error?: string
  audio_path?: string
  duration_ms?: number
  quiet_since: number
  created_at: number
  updated_at: number
  sent_at?: number
  name: string
  avatar?: string
  demo: boolean
  interest: Interest
  interest_reason: string
  last_lead_text: string
  link_sent: boolean
}
export interface NudgeQueue {
  items: Nudge[] | null
  waiting: number
  after_hours: number
  auto: boolean
}

export const nudgeApi = {
  queue: () => fetch("/api/nudges").then(json<NudgeQueue>),
  draft: () => fetch("/api/nudges/draft", { method: "POST" }).then(json<{ drafted: number }>),
  send: (ids: number[]) => fetch("/api/nudges/send", { method: "POST", body: JSON.stringify({ ids }) }).then(json<{ sending: number[] }>),
  edit: (id: number, script: string, followup: string) => fetch(`/api/nudges/${id}`, { method: "PUT", body: JSON.stringify({ script, followup }) }).then(json<Nudge>),
  listen: (id: number) => fetch(`/api/nudges/${id}/listen`, { method: "POST" }).then(json<Nudge>),
  redraft: (id: number) => fetch(`/api/nudges/${id}/redraft`, { method: "POST" }).then(json<Nudge>),
  skip: (id: number) => fetch(`/api/nudges/${id}/skip`, { method: "POST" }).then(json),
}

export const displayName = (c?: Contact) => (c ? c.name || c.push_name || `+${c.phone}` : "")

type LiveEvent =
  | { type: "contact"; data: Contact }
  | { type: "message"; data: Message }
  | { type: "run"; data: Run }
  | { type: "campaign"; data: CampaignStatus & { contact_id: number } }
  | { type: "whatsapp"; data: WhatsAppStatus }
  | { type: "brain"; data: BrainHealth }
  | { type: "reset"; data: null }
  | { type: "chat_reset"; data: { contact_id: number } }
  | { type: "ambassador_avatar"; data: { avatar: string } }
  | { type: "harness"; data: null }
  | { type: "nudges"; data: null }

/** Loads dashboard state and keeps it live via SSE. */
declare global {
  interface Window {
    __BOOT__?: { state: State; runs: Run[]; messages: Record<number, Message[]> } | null
  }
}

export function useLive() {
  const [state, setState] = useState<State | null>(() => window.__BOOT__?.state ?? null)
  const [runs, setRuns] = useState<Run[]>(() => window.__BOOT__?.runs ?? [])
  const [messages, setMessages] = useState<Record<number, Message[]>>(() => window.__BOOT__?.messages ?? {})
  const [connected, setConnected] = useState(false)
  const [campaign, setCampaign] = useState<Record<number, CampaignStatus>>({})
  const refetchTimer = useRef<number | undefined>(undefined)

  const refetch = useCallback(() => {
    window.clearTimeout(refetchTimer.current)
    refetchTimer.current = window.setTimeout(() => {
      api.state().then(setState).catch(() => {})
    }, 400)
  }, [])

  const loaded = useRef(new Set<number>(Object.keys(window.__BOOT__?.messages ?? {}).map(Number)))
  const loadMessages = useCallback((id: number) => {
    loaded.current.add(id)
    api.messages(id).then((m) => setMessages((prev) => ({ ...prev, [id]: m }))).catch(() => {})
  }, [])

  useEffect(() => {
    api.state().then(setState).catch(() => {})
    api.runs().then(setRuns).catch(() => {})
    let es: EventSource | null = null
    let retry: number | undefined
    let dropped = false
    const connect = () => {
      es = new EventSource("/api/events")
      es.onopen = () => {
        setConnected(true)
        if (!dropped) return
        // Events sent while we were disconnected (a server restart, a
        // Wi-Fi blip, a sleeping laptop) are gone: catch up on everything.
        dropped = false
        api.state().then(setState).catch(() => {})
        api.runs().then(setRuns).catch(() => {})
        loaded.current.forEach((id) => loadMessages(id))
        window.dispatchEvent(new Event("ureach:nudges"))
      }
      es.onerror = () => {
        dropped = true
        setConnected(false)
        es?.close()
        retry = window.setTimeout(connect, 2000)
      }
      es.onmessage = (e) => {
        const ev = JSON.parse(e.data) as LiveEvent
        if (ev.type === "message") {
          const m = ev.data
          setMessages((prev) => {
            const list = prev[m.contact_id]
            if (!list) return prev
            const i = list.findIndex((x) => x.id === m.id)
            const next = i >= 0 ? list.map((x) => (x.id === m.id ? m : x)) : [...list, m]
            return { ...prev, [m.contact_id]: next }
          })
          refetch()
        } else if (ev.type === "contact") {
          setState((s) => {
            if (!s) return s
            const exists = s.contacts.some((c) => c.id === ev.data.id)
            return {
              ...s,
              contacts: exists ? s.contacts.map((c) => (c.id === ev.data.id ? ev.data : c)) : [ev.data, ...s.contacts],
            }
          })
          refetch()
        } else if (ev.type === "nudges") {
          // The follow-up queue changed: the Campaign tab refetches it.
          window.dispatchEvent(new Event("ureach:nudges"))
        } else if (ev.type === "harness") {
          // John was edited (maybe in another tab): the John page refetches.
          window.dispatchEvent(new Event("ureach:harness"))
          refetch()
        } else if (ev.type === "ambassador_avatar") {
          setState((s) => (s ? { ...s, ambassador: { ...s.ambassador, avatar: ev.data.avatar } } : s))
        } else if (ev.type === "chat_reset") {
          // The lead is removed everywhere; a new message brings them back fresh.
          const id = ev.data.contact_id
          loaded.current.delete(id)
          setState((s) => (s ? { ...s, contacts: s.contacts.filter((c) => c.id !== id) } : s))
          setMessages((prev) => {
            const next = { ...prev }
            delete next[id]
            return next
          })
          setRuns((prev) => prev.filter((r) => r.contact_id !== id))
          window.dispatchEvent(new Event("ureach:nudges"))
          refetch()
        } else if (ev.type === "reset") {
          window.location.reload()
        } else if (ev.type === "whatsapp") {
          setState((s) => (s ? { ...s, whatsapp: ev.data } : s))
        } else if (ev.type === "brain") {
          setState((s) => (s ? { ...s, brain: ev.data } : s))
        } else if (ev.type === "campaign") {
          setCampaign((prev) => ({ ...prev, [ev.data.contact_id]: { status: ev.data.status, error: ev.data.error } }))
        } else if (ev.type === "run") {
          setRuns((prev) => {
            const i = prev.findIndex((r) => r.id === ev.data.id)
            return i >= 0 ? prev.map((r) => (r.id === ev.data.id ? ev.data : r)) : [ev.data, ...prev]
          })
        }
      }
    }
    connect()
    return () => {
      es?.close()
      window.clearTimeout(retry)
    }
  }, [refetch, loadMessages])

  return { state, runs, messages, loadMessages, connected, refetch, campaign }
}

export function timeAgo(ms: number) {
  if (!ms) return ""
  const s = Math.round((Date.now() - ms) / 1000)
  if (s < 60) return "now"
  if (s < 3600) return `${Math.floor(s / 60)}m`
  if (s < 86400) return `${Math.floor(s / 3600)}h`
  return `${Math.floor(s / 86400)}d`
}

export function clock(ms: number) {
  return new Date(ms).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
}

/** Animates a number toward its new value (for the stat counters). */
export function useCountUp(target: number, ms = 900) {
  const [value, setValue] = useState(target)
  const from = useRef(target)
  useEffect(() => {
    const start = performance.now()
    const a = from.current
    let raf = 0
    const step = (t: number) => {
      const p = Math.min(1, (t - start) / ms)
      const eased = 1 - Math.pow(1 - p, 3)
      setValue(a + (target - a) * eased)
      if (p < 1) raf = requestAnimationFrame(step)
      else from.current = target
    }
    raf = requestAnimationFrame(step)
    return () => {
      cancelAnimationFrame(raf)
      from.current = target
    }
  }, [target, ms])
  return value
}

export function dayLabel(ms: number) {
  const d = new Date(ms)
  const today = new Date()
  const yesterday = new Date(Date.now() - 86400000)
  const same = (a: Date, b: Date) => a.toDateString() === b.toDateString()
  if (same(d, today)) return "Today"
  if (same(d, yesterday)) return "Yesterday"
  return d.toLocaleDateString([], { weekday: "long", day: "numeric", month: "short" })
}
