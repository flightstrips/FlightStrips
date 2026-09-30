import { useEffect, useRef, useState, type ComponentProps, type CSSProperties } from "react";
import { Link } from "react-router";
import { ANNOUNCEMENT, FOOTER_COLUMNS, SITE } from "./content";
import { ActionButton, ActionLink, GlowText, TextLink } from "./primitives";
import { useLandingAuth } from "./useLandingAuth";

/** The three-strip mark. Kept exactly as it is elsewhere in the product. */
export function LandingLogo({ className = "" }: { className?: string }) {
  return (
    <svg className={`fsl-logo ${className}`} viewBox="0 0 28 28" fill="none" aria-hidden="true">
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
] as const;

const MOBILE_LINK =
  "fsl-rise fsl-mono border-b border-[var(--fsl-line)] py-5 text-base uppercase tracking-[0.08em]";

const ANNOUNCEMENT_KEY = "fsl-announcement-dismissed";

function isAnnouncementDismissed() {
  try {
    return window.localStorage.getItem(ANNOUNCEMENT_KEY) === ANNOUNCEMENT.id;
  } catch {
    return false;
  }
}

/** Stagger index for `.fsl-rise`. */
const rise = (index: number) => ({ "--i": index }) as CSSProperties;

/** Section links are in-page anchors on the landing page and routes back to it elsewhere. */
function SectionLink({
  home,
  href,
  ...rest
}: { home: boolean; href: string } & Omit<ComponentProps<"a">, "href">) {
  return home ? <a href={href} {...rest} /> : <Link to={`/${href}`} {...rest} />;
}

/**
 * Sticky header on a blurred plate. The announcement hangs below the bar and
 * collapses once the page has moved; the Docs panel drops from the same edge.
 *
 * Off the landing page (`home={false}`) the plate is opaque, since the page
 * beneath is not the dark canvas, and the announcement stays collapsed.
 */
export function LandingHeader({
  home = true,
  themeToggle,
}: {
  home?: boolean;
  /** Supplied by pages whose body follows a light/dark choice. */
  themeToggle?: { theme: "light" | "dark"; onToggle: () => void };
}) {
  const { isAuthenticated, signIn, signOut } = useLandingAuth();
  const [scrolled, setScrolled] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const [docsOpen, setDocsOpen] = useState(false);
  const [dismissed, setDismissed] = useState(isAnnouncementDismissed);
  const headerRef = useRef<HTMLElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const docsTriggerRef = useRef<HTMLButtonElement>(null);
  const docsCloseTimer = useRef<number | undefined>(undefined);

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 30);
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  useEffect(() => {
    if (!menuOpen && !docsOpen) return;

    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      if (docsOpen) docsTriggerRef.current?.focus();
      else triggerRef.current?.focus();
      setMenuOpen(false);
      setDocsOpen(false);
    };

    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [menuOpen, docsOpen]);

  useEffect(() => () => window.clearTimeout(docsCloseTimer.current), []);

  // Hover intent: the pointer has to cross from the trigger to the panel, so
  // closing waits a beat and either element can cancel it.
  const openDocs = () => {
    window.clearTimeout(docsCloseTimer.current);
    setDocsOpen(true);
  };
  const closeDocsSoon = () => {
    window.clearTimeout(docsCloseTimer.current);
    docsCloseTimer.current = window.setTimeout(() => setDocsOpen(false), 140);
  };

  const dismissAnnouncement = () => {
    setDismissed(true);
    try {
      window.localStorage.setItem(ANNOUNCEMENT_KEY, ANNOUNCEMENT.id);
    } catch {
      // Storage unavailable: it stays dismissed for this visit only.
    }
  };

  const announce = home && !dismissed && !scrolled && !menuOpen && !docsOpen;

  return (
    <>
      <header
        ref={headerRef}
        className="fsl-header sticky top-0 z-40"
        data-solid={scrolled || menuOpen || docsOpen}
        data-announce={announce}
        data-opaque={!home}
        onBlur={(event) => {
          if (!headerRef.current?.contains(event.relatedTarget)) setDocsOpen(false);
        }}
      >
        <div className="fsl-header__plate" aria-hidden="true" />

        <div className="fsl-gutter flex h-[var(--fsl-header-h)] items-center justify-between gap-6">
          <div className="flex items-center gap-12 xl:gap-16">
            <Link to="/" className="flex shrink-0 items-center gap-2.5 text-[17px] font-semibold tracking-tight">
              <LandingLogo className="h-6 w-6" />
              FlightStrips
            </Link>

            <nav className="hidden items-center gap-7 lg:flex" aria-label="Primary">
              {NAV.map((item) => (
                <SectionLink key={item.label} home={home} href={item.href} className="fsl-nav-link">
                  {item.label}
                </SectionLink>
              ))}
              <button
                ref={docsTriggerRef}
                type="button"
                className="fsl-nav-link"
                aria-expanded={docsOpen}
                aria-controls="fsl-docs-menu"
                onClick={() => setDocsOpen((open) => !open)}
                onPointerEnter={openDocs}
                onPointerLeave={closeDocsSoon}
              >
                Docs
                <svg className="fsl-nav-link__chevron h-3 w-3" viewBox="0 0 12 12" fill="none" aria-hidden="true">
                  <path d="M3 4.5l3 3 3-3" stroke="currentColor" strokeWidth="1.25" />
                </svg>
              </button>
            </nav>
          </div>

          {themeToggle ? (
            <button
              type="button"
              className="fsl-theme-toggle"
              onClick={themeToggle.onToggle}
              aria-label={themeToggle.theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
            >
              <svg
                key={themeToggle.theme}
                className="h-[18px] w-[18px]"
                viewBox="0 0 24 24"
                fill="none"
                aria-hidden="true"
              >
                {themeToggle.theme === "dark" ? (
                  <path
                    d="M12 3v2m0 14v2M3 12h2m14 0h2M5.6 5.6l1.4 1.4m10 10l1.4 1.4M5.6 18.4L7 17m10-10l1.4-1.4M16 12a4 4 0 11-8 0 4 4 0 018 0z"
                    stroke="currentColor"
                    strokeWidth="1.5"
                    strokeLinecap="round"
                  />
                ) : (
                  <path
                    d="M20 14.5A8 8 0 019.5 4a8 8 0 1010.5 10.5z"
                    stroke="currentColor"
                    strokeWidth="1.5"
                    strokeLinejoin="round"
                  />
                )}
              </svg>
            </button>
          ) : null}

          <div className="hidden items-center gap-3 lg:flex">
            {isAuthenticated ? (
              <>
                <ActionLink label="Open the board" to="/app" />
                <ActionButton label="Sign out" variant="secondary" onClick={signOut} />
              </>
            ) : (
              // Signed out: the only offer is signing in. The board is not
              // something a visitor can open.
              <ActionButton label="Sign in with VATSIM" onClick={signIn} />
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

        <div className="fsl-announce" data-open={announce}>
          <a
            href={ANNOUNCEMENT.href}
            target="_blank"
            rel="noopener noreferrer"
            className="fsl-link fsl-link--ink h-[var(--fsl-announce-h)] max-w-full px-12 text-sm"
          >
            <span
              aria-hidden="true"
              className="h-1.5 w-1.5 shrink-0 rounded-full bg-[var(--fsl-accent)] shadow-[0_0_8px_var(--fsl-accent)]"
            />
            <span className="hidden text-[var(--fsl-ink-muted)] sm:inline">{ANNOUNCEMENT.lead}</span>
            <span className="truncate">
              <GlowText>{ANNOUNCEMENT.label}</GlowText>
            </span>
          </a>
          <button
            type="button"
            aria-label="Dismiss announcement"
            onClick={dismissAnnouncement}
            className="absolute right-[var(--fsl-gutter)] top-1/2 -translate-y-1/2 p-2 text-[var(--fsl-ink-muted)] transition-colors hover:text-[var(--fsl-ink)]"
          >
            <svg className="h-3 w-3" viewBox="0 0 12 12" fill="none" aria-hidden="true">
              <path d="M2 2l8 8M10 2l-8 8" stroke="currentColor" strokeWidth="1.25" />
            </svg>
          </button>
          <span className="fsl-announce__rule" aria-hidden="true" />
        </div>

        <div
          id="fsl-docs-menu"
          className="fsl-menu hidden lg:block"
          data-open={docsOpen}
          onPointerEnter={openDocs}
          onPointerLeave={closeDocsSoon}
        >
          <div className="fsl-gutter grid grid-cols-4 gap-8 py-10">
            <div className="fsl-rise" style={rise(0)}>
              <p className="fsl-display mb-3 text-[28px]">Documentation</p>
              <p className="mb-6 max-w-[26ch] text-sm leading-relaxed text-[var(--fsl-ink-muted)]">
                Every position, procedure and concept on the board.
              </p>
              <TextLink label="Open the docs" href={SITE.docs} ink arrow external />
            </div>

            {FOOTER_COLUMNS.map((column, index) => (
              <div key={column.title} className="fsl-rise" style={rise(index + 1)}>
                <p className="fsl-eyebrow mb-5">{column.title}</p>
                <ul className="space-y-3">
                  {column.links.map((link) => (
                    <li key={link.label}>
                      <TextLink label={link.label} href={link.href} tone="plain" className="text-sm" />
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        </div>
      </header>

      {menuOpen ? (
        <div
          id="fsl-mobile-menu"
          className="fsl-mobile-menu fixed inset-x-0 bottom-0 top-[var(--fsl-header-h)] z-30 overflow-y-auto border-t border-[var(--fsl-line)] bg-[var(--fsl-canvas)] lg:hidden"
        >
          <nav className="fsl-gutter flex flex-col py-4" aria-label="Primary, mobile">
            {NAV.map((item, index) => (
              <SectionLink
                key={item.label}
                home={home}
                href={item.href}
                onClick={() => setMenuOpen(false)}
                className={MOBILE_LINK}
                style={rise(index)}
              >
                {item.label}
              </SectionLink>
            ))}
            <a
              href={SITE.docs}
              target="_blank"
              rel="noopener noreferrer"
              onClick={() => setMenuOpen(false)}
              className={MOBILE_LINK}
              style={rise(NAV.length)}
            >
              Docs
            </a>

            <div className="fsl-rise flex flex-col gap-3 pt-8" style={rise(NAV.length + 1)}>
              {isAuthenticated ? (
                <>
                  <ActionLink label="Open the board" to="/app" className="py-4" />
                  <ActionButton
                    label="Sign out"
                    variant="secondary"
                    className="py-4"
                    onClick={() => {
                      setMenuOpen(false);
                      signOut();
                    }}
                  />
                </>
              ) : (
                <ActionButton
                  label="Sign in with VATSIM"
                  className="py-4"
                  onClick={() => {
                    setMenuOpen(false);
                    signIn();
                  }}
                />
              )}
            </div>
          </nav>
        </div>
      ) : null}
    </>
  );
}
