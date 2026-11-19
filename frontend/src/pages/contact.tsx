import { PageHero, PublicPage } from "@/components/public/SiteChrome";
import { TextLink } from "@/components/public/landing/primitives";

const CONTACT_EMAIL = "info@flightstrips.dk";

const CONTRIBUTORS = ["Lukas Agerskov", "Frederik Rosenberg", "Simon Bjerre"] as const;

const BODY = "text-[15px] leading-relaxed text-[var(--fsl-ink-muted)]";

export default function Contact() {
  return (
    <PublicPage>
      <PageHero eyebrow="Get in touch" title="Contact" />

      <section>
        <div className="fsl-gutter py-16 sm:py-24">
          <div className="grid border-l border-t border-[var(--fsl-line)] md:grid-cols-2">
            <div className="fsl-hoverbar border-b border-r border-[var(--fsl-line)] bg-[var(--fsl-surface)] p-8 sm:p-10">
              <h2 className="fsl-display mb-4 text-[28px] sm:text-[32px]">Email</h2>
              <p className={`mb-6 ${BODY}`}>For general enquiries, support, or feedback:</p>
              <TextLink label={CONTACT_EMAIL} href={`mailto:${CONTACT_EMAIL}`} tone="plain" ink className="text-lg" />
            </div>

            <div className="fsl-hoverbar border-b border-r border-[var(--fsl-line)] p-8 sm:p-10">
              <h2 className="fsl-display mb-4 text-[28px] sm:text-[32px]">Contributors</h2>
              <p className={`mb-6 ${BODY}`}>
                FlightStrips is an open-source project. Thanks to everyone who contributes.
              </p>
              <ul className="mb-8 space-y-2 text-[15px] font-medium">
                {CONTRIBUTORS.map((name) => (
                  <li key={name}>{name}</li>
                ))}
              </ul>
              <TextLink label="All contributors on GitHub" href="https://github.com/flightstrips" ink arrow external />
            </div>
          </div>
        </div>
      </section>
    </PublicPage>
  );
}
