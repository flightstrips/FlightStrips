import { useEffect, useRef, useState, type ReactNode } from "react";

/**
 * Arm the reveal animation. Set at module scope so it lands before the first
 * paint (no flash of visible-then-hidden), and stays unset when this bundle
 * never runs — in which case the CSS leaves revealed content plainly visible.
 */
if (typeof document !== "undefined") {
  document.documentElement.dataset.fslJs = "true";
}

/** Section themes. A section owns its canvas so adjacency drives spacing. */
export type LandingTheme = "dark" | "light";

type SectionProps = {
  theme?: LandingTheme;
  /** Draw the decorative column rules behind the content. */
  guides?: boolean;
  /** Hairline rule along the top edge. */
  topRule?: boolean;
  id?: string;
  className?: string;
  children: ReactNode;
};

/**
 * Every major band on the page is a Section: local theme, optional guide
 * columns, hairline boundary, then a gutter wrapper around the content.
 */
export function Section({
  theme = "dark",
  guides = false,
  topRule = true,
  id,
  className = "",
  children,
}: SectionProps) {
  return (
    <section
      id={id}
      data-fsl-theme={theme}
      className={`relative isolate ${topRule ? "border-t border-[var(--fsl-line)]" : ""} ${className}`}
    >
      {guides ? <GuideColumns /> : null}
      <div className="fsl-gutter relative z-10">{children}</div>
    </section>
  );
}

/** Five 1px rules aligned to the 16/8-column grid (columns 1, 5, 9, 13, 17). */
export function GuideColumns() {
  return (
    <div className="fsl-guides" aria-hidden="true">
      <div className="fsl-guides__inner">
        <span style={{ left: 0 }} />
        <span data-inner="true" style={{ left: "25%" }} />
        <span data-inner="true" style={{ left: "50%" }} />
        <span data-inner="true" style={{ left: "75%" }} />
        <span style={{ right: 0 }} />
      </div>
    </div>
  );
}

/** Small uppercase mono label that opens most sections. */
export function Eyebrow({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <p className={`fsl-eyebrow ${className}`}>{children}</p>;
}

/**
 * Fade-and-rise on first intersection. Content is fully readable without
 * JavaScript and the transition is dropped under reduced motion (see CSS).
 */
export function Reveal({
  children,
  delay = 0,
  className = "",
}: {
  children: ReactNode;
  delay?: number;
  className?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    const node = ref.current;
    if (!node) return;

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            setVisible(true);
            observer.disconnect();
          }
        }
      },
      { threshold: 0.15, rootMargin: "0px 0px -60px 0px" },
    );

    observer.observe(node);
    return () => observer.disconnect();
  }, []);

  return (
    <div
      ref={ref}
      className={`fsl-reveal ${className}`}
      data-visible={visible || undefined}
      style={{ transitionDelay: `${delay}ms` }}
    >
      {children}
    </div>
  );
}

type SwapProps = {
  label: string;
  variant?: "primary" | "secondary";
  className?: string;
};

/**
 * The label is rendered twice: the resting face leaves upward while the
 * inverted face arrives from below. The duplicate is hidden from assistive
 * tech so the control announces its label once.
 */
function SwapFaces({ label }: { label: string }) {
  return (
    <>
      <span className="fsl-swap__face fsl-swap__face--rest">{label}</span>
      <span className="fsl-swap__face fsl-swap__face--hover" aria-hidden="true">
        {label}
      </span>
    </>
  );
}

export function SwapButton({
  label,
  variant = "primary",
  className = "",
  onClick,
}: SwapProps & { onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} className={`fsl-swap fsl-swap--${variant} ${className}`}>
      <SwapFaces label={label} />
    </button>
  );
}

export function SwapLink({
  label,
  href,
  variant = "primary",
  className = "",
  external = false,
}: SwapProps & { href: string; external?: boolean }) {
  return (
    <a
      href={href}
      className={`fsl-swap fsl-swap--${variant} ${className}`}
      {...(external ? { target: "_blank", rel: "noopener noreferrer" } : {})}
    >
      <SwapFaces label={label} />
    </a>
  );
}
