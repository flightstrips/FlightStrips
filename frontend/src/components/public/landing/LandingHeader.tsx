import { useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { SITE } from "./content";
import { useLandingAuth } from "./useLandingAuth";

/** The three-strip mark. Kept exactly as it is elsewhere in the product. */
export function LandingLogo({ className = "" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 28 28" fill="none" aria-hidden="true">
      <rect x="3" y="6" width="22" height="4" rx="1" fill="#a0dae4" />
      <rect x="3" y="12" width="22" height="4" rx="1" fill="#a0dae4" opacity="0.7" />
      <rect x="3" y="18" width="22" height="4" rx="1" fill="#a0dae4" opacity="0.4" />
    </svg>
  );
}

const NAV = [
  { label: "Positions", href: "#positions" },
  { label: "How it connects", href: "#architecture" },
  { label: "Airports", href: "#airports" },
  { label: "Docs", href: SITE.docs, external: true },
] as const;

/**
 * Fixed header: transparent over the hero, solid once the page has moved.
 * The announcement strip above it collapses on the same threshold.
 */
export function LandingHeader() {
  const { isAuthenticated, signIn, signOut } = useLandingAuth();
  const [scrolled, setScrolled] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 30);
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  useEffect(() => {
    if (!menuOpen) return;

    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setMenuOpen(false);
        triggerRef.current?.focus();
      }
    };

    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [menuOpen]);

  const solid = scrolled || menuOpen;

  return (
    <>
      <header
        className={`sticky top-0 z-40 border-b transition-colors duration-300 ease-out ${
          solid
            ? "border-[var(--fsl-line)] bg-[var(--fsl-canvas)]/95 backdrop-blur-md"
            : "border-transparent bg-transparent"
        }`}
      >
        <div className="fsl-gutter flex h-[var(--fsl-header-h)] items-center justify-between gap-6">
          <Link to="/" className="flex shrink-0 items-center gap-2.5 text-[17px] font-semibold tracking-tight">
            <LandingLogo className="h-6 w-6" />
            FlightStrips
          </Link>

          <nav className="hidden items-center gap-8 lg:flex" aria-label="Primary">
            {NAV.map((item) => (
              <a
                key={item.label}
                href={item.href}
                className="text-sm text-[var(--fsl-ink-muted)] transition-colors hover:text-[var(--fsl-ink)]"
                {...("external" in item && item.external
                  ? { target: "_blank", rel: "noopener noreferrer" }
                  : {})}
              >
                {item.label}
              </a>
            ))}
          </nav>

          <div className="hidden items-center gap-3 lg:flex">
            {isAuthenticated ? (
              <>
                <Link
                  to="/app"
                  className="border border-[var(--fsl-line-strong)] px-4 py-2 text-sm transition-colors hover:border-[var(--fsl-brand-ink)] hover:text-[var(--fsl-brand-ink)]"
                >
                  Open the board
                </Link>
                <button
                  type="button"
                  onClick={signOut}
                  className="px-2 text-sm text-[var(--fsl-ink-muted)] transition-colors hover:text-[var(--fsl-ink)]"
                >
                  Sign out
                </button>
              </>
            ) : (
              // Signed out: the only offer is signing in. The board is not
              // something a visitor can open.
              <button
                type="button"
                onClick={signIn}
                className="bg-[var(--fsl-accent)] px-4 py-2 text-sm font-medium text-[#04100f] transition-colors hover:bg-white"
              >
                Sign in with VATSIM
              </button>
            )}
          </div>

          <button
            ref={triggerRef}
            type="button"
            className="lg:hidden"
            aria-expanded={menuOpen}
            aria-controls="fsl-mobile-menu"
            aria-label={menuOpen ? "Close menu" : "Open menu"}
            onClick={() => setMenuOpen((open) => !open)}
          >
            <span className="fsl-mono text-[11px] uppercase tracking-[0.18em]">
              {menuOpen ? "Close" : "Menu"}
            </span>
          </button>
        </div>
      </header>

      {menuOpen ? (
        <div
          id="fsl-mobile-menu"
          className="fixed inset-x-0 bottom-0 top-[var(--fsl-header-h)] z-30 overflow-y-auto border-t border-[var(--fsl-line)] bg-[var(--fsl-canvas)] lg:hidden"
        >
          <nav className="fsl-gutter flex flex-col py-4" aria-label="Primary, mobile">
            {NAV.map((item) => (
              <a
                key={item.label}
                href={item.href}
                onClick={() => setMenuOpen(false)}
                className="border-b border-[var(--fsl-line)] py-5 text-lg"
                {...("external" in item && item.external
                  ? { target: "_blank", rel: "noopener noreferrer" }
                  : {})}
              >
                {item.label}
              </a>
            ))}

            <div className="flex flex-col gap-3 pt-8">
              {isAuthenticated ? (
                <>
                  <Link
                    to="/app"
                    className="bg-[var(--fsl-accent)] px-4 py-3.5 text-center text-sm font-medium text-[#04100f]"
                  >
                    Open the board
                  </Link>
                  <button
                    type="button"
                    onClick={() => {
                      setMenuOpen(false);
                      signOut();
                    }}
                    className="border border-[var(--fsl-line-strong)] px-4 py-3.5 text-sm"
                  >
                    Sign out
                  </button>
                </>
              ) : (
                <button
                  type="button"
                  onClick={() => {
                    setMenuOpen(false);
                    signIn();
                  }}
                  className="bg-[var(--fsl-accent)] px-4 py-3.5 text-sm font-medium text-[#04100f]"
                >
                  Sign in with VATSIM
                </button>
              )}
            </div>
          </nav>
        </div>
      ) : null}
    </>
  );
}
