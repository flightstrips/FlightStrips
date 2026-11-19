import { useEffect, useRef, useState, type ComponentProps } from "react";
import { Link, useLocation } from "react-router";
import { FOOTER_COLUMNS, SITE } from "./content";
import { ActionButton, ActionLink, TextLink } from "./primitives";
import { useLandingAuth } from "./useLandingAuth";

/** The product's three-strip mark. */
export function LandingLogo({ className = "" }: { className?: string }) {
  return (
    <svg
      className={`fsl-logo ${className}`}
      viewBox="0 0 28 28"
      fill="none"
      aria-hidden="true"
    >
      <rect x="3" y="6" width="22" height="4" rx="1" fill="#a0dae4" />
      <rect
        x="3"
        y="12"
        width="22"
        height="4"
        rx="1"
        fill="#a0dae4"
        opacity="0.7"
      />
      <rect
        x="3"
        y="18"
        width="22"
        height="4"
        rx="1"
        fill="#a0dae4"
        opacity="0.4"
      />
    </svg>
  );
}

const NAV = [
  { label: "Set up", href: "#setup" },
  { label: "Positions", href: "#positions" },
  { label: "How it connects", href: "#architecture" },
  { label: "Airports", href: "#airports" },
] as const;

function SectionLink({
  home,
  href,
  ...rest
}: { home: boolean; href: string } & Omit<ComponentProps<"a">, "href">) {
  return home ? (
    <a href={href} {...rest} />
  ) : (
    <Link to={`/${href}`} {...rest} />
  );
}

