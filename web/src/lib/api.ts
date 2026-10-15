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
  created_at: number
  last_message_at: number
}

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
  how_he_sells?: string[]
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

// John's editable harness (the John page).
export interface HarnessPersona {
  name: string
  short_name: string
  role: string
  background: string[]
  voice: { tone: string; rhythm: string; english: string; brand_words: string }
  openers: string[]
  signature_phrases: string[]
  metaphors: string[]
  values: string[]
  closers: string[]
  never: string[]
  sample_lines: string[]
  how_he_sells: string[]
}
export type LinkTiming = "first_reply" | "when_interested" | "when_asked"
export interface HarnessStyle {
  min_words: number
  max_words: number
  link_timing: LinkTiming
  language: "english" | "match"
}
export interface Harness {
  persona: HarnessPersona
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
  active_id: number
  presets: HarnessPreset[]
  faq: string
  referral_url: string
  documents: PlaybookDoc[]
  playbook_max_chars: number
  always_handoff: string[]
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
  saveActive: (h: Harness) => send("PUT", "/api/harness/active", h),
  saveAs: (name: string, description: string, harness: Harness) => send("POST", "/api/harness/presets", { name, description, harness }),
  activate: (id: number) => send("POST", `/api/harness/presets/${id}/activate`),
  reset: (id: number) => send("POST", `/api/harness/presets/${id}/reset`),
  remove: (id: number) => send("DELETE", `/api/harness/presets/${id}`),
  rename: (id: number, name: string, description: string) => send("PATCH", `/api/harness/presets/${id}`, { name, description }),
  setFAQ: (text: string) => send("PUT", "/api/harness/faq", { text }),
  setReferral: (url: string) => send("PUT", "/api/harness/referral", { url }),
  upload: (file: File) => {
    const f = new FormData()
    f.append("file", file, file.name)
    return fetch("/api/harness/documents", { method: "POST", body: f }).then(json<HarnessSnapshot>)
  },
  docText: (id: number) => fetch(`/api/harness/documents/${id}`).then(json<{ text: string }>),
  setDocActive: (id: number, active: boolean) => send("PATCH", `/api/harness/documents/${id}`, { active }),
  removeDoc: (id: number) => send("DELETE", `/api/harness/documents/${id}`),
  tryIt: (name: string, message: string) =>
    fetch("/api/harness/try", { method: "POST", body: JSON.stringify({ name, message }) }).then(json<TryResult>),
}

export const api = {
  state: () => fetch("/api/state").then(json<State>),
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

export const displayName = (c?: Contact) => (c ? c.name || c.push_name || `+${c.phone}` : "")

type LiveEvent =
  | { type: "contact"; data: Contact }
  | { type: "message"; data: Message }
  | { type: "run"; data: Run }
  | { type: "campaign"; data: CampaignStatus & { contact_id: number } }
  | { type: "whatsapp"; data: WhatsAppStatus }
  | { type: "reset"; data: null }
  | { type: "chat_reset"; data: { contact_id: number } }
  | { type: "ambassador_avatar"; data: { avatar: string } }
  | { type: "harness"; data: null }

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

  const loadMessages = useCallback((id: number) => {
    api.messages(id).then((m) => setMessages((prev) => ({ ...prev, [id]: m }))).catch(() => {})
  }, [])

  useEffect(() => {
    api.state().then(setState).catch(() => {})
    api.runs().then(setRuns).catch(() => {})
    let es: EventSource | null = null
    let retry: number | undefined
    const connect = () => {
      es = new EventSource("/api/events")
      es.onopen = () => setConnected(true)
      es.onerror = () => {
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
        } else if (ev.type === "harness") {
          // John was edited (maybe in another tab): the John page refetches.
          window.dispatchEvent(new Event("ureach:harness"))
          refetch()
        } else if (ev.type === "ambassador_avatar") {
          setState((s) => (s ? { ...s, ambassador: { ...s.ambassador, avatar: ev.data.avatar } } : s))
        } else if (ev.type === "chat_reset") {
          setMessages((prev) => ({ ...prev, [ev.data.contact_id]: [] }))
          setRuns((prev) => prev.filter((r) => r.contact_id !== ev.data.contact_id))
          refetch()
        } else if (ev.type === "reset") {
          window.location.reload()
        } else if (ev.type === "whatsapp") {
          setState((s) => (s ? { ...s, whatsapp: ev.data } : s))
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
  }, [refetch])

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
