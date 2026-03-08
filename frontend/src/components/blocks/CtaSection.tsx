import { ArrowRight } from "lucide-react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";

export function CtaSection() {
  return (
    <section className="py-24 px-6 sm:px-8 bg-cream">
      <div className="max-w-3xl mx-auto text-center">
        <p className="text-[11px] font-medium tracking-[0.2em] uppercase text-primary mb-3">
          Get Started
        </p>
        <h2
          className="font-display font-semibold text-3xl sm:text-4xl md:text-5xl text-navy tracking-tight mb-6"
          style={{ letterSpacing: "-0.02em" }}
        >
          Ready to run your strip board?
        </h2>
        <p className="font-sans font-light text-navy/80 mb-10 leading-relaxed">
          Join virtual ATC communities worldwide using FlightStrips for simulation and training.
        </p>
        <Button asChild size="lg" className="bg-primary hover:opacity-95 text-white rounded-sm shadow-sm">
          <Link to="/login">
            Sign In
            <ArrowRight className="ml-2 h-4 w-4" />
          </Link>
        </Button>
      </div>
    </section>
  );
}
