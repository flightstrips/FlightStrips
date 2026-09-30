import { useEffect, useRef, useState, type PointerEvent, type ReactNode } from "react";
import { Link } from "react-router";

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

/** Point a button's gradient at the cursor (see `.fsl-btn` in landing.css). */
function trackGlow(event: PointerEvent<HTMLElement>) {
  const target = event.currentTarget;
  const rect = target.getBoundingClientRect();
  target.style.setProperty("--glow-x", `${((event.clientX - rect.left) / rect.width) * 100}%`);
  target.style.setProperty("--glow-y", `${((event.clientY - rect.top) / rect.height) * 100}%`);
}

function resetGlow(event: PointerEvent<HTMLElement>) {
  event.currentTarget.style.removeProperty("--glow-x");
  event.currentTarget.style.removeProperty("--glow-y");
}

const glowHandlers = { onPointerMove: trackGlow, onPointerLeave: resetGlow };

const externalProps = { target: "_blank", rel: "noopener noreferrer" };

type ActionProps = {
  label: string;
  variant?: "primary" | "secondary";
  className?: string;
};

export function ActionButton({
  label,
  variant = "primary",
  className = "",
  onClick,
}: ActionProps & { onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`fsl-btn fsl-btn--${variant} ${className}`}
      {...glowHandlers}
    >
      {label}
    </button>
  );
}

/** `to` routes inside the app; `href` is a plain anchor. */
export function ActionLink({
  label,
  variant = "primary",
  className = "",
  external = false,
  onClick,
  ...target
}: ActionProps & { external?: boolean; onClick?: () => void } & ({ href: string } | { to: string })) {
  const classes = `fsl-btn fsl-btn--${variant} ${className}`;

  if ("to" in target) {
    return (
      <Link to={target.to} onClick={onClick} className={classes} {...glowHandlers}>
        {label}
      </Link>
    );
  }

  return (
    <a
      href={target.href}
      onClick={onClick}
      className={classes}
      {...glowHandlers}
      {...(external ? externalProps : {})}
    >
      {label}
    </a>
  );
}

/**
 * A label that is swept by the brand gradient when its link is hovered. The
 * label is rendered twice; the travelling copy is hidden from assistive tech
 * so the link announces its text once.
 */
export function GlowText({ children }: { children: string }) {
  return (
    <span className="fsl-glow">
      <span>{children}</span>
      <span className="fsl-glow__pass" aria-hidden="true">
        {children}
      </span>
    </span>
  );
}

function Arrow() {
  return (
    <svg className="fsl-link__arrow" viewBox="0 0 12 7" fill="none" aria-hidden="true">
      <path
        d="M8.59 0l-.7.72 2.14 2.14H0v1h10.03L7.89 6l.7.72 3.36-3.36L8.59 0z"
        fill="currentColor"
      />
    </svg>
  );
}

type TextLinkProps = {
  label: string;
  /** `mono` is the small uppercase treatment; `plain` inherits the text style. */
  tone?: "mono" | "plain";
  /** Rest at full ink instead of muted. */
  ink?: boolean;
  arrow?: boolean;
  external?: boolean;
  className?: string;
  onClick?: () => void;
} & ({ href: string } | { to: string });

export function TextLink({
  label,
  tone = "mono",
  ink = false,
  arrow = false,
  external = false,
  className = "",
  onClick,
  ...target
}: TextLinkProps) {
  const classes = `fsl-link ${tone === "mono" ? "fsl-link--mono" : ""} ${ink ? "fsl-link--ink" : ""} ${className}`;
  const content = (
    <>
      <GlowText>{label}</GlowText>
      {arrow ? <Arrow /> : null}
    </>
  );

  if ("to" in target) {
    return (
      <Link to={target.to} onClick={onClick} className={classes}>
        {content}
      </Link>
    );
  }

  return (
    <a href={target.href} onClick={onClick} className={classes} {...(external ? externalProps : {})}>
      {content}
    </a>
  );
}
