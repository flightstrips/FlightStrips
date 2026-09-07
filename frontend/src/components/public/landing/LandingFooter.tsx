import { Link } from "react-router";
import { FOOTER_COLUMNS, SITE } from "./content";
import { LandingLogo } from "./LandingHeader";

export function LandingFooter() {
  return (
    <footer data-fsl-theme="dark" className="border-t border-[var(--fsl-line)]">
      <div className="fsl-gutter py-16 sm:py-20">
        <div className="grid gap-12 md:grid-cols-[1.4fr_1fr_1fr_1fr]">
          <div>
            <Link to="/" className="mb-4 flex w-fit items-center gap-2.5 text-[17px] font-semibold">
              <LandingLogo className="h-6 w-6" />
              FlightStrips
            </Link>
            <p className="max-w-xs text-sm leading-relaxed text-[var(--fsl-ink-muted)]">
              A shared electronic strip board for VATSIM controllers. Built for EKCH, open source, free
              to use.
            </p>
            <div className="mt-6 flex gap-5 text-sm text-[var(--fsl-ink-muted)]">
              <a
                href={SITE.github}
                target="_blank"
                rel="noopener noreferrer"
                className="transition-colors hover:text-[var(--fsl-brand-ink)]"
              >
                GitHub
              </a>
              <a
                href={SITE.discord}
                target="_blank"
                rel="noopener noreferrer"
                className="transition-colors hover:text-[var(--fsl-brand-ink)]"
              >
                Discord
              </a>
              <a href={`mailto:${SITE.email}`} className="transition-colors hover:text-[var(--fsl-brand-ink)]">
                Email
              </a>
            </div>
          </div>

          {FOOTER_COLUMNS.map((column) => (
            <div key={column.title}>
              <h2 className="fsl-eyebrow mb-5">{column.title}</h2>
              <ul className="space-y-3">
                {column.links.map((link) => (
                  <li key={link.label}>
                    <a
                      href={link.href}
                      className="text-sm text-[var(--fsl-ink-muted)] transition-colors hover:text-[var(--fsl-brand-ink)]"
                    >
                      {link.label}
                    </a>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>

        <div className="mt-16 flex flex-col gap-4 border-t border-[var(--fsl-line)] pt-8 text-sm text-[var(--fsl-ink-muted)] sm:flex-row sm:items-center sm:justify-between">
          <div className="flex flex-wrap items-center gap-x-5 gap-y-2">
            <span>© {new Date().getFullYear()} FlightStrips</span>
            <Link to="/about" className="transition-colors hover:text-[var(--fsl-brand-ink)]">
              About
            </Link>
            <Link to="/contact" className="transition-colors hover:text-[var(--fsl-brand-ink)]">
              Contact
            </Link>
            <Link to="/privacy" className="transition-colors hover:text-[var(--fsl-brand-ink)]">
              Privacy
            </Link>
            <Link to="/data-handling" className="transition-colors hover:text-[var(--fsl-brand-ink)]">
              Data handling
            </Link>
          </div>
          <p className="text-xs">
            For simulation use only. Not affiliated with VATSIM. Not for real-world operations.
          </p>
        </div>
      </div>
    </footer>
  );
}
