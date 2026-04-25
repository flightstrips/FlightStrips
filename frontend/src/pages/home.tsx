import {
  ArrowRight,
  ChevronDown,
  GitBranch,
  Layers,
  MapPin,
  MessageSquare,
  Radio,
  Smartphone,
} from "lucide-react";
import { Link } from "react-router";
import { PublicNavigation } from "@/components/public/PublicNavigation";
import { PublicFooter } from "@/components/public/PublicFooter";
import { CornerDots } from "@/components/public/CornerDots";
import type { LucideIcon } from "lucide-react";
import { PUBLIC_NAV_INDUSTRY_CLASS, PUBLIC_SECTION_BORDER } from "@/lib/public-page-style";
import { cn } from "@/lib/utils";

const stroke = 1.5;

const HERO_BG =
  "https://i.imgur.com/NRmM3BP.jpeg";

const heroNavCards = [
  {
    title: "Documentation",
    index: "01",
    to: "https://docs.flightstrips.dk",
    external: true
  },
  {
    title: "About",
    index: "02",
    to: "/about",
    external: false
  },
] as const;

const pillars = [
  {
    title: "Instantly syncronized",
    description:
      "All positions are instantly synchronized, ensuring that everyone is on the same page.",
    muted: false,
    icon: Layers,
  },
  {
    title: "Datalink Clearance",
    description:
      "Offload controller workload with automated Datalink Clearance and routing.",
    muted: true,
    icon: Radio,
  },
  {
    title: "Advanced Sequencing",
    description:
      "Handle high volume of traffic with ease using advanced sequencing features.",
    muted: true,
    icon: GitBranch,
  },
];

const capabilities = [
  {
    title: "Clearances",
    description:
      "PDC-style clearances and strip-driven state so everyone sees the same picture at the same time.",
    icon: Radio,
    visual: "bars" as const,
  },
  {
    title: "Ground movement",
    description:
      "Pushback releases, holding points, and runway assignments stay on the strip—no parallel spreadsheets.",
    icon: MapPin,
    visual: "lines" as const,
  },
  {
    title: "Collaboration",
    description:
      "Internal messaging and presence across positions, with Euroscope sync when you need radar alignment.",
    icon: MessageSquare,
    visual: "lines" as const,
  },
];

const moreFeatures = [
  {
    title: "Any device",
    description: "Desktop or tablet - touch-first where it matters.",
    icon: Smartphone,
  },
  {
    title: "Euroscope",
    description: "Two-way integration. Automatic updates & a light footprint.",
    icon: Layers,
  },
];

function CapabilityVisual({ variant }: { variant: "bars" | "lines" }) {
  if (variant === "bars") {
    const heights = [0.38, 0.58, 0.32, 0.72, 0.45];
    return (
      <div
        className={cn(
          "mb-8 flex h-24 items-end gap-1 rounded-sm border bg-[#0a1628] p-3 dark:border-white/10 dark:bg-[#050810]",
          PUBLIC_SECTION_BORDER,
        )}
      >
        {heights.map((h, i) => (
          <div
            key={i}
            className="flex-1 rounded-t-[2px] bg-gradient-to-t from-[#1a3352] to-[#003d48]/75 dark:from-[#243d5c] dark:to-[#003d48]/80"
            style={{ height: `${h * 100}%` }}
          />
        ))}
      </div>
    );
  }
  return (
    <div
      className={cn(
        "mb-8 flex h-24 flex-col justify-center gap-2.5 rounded-sm border border-dashed bg-neutral-100/90 px-4 dark:border-white/12 dark:bg-white/[0.04]",
        PUBLIC_SECTION_BORDER,
      )}
    >
      {[72, 48, 88, 56].map((w, i) => (
        <div
          key={i}
          className="h-px rounded-full bg-neutral-400/50 dark:bg-white/20"
          style={{ width: `${w}%` }}
        />
      ))}
    </div>
  );
}

