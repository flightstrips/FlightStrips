import { Link } from "react-router";
import { LandingLogo } from "./LandingHeader";
import { TextLink } from "./primitives";

const LEGAL = [
  { label: "About", to: "/about" },
  { label: "Contact", to: "/contact" },
  { label: "Privacy", to: "/privacy" },
  { label: "Data handling", to: "/data-handling" },
] as const;

export function LandingFooter() {
  return (
    <footer data-fsl-theme="dark" className="fsl-footer flex flex-col border-t border-[var(--fsl-line)]">
      <div className="fsl-rails" aria-hidden="true">
        <span />
        <span />
        <span />
      </div>

      <div className="fsl-gutter py-14 sm:py-20">
        <div>
          <Link to="/" className="fsl-footer__wordmark flex w-fit items-center gap-[0.18em]">
            <LandingLogo className="h-[0.8em] w-[0.8em]" />
            FlightStrips
          </Link>
          <p className="mt-6 max-w-xs text-sm leading-relaxed text-[var(--fsl-ink-muted)]">
            A shared EFS (Electric Flight System) for VATSIM controllers. Built for EKCH, open source, free
            to use.
          </p>

          <div className="mt-10 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex flex-wrap items-center gap-x-6 gap-y-3">
              {LEGAL.map((link) => (
                <TextLink key={link.label} label={link.label} to={link.to} />
              ))}
            </div>
            <p className="fsl-mono text-[11px] uppercase tracking-[0.08em] text-[var(--fsl-ink-muted)]">
              © {new Date().getFullYear()} FlightStrips. For simulation use only. Not affiliated with VATSIM.
            </p>
          </div>
        </div>
      </div>
    </footer>
  );
}
