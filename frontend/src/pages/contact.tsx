import { PublicNavigation } from "@/components/public/PublicNavigation";
import { PublicFooter } from "@/components/public/PublicFooter";
import { ScrollProgress } from "@/components/public/ScrollProgress";
import { ScrollReveal } from "@/components/public/ScrollReveal";

const CONTACT_EMAIL = "flightstripsdevelopment@gmail.com";

const CONTRIBUTORS = [
  "Lukas Agerskov",
  "Frederik Rosenberg",
  "Simon Bjerre"
] as const;

export default function Contact() {
  return (
    <div className="min-h-screen bg-cream text-navy flex flex-col">
      <ScrollProgress />
      <PublicNavigation />

      {/* Hero */}
      <section className="py-28 px-8 border-b border-navy/10">
        <div className="max-w-7xl mx-auto">
          <ScrollReveal>
            <div className="flex items-center gap-4 mb-4">
              <div className="w-8 h-px bg-navy/20" />
              <p className="text-xs tracking-widest uppercase text-navy/60">
                Get in touch
              </p>
            </div>
          </ScrollReveal>
          <ScrollReveal delay={0.1}>
            <h1
              className="font-display font-normal text-5xl md:text-7xl mb-6 text-navy"
              style={{ letterSpacing: "-0.02em" }}
            >
              Contact
            </h1>
          </ScrollReveal>
        </div>
      </section>

      {/* Content */}
      <section className="py-20 px-8 flex-1">
        <div className="max-w-4xl mx-auto space-y-16">
          <ScrollReveal>
            <div>
              <h2
                className="font-display font-normal text-2xl md:text-3xl mb-4 text-navy"
                style={{ letterSpacing: "-0.01em" }}
              >
                Email
              </h2>
              <p className="font-sans font-light text-base text-navy/80 leading-relaxed mb-2">
                For general enquiries, support, or feedback:
              </p>
              <a
                href={`mailto:${CONTACT_EMAIL}`}
                className="text-primary hover:underline font-medium text-lg"
              >
                {CONTACT_EMAIL}
              </a>
            </div>
          </ScrollReveal>

          <ScrollReveal delay={0.1}>
            <div>
              <h2
                className="font-display font-normal text-2xl md:text-3xl mb-4 text-navy"
                style={{ letterSpacing: "-0.01em" }}
              >
                Contributors
              </h2>
              <p className="font-sans font-light text-base text-navy/80 leading-relaxed mb-6">
                FlightStrips is an open-source project. Thanks to everyone who
                contributes.
              </p>
              <ul className="space-y-2">
                {CONTRIBUTORS.map((name) => (
                  <li
                    key={name}
                    className="font-sans text-navy font-medium"
                  >
                    {name}
                  </li>
                ))}
              </ul>
              <p className="mt-6 text-sm text-navy/60">
                See the{" "}
                <a
                  href="https://github.com/flightstrips"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-primary hover:underline"
                >
                  GitHub repository
                </a>{" "}
                for the full list of contributors.
              </p>
            </div>
          </ScrollReveal>
        </div>
      </section>

      <PublicFooter />
    </div>
  );
}
