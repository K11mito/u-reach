import logo from "@/assets/logo.webp"
import banner from "@/assets/header-banner.webp"

/**
 * The full-width brand bar on every page. The brand sits over the nav column,
 * the motto over the content, and the banner art fades in on the right.
 */
export function BrandHeader() {
  return (
    <header className="relative z-10 flex h-16 shrink-0 items-center overflow-hidden rounded-2xl bg-gradient-to-r from-[oklch(0.366_0.24_277)] to-[oklch(0.35_0.218_304)] text-white shadow-lg shadow-indigo-950/30">
      <div className="relative z-10 flex w-[clamp(220px,18vw,300px)] shrink-0 items-center gap-2.5 px-5">
        <img src={logo} alt="U-Reach" className="size-9 shrink-0" />
        <div className="min-w-0">
          <div className="text-lg leading-tight font-extrabold tracking-tight">U-Reach</div>
          <div className="truncate text-[11px] text-white/65">for U.Center ambassadors</div>
        </div>
      </div>
      <div className="relative z-10 ml-4 border-l border-white/15 pl-6 text-xl font-extrabold tracking-tight">Let's Go! Let's Grow!</div>
      <img src={banner} alt="" className="absolute inset-y-0 right-0 h-full w-auto max-w-[70%] object-cover object-right opacity-80 [mask-image:linear-gradient(to_right,transparent,black_40%)]" />
    </header>
  )
}
