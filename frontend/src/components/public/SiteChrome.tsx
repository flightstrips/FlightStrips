import type { ReactNode } from "react";
import "./landing/landing.css";

import { setStoredPublicTheme, usePublicTheme } from "@/lib/public-theme";
import { LandingFooter } from "./landing/LandingFooter";
import { LandingHeader } from "./landing/LandingHeader";
import { Eyebrow, GuideColumns, Reveal } from "./landing/primitives";

function useThemeToggle() {
  const theme = usePublicTheme();
  return { theme, onToggle: () => setStoredPublicTheme(theme === "dark" ? "light" : "dark") };
}

/**
 * Shell for the public pages other than the landing page: the landing header
 * and footer around a body that follows the visitor's light/dark choice.
 */
export function PublicPage({ children }: { children: ReactNode }) {
  const themeToggle = useThemeToggle();

  return (
    <div className="fsl">
      <div data-fsl-theme={themeToggle.theme} className="flex min-h-screen flex-col">
        <LandingHeader home={false} themeToggle={themeToggle} />
        <main className="flex-1">{children}</main>
        <LandingFooter />
      </div>
    </div>
  );
}

/**
 * Header and footer on their own, for pages that keep their own body styling.
 * `.fsl` only supplies the design tokens here.
 */
export function SiteHeader() {
  const themeToggle = useThemeToggle();

  // `contents` so the wrappers do not become the sticky header's scroll box.
  return (
    <div className="fsl contents">
      <div data-fsl-theme={themeToggle.theme} className="contents">
        <LandingHeader home={false} themeToggle={themeToggle} />
      </div>
    </div>
  );
}

export function SiteFooter() {
  return (
    <div className="fsl mt-auto">
      <LandingFooter />
    </div>
  );
}

/** Opening band of a public page. */
export function PageHero({
  eyebrow,
  title,
  lead,
  children,
}: {
  eyebrow: string;
  title: string;
  lead?: string;
  children?: ReactNode;
}) {
  return (
    <section className="relative isolate border-b border-[var(--fsl-line)]">
      <GuideColumns />
      <div className="fsl-gutter relative z-10 py-16 sm:py-24">
        <Reveal>
          <Eyebrow className="mb-5">{eyebrow}</Eyebrow>
          <h1 className="fsl-display max-w-[18ch] text-[40px] sm:text-[56px] lg:text-[72px]">{title}</h1>
          {lead ? (
            <p className="mt-6 max-w-2xl text-[17px] leading-relaxed text-[var(--fsl-ink-muted)]">{lead}</p>
          ) : null}
          {children}
        </Reveal>
      </div>
    </section>
  );
}
