import type { ReactNode } from "react";
import { STRIP_FRAME_COLOR } from "./shared";

// ─────────────────────────────────────────────────────────────────────────────
// StripCell
// A single bordered column inside a flight strip.
// ─────────────────────────────────────────────────────────────────────────────

interface StripCellProps {
  children?: ReactNode;
  /** Fixed pixel width. Omit for auto/flex sizing. */
  width?: number | string;
  className?: string;
  /** Right-border colour. Defaults to the strip frame colour. Pass the selection colour when the strip is selected. */
  borderColor?: string;
}

export function StripCell({ children, width, className, borderColor }: StripCellProps) {
  return (
    <div
      className={`border-r-2 h-full flex-shrink-0 overflow-hidden ${className ?? ""}`}
      style={{
        ...(width !== undefined ? { width } : {}),
        borderRightColor: borderColor ?? STRIP_FRAME_COLOR,
      }}
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
  /** Border colour for right edge and horizontal divider. Defaults to the strip frame colour. */
  borderColor?: string;
}

export function SplitStripCell({
  top,
  bottom,
  width,
  className,
  topClassName,
  bottomClassName,
  borderColor,
}: SplitStripCellProps) {
  return (
    <StripCell width={width} className={`flex flex-col ${className ?? ""}`} borderColor={borderColor}>
      <div
        className={`h-1/2 flex items-center border-b-2 overflow-hidden ${topClassName ?? ""}`}
        style={{ borderBottomColor: borderColor ?? STRIP_FRAME_COLOR }}
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