function HeroNavCard({
  title,
  indexLabel,
  to,
  external
}: {
  title: string;
  indexLabel: string;
  to: string;
  external: boolean;
}) {
  const className =
    "group relative flex min-h-[220px] w-[320px] flex-col overflow-hidden bg-white text-left shadow-[0_12px_40px_rgba(0,0,0,0.25)] transition-shadow hover:shadow-[0_16px_48px_rgba(0,0,0,0.35)]";

  const inner = (
    <>
      <div className="flex flex-1 flex-col px-6 pb-5 pt-6">
        <h3 className="font-display text-xl font-bold tracking-tight text-neutral-900">{title}</h3>
        <p className="mt-1 font-mono text-sm tabular-nums text-neutral-400">{indexLabel}</p>
        <div className="min-h-[2.5rem] flex-1" />
        <div className="flex justify-end">
          <span
            className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-neutral-900 text-white transition group-hover:bg-[#003d48]"
            aria-hidden
          >
            <ArrowRight className="h-4 w-4" strokeWidth={stroke} />
          </span>
        </div>
      </div>
      <div className="relative h-11 w-full overflow-hidden">
        <div className="absolute inset-0 bg-gradient-to-t from-black/20 to-transparent" aria-hidden />
      </div>
    </>
  );

  if (external) {
    return (
      <a href={to} target="_blank" rel="noopener noreferrer" className={className}>
        {inner}
      </a>
    );
  }
  return (
    <Link to={to} className={className}>
      {inner}
    </Link>
  );
}

function IconFoot({ icon: Icon, className }: { icon: LucideIcon; className?: string }) {
  return (
    <Icon
      className={cn("mt-auto size-5 shrink-0 text-neutral-500 dark:text-neutral-500", className)}
      strokeWidth={stroke}
      aria-hidden
    />
  );
}

export default function Home() {
  return (
    <div className="home-industrial flex min-h-screen flex-col">
      <PublicNavigation linkTone="industrial" className={PUBLIC_NAV_INDUSTRY_CLASS} />

      <main className="flex-1 pt-[4.5rem]">
        {/* Hero — full-bleed image, headline, bottom-left cards + scroll control */}
        <section className="relative border-b border-white/10">
          <div className="absolute inset-0">
            <img
              src={HERO_BG}
              alt=""
              className="h-full w-full object-cover brightness-[0.85] dark:brightness-[0.55]"
              fetchPriority="high"
              decoding="async"
            />
            <div
              className="absolute inset-0 bg-gradient-to-r from-black/80 via-black/50 to-black/30 dark:from-black/85 dark:via-black/60 dark:to-black/40"
              aria-hidden
            />
            <div
              className="absolute inset-0 bg-gradient-to-t from-black/50 via-transparent to-black/20"
              aria-hidden
            />
          </div>

          <div className="relative z-10 mx-auto flex min-h-[calc(100dvh-4.5rem)] max-w-[1400px] flex-col justify-around px-6 pb-10 pt-10 sm:px-10 sm:pb-12 sm:pt-14">
            <div className="max-w-2xl">
              <h1
                className="font-display text-4xl font-semibold leading-[1.05] tracking-tight text-white sm:text-5xl md:text-6xl lg:text-[3.5rem]"
                style={{ letterSpacing: "-0.03em" }}
              >
                Coordinated<br/> Ground Operations
              </h1>
              <p className="mt-6 max-w-xl text-base leading-relaxed text-white/90 sm:text-lg">
                FlightStrips centralizes clearance, movement, and handoff on the strip board—so delivery, ground, and tower
                stay aligned without parallel lists or clipboards.
              </p>
            </div>

            <div className="mt-12 flex flex-col gap-8 lg:mt-0 lg:flex-row lg:items-end lg:justify-between">
              <div className="flex flex-col gap-4 sm:flex-row sm:items-stretch sm:gap-5">
                {heroNavCards.map((card) => (
                  <HeroNavCard
                    key={card.index}
                    title={card.title}
                    indexLabel={card.index}
                    to={card.to}
                    external={card.external}
                  />
                ))}
              </div>
              <button
                type="button"
                className="flex h-12 w-12 shrink-0 items-center justify-center self-end rounded-full border border-white/30 bg-white text-neutral-900 shadow-lg transition hover:bg-white/95 lg:self-end"
                aria-label="Scroll to content"
                onClick={() => document.getElementById("after-hero")?.scrollIntoView({ behavior: "smooth" })}
              >
                <ChevronDown className="h-6 w-6" strokeWidth={stroke} aria-hidden />
              </button>
            </div>
          </div>
        </section>

        {/* Three pillars */}
        <section id="after-hero" className={cn("border-b scroll-mt-20 bg-[var(--hi-cap-bg)]", PUBLIC_SECTION_BORDER)}>
          <div className="mx-auto grid max-w-[1400px] md:grid-cols-3 py-8">
            {pillars.map((p) => {
              const Icon = p.icon;
              return (
                <div
                  key={p.title}
                  className={cn(
                    "group relative flex min-h-[260px] flex-col p-8 transition-colors duration-200 sm:p-10 md:border-r md:last:border-r-0",
                    "hover:!bg-white dark:hover:!bg-neutral-800",
                    PUBLIC_SECTION_BORDER,
                    "hi-grid-cell--muted"
                  )}
                >
                  <CornerDots />
                  <h2 className="font-display mb-4 text-2xl font-semibold tracking-tight text-neutral-950 sm:text-3xl dark:text-neutral-50 dark:group-hover:text-neutral-100">
                    {p.title}
                  </h2>
                  <p className="mb-8 flex-1 text-sm leading-relaxed text-neutral-600 dark:text-neutral-400 dark:group-hover:text-neutral-300">
                    {p.description}
                  </p>
                  <Link
                    to="/about"
                    className="text-[11px] font-semibold uppercase tracking-[0.2em] text-[#003d48] transition hover:text-neutral-950 dark:text-[#003d48] dark:hover:text-[#003d48] dark:group-hover:text-[#5cb8c4]"
                  >
                    Learn more
                  </Link>
                  <IconFoot icon={Icon} className="dark:group-hover:text-neutral-400" />
                </div>
              );
            })}
          </div>
        </section>

        {/* Community */}
        <section
          className={cn(
            "border-b bg-[#003d48] text-neutral-950 dark:border-white/10 dark:bg-[#003d48] dark:text-neutral-50",
            PUBLIC_SECTION_BORDER,
          )}
        >
          <div className="mx-auto grid max-w-[1400px] gap-8 px-6 py-14 sm:grid-cols-[1fr_auto] sm:items-center sm:px-10 sm:py-16">
            <div>
              <h2 className="font-display mb-4 max-w-xl text-3xl font-semibold leading-tight tracking-tight sm:text-4xl text-white/90">
                Built for VATSCA, open for everyone
              </h2>
              <p className="max-w-xl text-sm leading-relaxed text-white/90">
                FlightStrips is free and open source (GPL-3.0). <br/> Use it, modify it, and share it with the community.
              </p>
            </div>
          </div>
        </section>

        <section className={cn("border-b", PUBLIC_SECTION_BORDER)}>
          <div className="mx-auto grid max-w-[1400px] gap-0 md:grid-cols-2 py-8">
            {moreFeatures.map((f) => {
              const Icon = f.icon;
              return (
                <div
                  key={f.title}
                  className={cn(
                    "relative border-b p-8 sm:p-10 md:border-b-0 md:border-r md:last:border-r-0",
                    PUBLIC_SECTION_BORDER,
                    "hi-grid-cell",
                  )}
                >
                  <CornerDots />
                  <h3 className="font-display mb-2 text-xl font-semibold text-neutral-950 dark:text-neutral-50">{f.title}</h3>
                  <p className="text-sm leading-relaxed text-neutral-600 dark:text-neutral-400">{f.description}</p>
                  <IconFoot icon={Icon} />
                </div>
              );
            })}
          </div>
        </section>
        

        {/* Capabilities */}
        <section className={cn("border-b bg-[var(--hi-cap-bg)] dark:bg-neutral-950", PUBLIC_SECTION_BORDER)}>
          <div className="mx-auto max-w-[1400px] py-12">
            <h2
              className="font-display mb-10 text-2xl font-semibold tracking-tight text-neutral-950 sm:text-3xl dark:text-neutral-50"
              style={{ letterSpacing: "-0.02em" }}
            >
              An evolution in coordination
            </h2>
            <div className="grid gap-0 md:grid-cols-3">
              {capabilities.map((c) => {
                const Icon = c.icon;
                return (
                  <div
                    key={c.title}
                    className={cn(
                      "relative flex flex-col border bg-white/95 p-8 md:border-r md:last:border-r-0 dark:border-white/10 dark:bg-[#101010]",
                      PUBLIC_SECTION_BORDER,
                      "hi-grid-cell",
                    )}
                  >
                    <CornerDots />
                    <CapabilityVisual variant={c.visual} />
                    <h3 className="font-display mb-3 text-lg font-semibold text-neutral-950 dark:text-neutral-50">{c.title}</h3>
                    <p className="mb-6 flex-1 text-sm leading-relaxed text-neutral-600 dark:text-neutral-400">{c.description}</p>
                    <IconFoot icon={Icon} />
                  </div>
                );
              })}
            </div>
          </div>
        </section>
      </main>

      <PublicFooter tone="industrial" />
    </div>
  );
}
