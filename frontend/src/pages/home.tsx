import { Link } from "react-router";
import { ArrowRight, Radio, MapPin, MessageSquare, Smartphone, GitBranch, Layers } from "lucide-react";
import { PublicNavigation } from "@/components/public/PublicNavigation";
import { PublicFooter } from "@/components/public/PublicFooter";
import { DashedLine } from "@/components/blocks/DashedLine";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";

const features = [
  {
    title: "Datalink Clearance (DCL)",
    description:
      "Advanced DCL delivery with automated routing and conflict detection. Clearance at a glance—no direct Euroscope connection needed.",
    icon: Radio,
  },
  {
    title: "Pushback & holding points",
    description:
      "Manage pushback and holding point assignments with clarity. Assign release points and runway clearances in one place.",
    icon: MapPin,
  },
  {
    title: "Internal communication",
    description:
      "Stay coordinated with built-in internal comms, team visibility, and controller online/offline awareness.",
    icon: MessageSquare,
  },
  {
    title: "Any device, anywhere",
    description:
      "Run FlightStrips on desktop, tablet, or phone. Touch-first design for FSTools and similar devices—full capability from the browser.",
    icon: Smartphone,
  },
  {
    title: "Euroscope integration",
    description:
      "Two-way sync with Euroscope for full coordination between strip board and radar. Keep everyone on the same page.",
    icon: Layers,
  },
  {
    title: "Flow management & CDM",
    description:
      "Integrated Collaborative Decision Making: CTOT, TSAT, and ECFMP-style flow management for high-traffic scenarios.",
    icon: GitBranch,
  },
];

