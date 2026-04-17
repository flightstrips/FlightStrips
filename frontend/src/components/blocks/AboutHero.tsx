import { Link } from "react-router";
import { cn } from "@/lib/utils";
import { PUBLIC_SECTION_BORDER } from "@/lib/public-page-style";

const stats = [
  { value: "NITOS", label: "Inspired by" },
  { value: "Any device", label: "Runs on" },
  { value: "DCL + more", label: "Features" },
  { value: "Open source", label: "Community" },
];

export function AboutHero() {
  return (
    <section className={cn("border-b", PUBLIC_SECTION_BORDER)}>
      <div className="mx-auto max-w-[1400px] px-6 py-14 sm:px-10 sm:py-20">
        <p className="mb-5 text-[10px] font-semibold uppercase tracking-[0.28em] text-neutral-500 dark:text-neutral-500">
          About
        </p>
        <h1
          className="font-display max-w-3xl text-4xl font-semibold leading-[1.05] tracking-tight text-neutral-950 sm:text-5xl md:text-6xl dark:text-neutral-50"
          style={{ letterSpacing: "-0.03em" }}
        >
          Built for virtual ATC
        </h1>
        <p className="mt-6 max-w-2xl text-sm leading-relaxed text-neutral-600 dark:text-neutral-400 sm:text-base">
          FlightStrips brings NITOS-inspired strip management to simulation: precision, clarity, and reliability—on any device.
        </p>
        <p className="mt-6 text-sm text-neutral-500 dark:text-neutral-500">
          <Link to="/" className="transition hover:text-[#003d48] dark:hover:text-[#5cb8c4]">
            Home
          </Link>
          <span className="mx-2 text-neutral-400">/</span>
          About
        </p>

        <div className="mt-12 grid grid-cols-2 gap-px bg-neutral-300/90 sm:grid-cols-4 dark:bg-white/15">
          {stats.map((stat) => (
            <div key={stat.label} className="bg-[var(--hi-bg)] p-6 dark:bg-[#101010]">
              <p className="font-display text-xl font-semibold tracking-tight text-[#003d48] dark:text-[#5cb8c4] sm:text-2xl">
                {stat.value}
              </p>
              <p className="mt-1 text-xs text-neutral-600 dark:text-neutral-400">{stat.label}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