export function LandingHeader({
  home = true,
  themeToggle,
}: {
  home?: boolean;
  themeToggle?: { theme: "light" | "dark"; onToggle: () => void };
}) {
  const { isAuthenticated, isSigningIn, signInError, signIn, signOut } = useLandingAuth();
  const [menuOpen, setMenuOpen] = useState(false);
  const [docsOpen, setDocsOpen] = useState(false);
  const headerRef = useRef<HTMLElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const dialogRef = useRef<HTMLDialogElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);
  const docsTriggerRef = useRef<HTMLButtonElement>(null);
  const location = useLocation();

  // A native modal makes the background inert and contains keyboard focus.
  useEffect(() => {
    if (!menuOpen) return;
    const dialog = dialogRef.current;
    if (!dialog) return;
    const root = document.documentElement;
    const previousOverflow = root.style.overflow;
    const trigger = triggerRef.current;
    dialog.showModal();
    root.style.overflow = "hidden";
    closeRef.current?.focus();
    const desktop = window.matchMedia("(min-width: 1024px)");
    const onResize = () => {
      if (desktop.matches) setMenuOpen(false);
    };
    desktop.addEventListener("change", onResize);
    return () => {
      dialog.close();
      root.style.overflow = previousOverflow;
      desktop.removeEventListener("change", onResize);
      trigger?.focus({ preventScroll: true });
    };
  }, [menuOpen]);

  useEffect(() => {
    if (!docsOpen) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setDocsOpen(false);
        docsTriggerRef.current?.focus();
      }
    };
    const onOutside = (event: PointerEvent) => {
      if (!headerRef.current?.contains(event.target as Node))
        setDocsOpen(false);
    };
    document.addEventListener("keydown", onKey);
    document.addEventListener("pointerdown", onOutside);
    const desktop = window.matchMedia("(min-width: 1024px)");
    const onResize = () => {
      if (!desktop.matches) setDocsOpen(false);
    };
    desktop.addEventListener("change", onResize);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.removeEventListener("pointerdown", onOutside);
      desktop.removeEventListener("change", onResize);
    };
  }, [docsOpen]);

  useEffect(() => {
    setMenuOpen(false);
    setDocsOpen(false);
  }, [location.key]);

  const accountActions = (
    <>
      {isAuthenticated ? (
        <>
          <ActionLink
            label="Open the board"
            to="/app"
            onClick={() => setMenuOpen(false)}
          />
          <ActionButton
            label="Sign out"
            variant="secondary"
            onClick={() => {
              setMenuOpen(false);
              signOut();
            }}
          />
        </>
      ) : (
        <ActionButton
          label="Sign in with VATSIM"
          pending={isSigningIn}
          onClick={signIn}
        />
      )}
    </>
  );

  return (
    <>
      <header
        ref={headerRef}
        className="fsl-header sticky top-0 z-40"
        onBlur={(event) => {
          if (!headerRef.current?.contains(event.relatedTarget))
            setDocsOpen(false);
        }}
      >
        <div className="fsl-gutter flex h-[var(--fsl-header-h)] items-center justify-between gap-4">
          <Link
            to="/"
            className="fsl-brand flex shrink-0 items-center gap-3 text-[17px] font-semibold"
          >
            <LandingLogo className="h-6 w-6" />
            FlightStrips
          </Link>
          <nav
            className="hidden items-center gap-6 lg:flex"
            aria-label="Primary"
          >
            {NAV.map((item) => (
              <SectionLink
                key={item.href}
                home={home}
                href={item.href}
                className="fsl-nav-link"
              >
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
            >
              Docs
              <svg
                className="fsl-nav-link__chevron h-3 w-3"
                viewBox="0 0 12 12"
                fill="none"
                aria-hidden="true"
              >
                <path
                  d="M3 4.5l3 3 3-3"
                  stroke="currentColor"
                  strokeWidth="1.25"
                />
              </svg>
            </button>
          </nav>
          <div className="ml-auto flex items-center gap-3 lg:ml-0">
            {themeToggle ? (
              <button
                type="button"
                className="fsl-theme-toggle"
                onClick={themeToggle.onToggle}
                aria-label={
                  themeToggle.theme === "dark"
                    ? "Switch to light theme"
                    : "Switch to dark theme"
                }
              >
                <svg
                  className="h-5 w-5"
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
              {accountActions}
            </div>
            <button
              ref={triggerRef}
              type="button"
              className="fsl-menu-trigger lg:hidden"
              aria-expanded={menuOpen}
              aria-controls="fsl-mobile-menu"
              aria-label="Open menu"
              onClick={() => {
                setDocsOpen(false);
                setMenuOpen(true);
              }}
            >
              Menu
            </button>
          </div>
        </div>
        <div id="fsl-docs-menu" className="fsl-menu" hidden={!docsOpen}>
          <div className="fsl-gutter grid grid-cols-4 gap-8 py-8">
            <div>
              <p className="mb-3 text-xl font-medium">Documentation</p>
              <p className="mb-4 max-w-[26ch] text-sm leading-relaxed text-[var(--fsl-ink-muted)]">
                Every position, procedure and concept on the board.
              </p>
              <TextLink label="Open the docs" href={SITE.docs} ink arrow />
            </div>
            {FOOTER_COLUMNS.map((column) => (
              <div key={column.title}>
                <p className="fsl-eyebrow mb-3">{column.title}</p>
                <ul>
                  {column.links.map((link) => (
                    <li key={link.label}>
                      <TextLink
                        label={link.label}
                        href={link.href}
                        tone="plain"
                        className="text-sm"
                      />
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        </div>
        {signInError && !menuOpen ? (
          <div className="fsl-gutter pb-4"><p role="alert" className="fsl-auth-error">{signInError}</p></div>
        ) : null}
      </header>
      <dialog
        ref={dialogRef}
        id="fsl-mobile-menu"
        className="fsl-mobile-menu"
        aria-label="Navigation"
        onCancel={(event) => {
          event.preventDefault();
          setMenuOpen(false);
        }}
        onClose={() => setMenuOpen(false)}
        onKeyDown={(event) => {
          if (event.key !== "Tab") return;
          const targets = event.currentTarget.querySelectorAll<HTMLElement>(
            "a[href], button:not([disabled])",
          );
          const first = targets[0];
          const last = targets[targets.length - 1];
          if (event.shiftKey && document.activeElement === first) {
            event.preventDefault();
            last?.focus();
          } else if (!event.shiftKey && document.activeElement === last) {
            event.preventDefault();
            first?.focus();
          }
        }}
      >
        <div className="fsl-gutter flex h-[var(--fsl-header-h)] items-center justify-between border-b border-[var(--fsl-line)]">
          <span className="flex items-center gap-3 font-semibold">
            <LandingLogo className="h-6 w-6" />
            FlightStrips
          </span>
          <button
            ref={closeRef}
            type="button"
            className="fsl-menu-trigger"
            aria-label="Close menu"
            onClick={() => setMenuOpen(false)}
          >
            Close
          </button>
        </div>
        <nav
          className="fsl-gutter flex flex-col gap-1 py-6"
          aria-label="Primary, mobile"
        >
          {NAV.map((item) => (
            <SectionLink
              key={item.href}
              home={home}
              href={item.href}
              onClick={() => setMenuOpen(false)}
              className="fsl-mobile-link"
            >
              {item.label}
            </SectionLink>
          ))}
          <a
            href={SITE.docs}
            className="fsl-mobile-link"
            onClick={() => setMenuOpen(false)}
          >
            Docs
          </a>
          <div className="flex flex-col gap-3 pt-6">{accountActions}</div>
          {signInError ? <p role="alert" className="fsl-auth-error">{signInError}</p> : null}
        </nav>
      </dialog>
    </>
  );
}
