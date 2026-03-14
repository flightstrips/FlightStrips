import { Link } from "react-router";
import { DashedLine } from "./DashedLine";

const stats = [
  { value: "NITOS", label: "Inspired by" },
  { value: "Any device", label: "Runs on" },
  { value: "DCL + more", label: "Features" },
  { value: "Open source", label: "Community" },
];

export function AboutHero() {
  return (
    <section className="py-20 sm:py-28 px-6 sm:px-8 bg-cream dark:bg-background">
      <div className="max-w-5xl mx-auto">
        <div className="flex items-center gap-4 mb-8">
          <DashedLine className="flex-1 border-navy/20 dark:border-white/20" />
          <span className="text-[11px] font-medium tracking-[0.2em] uppercase text-primary whitespace-nowrap">
            About
          </span>
          <DashedLine className="flex-1 border-navy/20 dark:border-white/20" />
        </div>
        <h1
          className="font-display font-semibold text-4xl sm:text-5xl md:text-6xl lg:text-7xl text-navy dark:text-cream tracking-tight mb-6"
          style={{ letterSpacing: "-0.02em" }}
        >
          Built for virtual ATC
        </h1>
        <p className="text-lg sm:text-xl text-navy/80 dark:text-cream/80 font-light max-w-2xl mb-4">
          FlightStrips brings NITOS-inspired strip management to simulation: precision, clarity, and reliability—on any device.
        </p>
        <p className="text-sm text-navy/60 dark:text-cream/60 mb-12">
          <Link to="/" className="hover:text-primary transition-colors">Home</Link>
          <span className="mx-2">/</span>
          About Us
        </p>

        <div className="grid grid-cols-2 sm:grid-cols-4 gap-6 sm:gap-8">
          {stats.map((stat) => (
            <div key={stat.label}>
              <p className="font-display font-semibold text-2xl sm:text-3xl text-primary tracking-tight">
                {stat.value}
              </p>
              <p className="text-sm text-navy/70 dark:text-cream/70 mt-1">{stat.label}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
