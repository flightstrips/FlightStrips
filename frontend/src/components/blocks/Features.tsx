import { ChevronRight } from "lucide-react";
import { Link } from "react-router";
import { DashedLine } from "./DashedLine";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";

const items = [
  {
    title: "Datalink Clearance",
    label: "DCL",
    description:
      "Advanced DCL delivery with automated routing and conflict detection. Clearance at a glance, no direct Euroscope connection needed.",
  },
  {
    title: "Pushback & holding points",
    label: "Ground",
    description:
      "Manage pushback and holding point assignments with clarity and consistency.",
  },
  {
    title: "Internal communication",
    label: "Comms",
    description:
      "Stay coordinated with built-in internal comms and team visibility.",
  },
  {
    title: "Any device, anywhere",
    label: "Platform",
    description:
      "Run FlightStrips on desktop, tablet, or phone. No direct Euroscope link required—full capability from the browser.",
  },
];

export function Features() {
  return (
    <section className="py-24 px-6 sm:px-8 bg-white">
      <div className="max-w-5xl mx-auto">
        {/* Top dashed line with text (Mainline-style) */}
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
          Made for modern ATC teams
        </h2>
        <p className="text-navy/80 text-center max-w-2xl mx-auto mb-16 font-light">
          FlightStrips brings NITOS-inspired strip management to virtual ATC: DCL, pushback, holding points, and internal comms—on any device.
        </p>

        <div className="grid gap-6 sm:grid-cols-2">
          {items.map((item, i) => (
            <Card
              key={item.title}
              className="border-cream bg-cream/40 hover:border-primary/25 hover:shadow-md transition-all duration-300 overflow-hidden"
            >
              <CardContent className="p-6 sm:p-8">
                <p className="text-[11px] font-medium tracking-[0.2em] uppercase text-primary mb-3">
                  {item.label}
                </p>
                <div className="w-10 h-px bg-primary mb-4" />
                <h3 className="font-display font-semibold text-xl text-navy tracking-tight mb-2">
                  {item.title}
                </h3>
                <p className="text-navy/80 text-sm font-light leading-relaxed">
                  {item.description}
                </p>
                {i === 0 && (
                  <Button asChild variant="ghost" size="sm" className="mt-4 text-primary p-0 h-auto hover:bg-transparent">
                    <Link to="/login" className="inline-flex items-center gap-1">
                      Learn more
                      <ChevronRight className="h-4 w-4" />
                    </Link>
                  </Button>
                )}
              </CardContent>
            </Card>
          ))}
        </div>
      </div>
    </section>
  );
}
