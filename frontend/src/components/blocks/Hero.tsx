import {
  ArrowRight,
  Radio,
  MapPin,
  MessageSquare,
  Smartphone,
} from "lucide-react";
import { DashedLine } from "./DashedLine";
import { Button } from "@/components/ui/button";
import { useAuth0 } from "@auth0/auth0-react";
import { Link } from "react-router-dom";

const features = [
  {
    title: "Datalink Clearance",
    description: "Advanced DCL with routing and conflict detection.",
    icon: Radio,
  },
  {
    title: "Pushback & holding points",
    description: "Manage assignments with clarity.",
    icon: MapPin,
  },
  {
    title: "Internal communication",
    description: "Built-in comms and team visibility.",
    icon: MessageSquare,
  },
  {
    title: "Any device",
    description: "Desktop, tablet, or phone—no Euroscope required.",
    icon: Smartphone,
  },
];

export function Hero() {
  const { isAuthenticated, loginWithRedirect } = useAuth0();
  return (
    <section
      className="relative min-h-[90dvh] flex flex-col lg:flex-row items-stretch gap-0"
      style={{
        background: `
          radial-gradient(ellipse 100% 80% at 50% 0%, rgba(0, 61, 72, 0.08) 0%, transparent 55%),
          linear-gradient(180deg, #ffffff 0%, #F3EEE8 50%, #F3EEE8 100%)
        `,
      }}
    >
      {/* Left - Main content */}
      <div className="flex-1 flex flex-col justify-center px-6 sm:px-8 lg:pl-12 xl:pl-24 py-20 lg:py-28">
        <h1
          className="font-display font-semibold text-5xl sm:text-6xl md:text-7xl lg:text-8xl text-navy tracking-tight mb-6"
          style={{ letterSpacing: "-0.03em", lineHeight: 0.95 }}
        >
          FlightStrips
        </h1>
        <p className="font-sans font-light text-lg sm:text-xl text-navy/80 max-w-xl mb-10 leading-relaxed">
          Datalink clearance, pushback & holding points, internal comms. Runs on any device—no Euroscope required.
        </p>
        <div className="flex flex-col sm:flex-row gap-4">
          {isAuthenticated ? (
            <Button asChild size="lg" className="text-white rounded-sm shadow-sm w-fit">
              <Link to="/app">
                Open App
                <ArrowRight className="ml-2 h-4 w-4" />
              </Link>
            </Button>
          ) : (
            <Button size="lg" className="text-white rounded-sm shadow-sm w-fit" onClick={() => loginWithRedirect()}>
              Get Started
              <ArrowRight className="ml-2 h-4 w-4" />
            </Button>
          )}
          <Button asChild variant="outline" size="lg" className="border-2 border-primary/50 text-primary hover:bg-primary/10 rounded-sm w-fit">
            <Link to="/about">Learn More</Link>
          </Button>
        </div>
      </div>

      {/* Vertical dashed line */}
      <div className="hidden lg:flex items-center py-12">
        <DashedLine orientation="vertical" className="border-navy/20" />
      </div>

      {/* Right - Feature list (Mainline-style) */}
      <div className="flex-1 flex flex-col justify-center px-6 sm:px-8 lg:pr-12 xl:pr-24 py-12 lg:py-28">
        <div className="space-y-6">
          {features.map((feature) => {
            const Icon = feature.icon;
            return (
              <div
                key={feature.title}
                className="flex gap-4 items-start group"
              >
                <div className="rounded-lg border border-navy/15 bg-white/80 p-3 shrink-0 group-hover:border-primary/30 transition-colors">
                  <Icon className="h-5 w-5 text-primary" />
                </div>
                <div>
                  <h3 className="font-display font-semibold text-navy tracking-tight">
                    {feature.title}
                  </h3>
                  <p className="text-sm text-navy/70 font-light mt-0.5">
                    {feature.description}
                  </p>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      <div
        className="absolute bottom-0 left-0 w-full h-px"
        style={{
          background: "linear-gradient(90deg, transparent 0%, rgba(0, 61, 72, 0.25) 50%, transparent 100%)",
        }}
      />
    </section>
  );
}
