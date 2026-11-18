import type { ReactNode } from "react";
import { Link } from "react-router";

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
  pending = false,
}: ActionProps & { onClick: () => void; pending?: boolean }) {
  return (
    <button
      type="button"
      onClick={pending ? undefined : onClick}
      aria-disabled={pending || undefined}
      aria-busy={pending || undefined}
      className={`fsl-btn fsl-btn--${variant} ${className}`}
    >
      <span className={pending ? "invisible" : undefined} aria-hidden={pending || undefined}>{label}</span>
      {pending ? <span className="absolute inset-0 flex items-center justify-center">Connecting…</span> : null}
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
      <Link to={target.to} onClick={onClick} className={classes}>
        {label}
      </Link>
    );
  }

  return (
    <a
      href={target.href}
      onClick={onClick}
      className={classes}
      {...(external ? externalProps : {})}
    >
      {label}
    </a>
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
      <span>{label}</span>
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