export default function Home() {
  return (
    <div className="min-h-screen bg-cream text-navy flex flex-col">
      <PublicNavigation />

      <main className="flex-1">
        {/* Hero */}
        <section
          className="relative pt-24 pb-20 sm:pt-32 sm:pb-28 px-6 sm:px-8"
          style={{
            background: `
              radial-gradient(ellipse 100% 80% at 50% 0%, rgba(0, 61, 72, 0.06) 0%, transparent 55%),
              linear-gradient(180deg, #F3EEE8 0%, #F3EEE8 100%)
            `,
          }}
        >
          <div className="max-w-5xl mx-auto">
            <div className="flex items-center gap-4 mb-8">
              <DashedLine className="flex-1 border-navy/20" />
              <span className="text-[11px] font-medium tracking-[0.2em] uppercase text-primary whitespace-nowrap">
                Strip management for virtual ATC
              </span>
              <DashedLine className="flex-1 border-navy/20" />
            </div>
            <h1
              className="font-display font-semibold text-4xl sm:text-5xl md:text-6xl lg:text-7xl text-navy tracking-tight mb-6"
              style={{ letterSpacing: "-0.02em", lineHeight: 1.05 }}
            >
              FlightStrips
            </h1>
            <p className="font-sans font-light text-lg sm:text-xl text-navy/85 max-w-2xl leading-relaxed mb-10">
              FlightStrips is a strip management program designed for coordination and management of aircraft on the ground. The core value is the centralization of all required data in order to run all ground operations without any use of lists.
            </p>
            <div className="flex flex-col sm:flex-row gap-4">
              <Button asChild size="lg" className="bg-primary hover:bg-primary/90 text-white rounded-sm shadow-sm w-fit">
                <Link to="https://docs.flightstrips.dk">
                  Get Started
                  <ArrowRight className="ml-2 h-4 w-4" />
                </Link>
              </Button>
              <Button asChild variant="outline" size="lg" className="border-2 border-primary/50 text-primary hover:bg-primary/10 rounded-sm w-fit">
                <Link to="/about">Learn More</Link>
              </Button>
            </div>
          </div>
        </section>

        {/* Features grid */}
        <section className="py-24 px-6 sm:px-8 bg-white">
          <div className="max-w-5xl mx-auto">
            <div className="flex items-center gap-4 mb-12">
              <DashedLine className="flex-1 border-navy/20" />
              <span className="text-[11px] font-medium tracking-[0.2em] uppercase text-primary whitespace-nowrap">
                Built for the way you work
              </span>
              <DashedLine className="flex-1 border-navy/20" />
            </div>
            <h2
              className="font-display font-semibold text-3xl sm:text-4xl md:text-5xl text-navy tracking-tight mb-4 text-center"
              style={{ letterSpacing: "-0.02em" }}
            >
              Everything in one place
            </h2>
            <p className="text-navy/80 text-center max-w-2xl mx-auto mb-16 font-light">
              NITOS-inspired strip management for VATSIM: DCL, pushback, holding points, internal comms, and flow management—on any device.
            </p>
            <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
              {features.map((item) => {
                const Icon = item.icon;
                return (
                  <Card
                    key={item.title}
                    className="border-cream bg-cream/40 hover:border-primary/25 hover:shadow-md transition-all duration-300 overflow-hidden"
                  >
                    <CardContent className="p-6">
                      <div className="rounded-lg border border-navy/15 bg-white/80 p-3 w-fit mb-4">
                        <Icon className="h-5 w-5 text-primary" />
                      </div>
                      <h3 className="font-display font-semibold text-lg text-navy tracking-tight mb-2">
                        {item.title}
                      </h3>
                      <p className="text-navy/80 text-sm font-light leading-relaxed">
                        {item.description}
                      </p>
                    </CardContent>
                  </Card>
                );
              })}
            </div>
          </div>
        </section>

        {/* Centralization / no lists */}
        <section className="py-24 px-6 sm:px-8 bg-cream">
          <div className="max-w-5xl mx-auto">
            <div className="grid md:grid-cols-2 gap-12 md:gap-16 items-center">
              <div>
                <p className="text-[11px] font-medium tracking-[0.2em] uppercase text-primary mb-4">
                  One source of truth
                </p>
                <h2
                  className="font-display font-semibold text-3xl sm:text-4xl text-navy tracking-tight mb-6"
                  style={{ letterSpacing: "-0.02em" }}
                >
                  Run ground ops without lists
                </h2>
                <p className="text-navy/80 font-light leading-relaxed mb-4">
                  All data lives on the strip board. Clearance, pushback, taxi, runway, and handoff state are centralized—no separate lists or clipboards. One system for delivery, ground, and tower coordination.
                </p>
                <p className="text-navy/80 font-light leading-relaxed">
                  Designed to match real-life workflows 1:1 for true-to-life simulation and training on VATSIM and other networks.
                </p>
              </div>
              <div className="flex flex-wrap gap-3">
                {["DCL", "Pushback", "Holding points", "Runway", "Coordination", "CDM"].map((label) => (
                  <span
                    key={label}
                    className="px-4 py-2 rounded-md border border-navy/15 bg-white/60 text-navy/80 text-sm font-medium"
                  >
                    {label}
                  </span>
                ))}
              </div>
            </div>
          </div>
        </section>

        {/* VATSIM & open source */}
        <section className="py-24 px-6 sm:px-8 bg-white">
          <div className="max-w-3xl mx-auto text-center">
            <p className="text-[11px] font-medium tracking-[0.2em] uppercase text-primary mb-3">
              Community
            </p>
            <h2
              className="font-display font-semibold text-3xl sm:text-4xl md:text-5xl text-navy tracking-tight mb-6"
              style={{ letterSpacing: "-0.02em" }}
            >
              Built for VATSIM, open for everyone
            </h2>
            <p className="font-sans font-light text-navy/80 mb-10 leading-relaxed">
              FlightStrips is free and open-source (GPL-3.0), built by and for the virtual ATC community. Compatible with Windows, Mac, and Linux. Use it for simulation and training — no lists, no clutter, just strips.
            </p>
            <Button asChild size="lg" className="bg-primary hover:bg-primary/90 text-white rounded-sm shadow-sm">
              <Link to="/login">
                Sign in to get started
                <ArrowRight className="ml-2 h-4 w-4" />
              </Link>
            </Button>
          </div>
        </section>
      </main>

      <PublicFooter />
    </div>
  );
}
