import type { ReactNode } from "react"
import { Hand, Info, Keyboard, LayoutDashboard, MessageCircle, MonitorPlay, ShieldCheck, Smartphone } from "lucide-react"
import type { State } from "@/lib/api"

/** "+66 61 738 5892" for a Thai number; other numbers just get a plus. */
const pretty = (phone: string) => (phone.length === 11 && phone.startsWith("66") ? `+66 ${phone.slice(2, 4)} ${phone.slice(4, 7)} ${phone.slice(7)}` : `+${phone}`)

/** A guide for the hackathon judges: how to message John and what to look for. */
export function Help({ state }: { state: State }) {
  const phone = state.whatsapp.phone
  return (
    <div className="min-h-0 flex-1 space-y-4 overflow-y-auto pr-1 pb-2">
      <section className="grid gap-6 rounded-xl bg-gradient-to-br from-indigo-600 to-purple-700 p-7 text-white shadow-lg shadow-indigo-500/25 lg:grid-cols-[minmax(0,1fr)_auto]">
        <div className="max-w-2xl">
          <span className="inline-flex items-center gap-1.5 rounded-full bg-white/15 px-2.5 py-1 text-xs font-semibold">For the judges</span>
          <h1 className="mt-3 text-3xl font-extrabold tracking-tight">Try U-Reach yourself</h1>
          <p className="mt-3 text-[15px] leading-relaxed text-white/85">
            U-Reach turns U.Center ambassador John Klensch into a WhatsApp assistant. When a lead messages him, his AI answers in about ten seconds with a
            personal voice note in John's own cloned voice, then sends his referral link. This dashboard is John's control room. The best way to see it is to
            become one of his leads: it takes two minutes.
          </p>
        </div>
        <div className="flex items-center gap-5 self-center rounded-xl bg-white p-4 text-foreground shadow-xl">
          {phone ? (
            <>
              <img src="/api/whatsapp/chat-qr.png" alt={`QR code to message John on WhatsApp at ${pretty(phone)}`} className="size-36 [image-rendering:pixelated]" />
              <div>
                <div className="text-xs font-medium text-muted-foreground">Scan with your phone's camera</div>
                <div className="mt-1 text-lg font-bold">Message John</div>
                <div className="text-lg font-semibold tracking-tight whitespace-nowrap tabular-nums">{pretty(phone)}</div>
                <a href={`https://wa.me/${phone}`} target="_blank" rel="noreferrer" className="mt-2 inline-block text-xs font-medium text-indigo-600 hover:underline">
                  or open wa.me/{phone}
                </a>
              </div>
            </>
          ) : (
            <div className="max-w-56 text-sm text-muted-foreground">John's WhatsApp isn't connected right now. Ask the team and we'll bring it back up.</div>
          )}
        </div>
      </section>

      <div className="grid gap-4 lg:grid-cols-2">
        <Step n={1} icon={MessageCircle} title="Message John on WhatsApp">
          Scan the code above, or save <b>{phone ? pretty(phone) : "John's number"}</b> and send him a message. Ask what a curious friend would: "What is
          U-topia?", "How do I join?", "Do I need to know crypto?"
        </Step>
        <Step n={2} icon={Smartphone} title="Get a voice note back">
          You'll see John "recording audio…", and about ten seconds later a voice note arrives in his cloned voice, using your name, followed by his link.
          Send a voice note yourself: he understands those too. Ask a follow-up and he remembers the conversation.
        </Step>
        <Step n={3} icon={MonitorPlay} title="Watch it happen here">
          Your chat jumps to the top of the <b>Inbox</b> as it arrives. Open <b>Stage view</b> (press 2) to see each step live: received, heard, thinking,
          speaking, sent, with how long each one took.
        </Step>
        <Step n={4} icon={ShieldCheck} title="Ask the hard questions">
          "How much can I earn?", "Is this a pyramid scheme?", "How many countries does it work in?" It answers from what it learned from U.Center's calls
          and websites, honestly, and never promises income. Then say your payment didn't arrive: only a person can fix that, so it hands the chat to the
          real John and it turns <b>Needs John</b> here. That's the default, <b>Balanced</b>: move the autonomy slider on the <b>John</b> page to
          Cautious and the money questions come to you too, or to Full autonomy and only emergencies do.
        </Step>
        <Step n={5} icon={Hand} title="Take over as John">
          In the Inbox, open your chat and press <b>Take over</b> or <b>Reply as John</b>. Type a message and it lands on your phone from John's number.
          Press <b>Hand back to AI</b> and the AI picks the conversation up again.
        </Step>
        <Step n={6} icon={LayoutDashboard} title="Explore the rest">
          <b>Campaign</b>: pick leads and the AI writes each a personal voice-note opener for John to approve. <b>Analytics</b>: the funnel, and what it
          really costs to run. <b>Profile</b>: John's persona, sales presets, knowledge base and playbook, with a box to try replies before they go out.
        </Step>
      </div>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <Note icon={Info} title="Good to know">
          <li>This is a hackathon proof of concept. The other leads in the Inbox are made-up demo data; yours is real.</li>
          <li>Your messages go through Claude (Anthropic) to write the reply and ElevenLabs for John's voice. Your chat shows on this dashboard: ask us and we'll delete it.</li>
          <li>If several judges message at once, give it a few more seconds.</li>
          <li>Please leave <b>Reset demo</b> on the John page to us: it clears every chat, yours included.</li>
        </Note>
        <Note icon={Keyboard} title="Shortcuts">
          <li>
            <Kbd>1</Kbd>–<Kbd>6</Kbd> switch pages, <Kbd>P</Kbd> presents the Stage view
          </li>
          <li>
            <Kbd>/</Kbd> searches the Inbox
          </li>
          <li>
            On the Stage view, <Kbd>F</Kbd> goes fullscreen, <Kbd>Ctrl</Kbd> + <Kbd>+</Kbd> / <Kbd>−</Kbd> zooms, and <Kbd>Esc</Kbd> goes back
          </li>
        </Note>
      </div>
    </div>
  )
}

