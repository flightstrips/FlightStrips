import type { ReactNode } from "react";

// ─────────────────────────────────────────────────────────────────────────────
// StripCell
// A single bordered column inside a flight strip.
// ─────────────────────────────────────────────────────────────────────────────

interface StripCellProps {
  children?: ReactNode;
  /** Fixed pixel width. Omit for auto/flex sizing. */
  width?: number | string;
  className?: string;
}

export function StripCell({ children, width, className }: StripCellProps) {
  return (
    <div
      className={`border-r border-[#85b4af] h-full flex-shrink-0 overflow-hidden ${className ?? ""}`}
      style={width !== undefined ? { width } : undefined}
    >
      {children}
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// SplitStripCell
// A StripCell divided horizontally into top and bottom halves.
// ─────────────────────────────────────────────────────────────────────────────

interface SplitStripCellProps {
  top: ReactNode;
  bottom: ReactNode;
  width?: number | string;
  className?: string;
  /** Extra classes applied only to the top half. */
  topClassName?: string;
  /** Extra classes applied only to the bottom half. */
  bottomClassName?: string;
}

export function SplitStripCell({
  top,
  bottom,
  width,
  className,
  topClassName,
  bottomClassName,
}: SplitStripCellProps) {
  return (
    <StripCell width={width} className={`flex flex-col ${className ?? ""}`}>
      <div
        className={`h-1/2 flex items-center border-b border-[#85b4af] overflow-hidden ${topClassName ?? ""}`}
      >
        {top}
      </div>
      <div
        className={`h-1/2 flex items-center overflow-hidden ${bottomClassName ?? ""}`}
      >
        {bottom}
      </div>
    </StripCell>
  );
}
