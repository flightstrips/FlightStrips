import { PublicNavigation } from "@/components/public/PublicNavigation";
import { PublicFooter } from "@/components/public/PublicFooter";
import { CornerDots } from "@/components/public/CornerDots";
import { cn } from "@/lib/utils";
import { PUBLIC_NAV_INDUSTRY_CLASS, PUBLIC_PAGE_SHELL_CLASS, PUBLIC_SECTION_BORDER } from "@/lib/public-page-style";

const CONTACT_EMAIL = "info@flightstrips.dk";

const CONTRIBUTORS = ["Lukas Agerskov", "Frederik Rosenberg", "Simon Bjerre"] as const;

export default function Contact() {
  return (
    <div className={PUBLIC_PAGE_SHELL_CLASS}>
      <PublicNavigation linkTone="industrial" className={PUBLIC_NAV_INDUSTRY_CLASS} />

      <main className="flex-1 pt-[4.5rem]">
        <section className={cn("border-b", PUBLIC_SECTION_BORDER)}>
          <div className="mx-auto max-w-[1400px] px-6 py-14 sm:px-10 sm:py-20">
            <p className="mb-5 text-[10px] font-semibold uppercase tracking-[0.28em] text-neutral-500 dark:text-neutral-500">
              Get in touch
            </p>
            <h1
              className="font-display text-4xl font-semibold leading-[1.05] tracking-tight text-neutral-950 md:text-5xl lg:text-6xl dark:text-neutral-50"
              style={{ letterSpacing: "-0.03em" }}
            >
              Contact
            </h1>
          </div>
        </section>

        <section className={cn("flex-1 border-b", PUBLIC_SECTION_BORDER)}>
          <div className="mx-auto max-w-[1400px] px-6 py-12 sm:px-10 sm:py-16">
            <div className="grid gap-0 md:grid-cols-2">
              <div
                className={cn(
                  "relative border-b p-8 sm:p-10 md:border-b-0 md:border-r",
                  PUBLIC_SECTION_BORDER,
                  "hi-grid-cell bg-white dark:bg-[#101010]",
                )}
              >
                <CornerDots />
                <h2
                  className="font-display mb-4 text-2xl font-semibold tracking-tight text-neutral-950 dark:text-neutral-50 md:text-3xl"
                  style={{ letterSpacing: "-0.02em" }}
                >
                  Email
                </h2>
                <p className="mb-4 text-sm leading-relaxed text-neutral-600 dark:text-neutral-400">
                  For general enquiries, support, or feedback:
                </p>
                <a
                  href={`mailto:${CONTACT_EMAIL}`}
                  className="text-lg font-medium text-[#003d48] transition hover:underline dark:text-[#5cb8c4]"
                >
                  {CONTACT_EMAIL}
                </a>
              </div>

              <div className={cn("relative p-8 sm:p-10", "hi-grid-cell bg-[var(--hi-cap-bg)] dark:bg-[#0f0f0f]")}>
                <CornerDots />
                <h2
                  className="font-display mb-4 text-2xl font-semibold tracking-tight text-neutral-950 dark:text-neutral-50 md:text-3xl"
                  style={{ letterSpacing: "-0.02em" }}
                >
                  Contributors
                </h2>
                <p className="mb-6 text-sm leading-relaxed text-neutral-600 dark:text-neutral-400">
                  FlightStrips is an open-source project. Thanks to everyone who contributes.
                </p>
                <ul className="space-y-2 text-sm font-medium text-neutral-950 dark:text-neutral-100">
                  {CONTRIBUTORS.map((name) => (
                    <li key={name}>{name}</li>
                  ))}
                </ul>
                <p className="mt-8 text-sm text-neutral-500 dark:text-neutral-500">
                  See the{" "}
                  <a
                    href="https://github.com/flightstrips"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="font-medium text-[#003d48] transition hover:underline dark:text-[#5cb8c4]"
                  >
                    GitHub repository
                  </a>{" "}
                  for the full list of contributors.
                </p>
              </div>
            </div>
          </div>
        </section>
      </main>

      <PublicFooter tone="industrial" />
    </div>
  );
}
