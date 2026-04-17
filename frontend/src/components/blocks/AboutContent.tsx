import { CornerDots } from "@/components/public/CornerDots";
import { cn } from "@/lib/utils";
import { PUBLIC_SECTION_BORDER } from "@/lib/public-page-style";

const principles = [
  {
    id: "01",
    title: "Precision",
    description:
      "FlightStrips is designed to match the real-life counterpart 1:1 in nearly all scenarios. Every workflow, interaction, and system behavior mirrors authentic air traffic control operations for true-to-life simulation and training.",
  },
  {
    id: "02",
    title: "Reliability",
    description:
      "All systems are connected and talk instantly and securely together. Real-time synchronization ensures seamless communication between components, maintaining data integrity and operational continuity across the entire platform.",
  },
  {
    id: "03",
    title: "Clarity",
    description:
      "Critical data is presented clearly and comprehensively, enabling controllers to make informed decisions with full situational awareness and seamless coordination between positions.",
  },
];

export function AboutContent() {
  return (
    <>
      <section className={cn("border-b bg-white dark:bg-[#0a0a0a]", PUBLIC_SECTION_BORDER)}>
        <div className="mx-auto max-w-[1400px] px-6 py-16 sm:px-10 sm:py-20">
          <div className="grid items-start gap-12 md:grid-cols-2 md:gap-16">
            <div className="flex items-start gap-4">
              <div className="h-24 w-1 shrink-0 rounded-full bg-[#003d48]" />
              <div>
                <p className="mb-4 text-[10px] font-semibold uppercase tracking-[0.2em] text-[#003d48]">Vision</p>
                <h2
                  className="font-display text-3xl font-semibold tracking-tight text-neutral-950 sm:text-4xl md:text-5xl dark:text-neutral-50"
                  style={{ letterSpacing: "-0.02em" }}
                >
                  Our vision for a next-generation strip management system
                </h2>
              </div>
            </div>
            <div className="space-y-6 text-sm leading-relaxed text-neutral-600 dark:text-neutral-400">
              <p>
                FlightStrips represents a fundamental reimagining of air traffic control strip management, designed specifically for
                virtual ATC environments. We combine precision engineering with intuitive design to deliver a system that feels
                both powerful and effortless.
              </p>
              <p>
                Built for simulation communities, FlightStrips enables controllers to focus on what matters: safe, efficient air
                traffic management. Every feature is crafted with the understanding that clarity and reliability are non-negotiable
                in high-stakes environments.
              </p>
            </div>
          </div>
        </div>
      </section>

      <section className={cn("border-b", PUBLIC_SECTION_BORDER)}>
        <div className="mx-auto max-w-[1400px] px-6 py-16 sm:px-10 sm:py-20">
          <p className="mb-4 text-[10px] font-semibold uppercase tracking-[0.2em] text-neutral-500 dark:text-neutral-500">
            Principles
          </p>
          <h2
            className="font-display mb-12 max-w-2xl text-3xl font-semibold tracking-tight text-neutral-950 sm:text-4xl md:text-5xl dark:text-neutral-50"
            style={{ letterSpacing: "-0.02em" }}
          >
            Built on core principles
          </h2>
          <div className="grid gap-0 md:grid-cols-3">
            {principles.map((item) => (
              <div
                key={item.id}
                className={cn(
                  "relative border-b p-8 last:border-b-0 md:border-b-0 md:border-r md:last:border-r-0",
                  PUBLIC_SECTION_BORDER,
                  "hi-grid-cell hi-grid-cell--muted",
                )}
              >
                <CornerDots />
                <p className="mb-3 font-mono text-xs tabular-nums text-neutral-400">{item.id}</p>
                <h3 className="font-display mb-3 text-xl font-semibold text-neutral-950 dark:text-neutral-50">{item.title}</h3>
                <p className="text-sm leading-relaxed text-neutral-600 dark:text-neutral-400">{item.description}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className={cn("border-b bg-white dark:bg-[#0a0a0a]", PUBLIC_SECTION_BORDER)}>
        <div className="mx-auto max-w-[1400px] px-6 py-16 sm:px-10 sm:py-20">
          <blockquote className="border-l-2 border-[#003d48] pl-6 md:pl-8">
            <p className="text-base font-light italic leading-relaxed text-neutral-800 dark:text-neutral-200 md:text-lg">
              &ldquo;FlightStrips has transformed how our vACC manages operations. The precision and clarity of the system allows
              controllers to focus entirely on what they do best. Compared to previous systems, FlightStrips is a game
              changer.&rdquo;
            </p>
            <footer className="mt-6">
              <p className="text-sm font-medium text-neutral-950 dark:text-neutral-50">VATSCA vACC Director</p>
              <p className="text-xs text-neutral-500 dark:text-neutral-500">Simon Bjerre</p>
            </footer>
          </blockquote>
        </div>
      </section>

      <section className={cn("border-b bg-[var(--hi-cap-bg)] dark:bg-neutral-950", PUBLIC_SECTION_BORDER)}>
        <div className="mx-auto max-w-[1400px] px-6 py-16 sm:px-10 sm:py-20">
          <p className="mb-4 text-[10px] font-semibold uppercase tracking-[0.2em] text-[#003d48]">Open source</p>
          <h2
            className="font-display mb-6 max-w-2xl text-3xl font-semibold tracking-tight text-neutral-950 sm:text-4xl md:text-5xl dark:text-neutral-50"
            style={{ letterSpacing: "-0.02em" }}
          >
            Free and open-source
          </h2>
          <p className="mb-8 max-w-2xl text-sm leading-relaxed text-neutral-600 dark:text-neutral-400">
            FlightStrips is a free and open-source project, built by and for the virtual ATC community. Support is available via
            GitHub, and contributions are welcome from developers and controllers who share our vision for better strip management.
          </p>
          <a
            href="https://github.com/flightstrips/FlightStrips"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-2 rounded-sm border border-[#003d48] bg-[#003d48] px-6 py-3 text-sm font-semibold text-white transition hover:bg-[#004d5c]"
          >
            View on GitHub
            <span aria-hidden>→</span>
          </a>
        </div>
      </section>
    </>
  );
}
