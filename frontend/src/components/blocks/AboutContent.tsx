import { ActionLink, Eyebrow, Reveal } from "@/components/public/landing/primitives";

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

const BAND = "border-b border-[var(--fsl-line)]";
const H2 = "fsl-display text-[32px] sm:text-[44px] lg:text-[52px]";
const BODY = "text-[15px] leading-relaxed text-[var(--fsl-ink-muted)]";

export function AboutContent() {
  return (
    <>
      <section className={`${BAND} bg-[var(--fsl-surface)]`}>
        <div className="fsl-gutter grid gap-10 py-16 sm:py-24 lg:grid-cols-2 lg:gap-20">
          <Reveal>
            <Eyebrow className="mb-5">Vision</Eyebrow>
            <h2 className={H2}>Our vision for a next-generation strip management system</h2>
          </Reveal>
          <Reveal delay={100} className={`space-y-6 ${BODY}`}>
            <p>
              FlightStrips represents a fundamental reimagining of air traffic control strip management, designed
              specifically for virtual ATC environments. We combine precision engineering with intuitive design to
              deliver a system that feels both powerful and effortless.
            </p>
            <p>
              Built for simulation communities, FlightStrips enables controllers to focus on what matters: safe,
              efficient air traffic management. Every feature is crafted with the understanding that clarity and
              reliability are non-negotiable in high-stakes environments.
            </p>
          </Reveal>
        </div>
      </section>

      <section className={BAND}>
        <div className="fsl-gutter py-16 sm:py-24">
          <Reveal>
            <Eyebrow className="mb-5">Principles</Eyebrow>
            <h2 className={`${H2} mb-12 max-w-[16ch]`}>Built on core principles</h2>
          </Reveal>
          <div className="grid border-l border-t border-[var(--fsl-line)] md:grid-cols-3">
            {principles.map((item, index) => (
              <Reveal
                key={item.id}
                delay={index * 80}
                className="fsl-hoverbar border-b border-r border-[var(--fsl-line)] p-8"
              >
                <p className="fsl-mono mb-4 text-[11px] tracking-[0.16em] text-[var(--fsl-brand-ink)]">{item.id}</p>
                <h3 className="fsl-display mb-3 text-[24px]">{item.title}</h3>
                <p className={BODY}>{item.description}</p>
              </Reveal>
            ))}
          </div>
        </div>
      </section>

      <section className={`${BAND} bg-[var(--fsl-surface)]`}>
        <Reveal className="fsl-gutter py-16 sm:py-24">
          <blockquote className="max-w-3xl border-l border-[var(--fsl-brand-ink)] pl-6 md:pl-8">
            <p className="text-[19px] leading-relaxed sm:text-[22px]">
              &ldquo;FlightStrips has transformed how our vACC manages operations. The precision and clarity of the
              system allows controllers to focus entirely on what they do best. Compared to previous systems,
              FlightStrips is a game changer.&rdquo;
            </p>
            <footer className="mt-6">
              <p className="text-sm font-medium">VATSCA vACC Director</p>
              <p className="fsl-eyebrow mt-2">Simon Bjerre</p>
            </footer>
          </blockquote>
        </Reveal>
      </section>

      <section>
        <Reveal className="fsl-gutter py-16 sm:py-24">
          <Eyebrow className="mb-5">Open source</Eyebrow>
          <h2 className={`${H2} mb-6`}>Free and open-source</h2>
          <p className={`mb-8 max-w-2xl ${BODY}`}>
            FlightStrips is a free and open-source project, built by and for the virtual ATC community. Support is
            available via GitHub, and contributions are welcome from developers and controllers who share our vision
            for better strip management.
          </p>
          <ActionLink label="View on GitHub" href="https://github.com/flightstrips/FlightStrips" external />
        </Reveal>
      </section>
    </>
  );
}