function Step({ n, icon: Icon, title, children }: { n: number; icon: typeof Info; title: string; children: ReactNode }) {
  return (
    <section className="flex gap-4 rounded-xl border bg-card p-5 shadow-sm">
      <div className="relative shrink-0">
        <div className="grid size-11 place-items-center rounded-xl bg-gradient-to-br from-indigo-600 to-purple-700 text-white shadow-md shadow-indigo-500/25">
          <Icon className="size-5" />
        </div>
        <span className="absolute -top-1.5 -right-1.5 grid size-5 place-items-center rounded-full bg-card text-[11px] font-bold text-indigo-700 ring-1 ring-border">{n}</span>
      </div>
      <div className="min-w-0">
        <h2 className="text-[15px] font-semibold tracking-tight">{title}</h2>
        <p className="mt-1 text-sm leading-relaxed text-muted-foreground [&_b]:font-semibold [&_b]:text-foreground">{children}</p>
      </div>
    </section>
  )
}

function Note({ icon: Icon, title, children }: { icon: typeof Info; title: string; children: ReactNode }) {
  return (
    <section className="rounded-xl border bg-card p-5 shadow-sm">
      <h2 className="flex items-center gap-2 text-[15px] font-semibold tracking-tight">
        <Icon className="size-4 text-indigo-600" /> {title}
      </h2>
      <ul className="mt-2 list-disc space-y-1.5 pl-5 text-sm leading-relaxed text-muted-foreground marker:text-indigo-300 [&_b]:font-semibold [&_b]:text-foreground">{children}</ul>
    </section>
  )
}

function Kbd({ children }: { children: ReactNode }) {
  return <kbd className="rounded border bg-muted px-1.5 py-0.5 font-mono text-[11px] text-foreground">{children}</kbd>
}
