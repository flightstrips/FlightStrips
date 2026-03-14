import { DashedLine } from "./DashedLine";
import { Card, CardContent } from "@/components/ui/card";

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
      {/* Vision */}
      <section className="py-20 px-6 sm:px-8 bg-white">
        <div className="max-w-5xl mx-auto">
          <div className="grid md:grid-cols-2 gap-12 md:gap-16 items-center">
            <div className="flex items-start gap-4">
              <div className="w-1 h-24 bg-primary rounded-full shrink-0" />
              <div>
                <p className="text-[11px] font-medium tracking-[0.2em] uppercase text-primary mb-4">
                  Vision
                </p>
                <h2
                  className="font-display font-semibold text-3xl sm:text-4xl md:text-5xl text-navy tracking-tight"
                  style={{ letterSpacing: "-0.02em" }}
                >
                  Our vision for a next-generation strip management system
                </h2>
              </div>
            </div>
            <div className="space-y-6">
              <p className="text-navy/80 font-light leading-relaxed">
                FlightStrips represents a fundamental reimagining of air traffic control strip management,
                designed specifically for virtual ATC environments. We combine precision engineering with
                intuitive design to deliver a system that feels both powerful and effortless.
              </p>
              <p className="text-navy/80 font-light leading-relaxed">
                Built for simulation communities, FlightStrips enables controllers to focus on what matters:
                safe, efficient air traffic management. Every feature is crafted with the understanding that
                clarity and reliability are non-negotiable in high-stakes environments.
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* Principles */}
      <section className="py-20 px-6 sm:px-8 bg-cream">
        <div className="max-w-5xl mx-auto">
          <div className="flex items-center gap-4 mb-8">
            <DashedLine className="flex-1 border-navy/20" />
            <span className="text-[11px] font-medium tracking-[0.2em] uppercase text-primary whitespace-nowrap">
              Principles
            </span>
            <DashedLine className="flex-1 border-navy/20" />
          </div>
          <h2
            className="font-display font-semibold text-3xl sm:text-4xl md:text-5xl text-navy tracking-tight mb-12"
            style={{ letterSpacing: "-0.02em" }}
          >
            Built on core principles
          </h2>
          <div className="grid md:grid-cols-3 gap-6">
            {principles.map((item) => (
              <Card
                key={item.id}
                className="border-navy/10 bg-white hover:border-primary/20 transition-colors"
              >
                <CardContent className="p-6 sm:p-8">
                  <p className="text-[11px] font-medium tracking-[0.2em] uppercase text-navy/60 mb-4">
                    {item.id}
                  </p>
                  <div className="w-10 h-px bg-primary mb-4" />
                  <h3 className="font-display font-semibold text-xl text-navy tracking-tight mb-3">
                    {item.title}
                  </h3>
                  <p className="text-navy/80 text-sm font-light leading-relaxed">
                    {item.description}
                  </p>
                </CardContent>
              </Card>
            ))}
          </div>
        </div>
      </section>

      {/* Quote */}
      <section className="py-20 px-6 sm:px-8 bg-white">
        <div className="max-w-3xl mx-auto">
          <blockquote className="border-l-2 border-primary pl-6">
            <p className="text-navy/90 text-lg md:text-xl font-light italic leading-relaxed mb-4">
              &ldquo;FlightStrips has transformed how our vACC manages operations.
              The precision and clarity of the system allows controllers to focus entirely on
              what they do best. Compared to previous systems, FlightStrips is a game changer.&rdquo;
            </p>
            <footer className="mt-4">
              <p className="text-sm font-medium text-navy">VATSCA vACC Director</p>
              <p className="text-xs text-navy/60">Simon Bjerre</p>
            </footer>
          </blockquote>
        </div>
      </section>

      {/* Open source CTA */}
      <section className="py-20 px-6 sm:px-8 bg-cream">
        <div className="max-w-5xl mx-auto">
          <p className="text-[11px] font-medium tracking-[0.2em] uppercase text-primary mb-4">
            Open Source
          </p>
          <h2
            className="font-display font-semibold text-3xl sm:text-4xl md:text-5xl text-navy tracking-tight mb-6"
            style={{ letterSpacing: "-0.02em" }}
          >
            Free and open-source
          </h2>
          <p className="text-navy/80 font-light max-w-2xl mb-8 leading-relaxed">
            FlightStrips is a free and open-source project, built by and for the virtual ATC community.
            Support is available via GitHub, and contributions are welcome from developers and controllers
            who share our vision for better strip management.
          </p>
          <a
            href="https://github.com/flightstrips/FlightStrips"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-2 bg-primary text-white px-6 py-3 text-sm font-medium rounded-sm hover:opacity-95 transition-opacity"
          >
            View on GitHub
            <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth="2" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" d="M17.25 8.25L21 12m0 0l-3.75 3.75M21 12H3" />
            </svg>
          </a>
        </div>
      </section>
    </>
  );
}
