import { useState, useRef, useEffect } from "react";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import * as VisuallyHidden from "@radix-ui/react-visually-hidden";
import { useDragDisabled } from "@/components/bays/DragDisabledContext";
import type { ClickPoint, StaticMapPoint, VisibilityContext } from "@/config/ekch";

// Map dialog color constants (used in CSSProperties style objects)
const COLOR_MAP_BTN_BG    = "#D6D6D6"; // light grey button background
const COLOR_MAP_BTN_DARK  = "#3F3F3F"; // dark button (OK/ERASE)
const COLOR_MAP_INPUT_BG  = "#D6D6D6"; // text input background (same as button)
const COLOR_DIALOG_BORDER = "#888";    // dialog shell border
// eslint-disable-next-line react-refresh/only-export-components
export const MAP_POINT_STROKE_STYLE: React.CSSProperties = {
  borderBottom: "1px solid #333",
  borderRight: "1px solid #333",
};
const CONTROL_WIDE_BTN_WIDTH = "4.69cqw";
const CONTROL_NARROW_BTN_WIDTH = "4.17cqw";
const CONTROL_GAP = "0.52cqh";

/** Shared button base — each dialog extends this with its own width/height/fontSize. */
// eslint-disable-next-line react-refresh/only-export-components
export const MAP_BTN_BASE: React.CSSProperties = {
  backgroundColor: COLOR_MAP_BTN_BG,
  fontFamily: "var(--font-bay)",
  fontWeight: "bold",
  color: "black",
  border: "none",
  cursor: "pointer",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  userSelect: "none",
  flexShrink: 0,
  boxShadow: "0 4px 4px rgba(0,0,0,0.25)",
};

const MAP_CLOSE_BTN_BASE: React.CSSProperties = {
  ...MAP_BTN_BASE,
  width: "max(70px, 4.69cqw)",
  height: "max(40px, 3.95cqh)",
  fontSize: "max(20px, 2cqh)",
};

const DIALOG_CONTENT_STYLE: React.CSSProperties = {
  position: "fixed",
  left: 10,
  right: 10,
  top: "50%",
  transform: "translateY(-50%)",
  zIndex: 51,
  outline: "none",
  overflow: "hidden",
  maxHeight: "calc(100dvh - 20px)",
  maxWidth: "calc(100vw - 20px)",
};

// ---------------------------------------------------------------------------
// MapEraseControls — ERASE button, text input, and OK button
// ---------------------------------------------------------------------------

interface MapEraseControlsProps {
  onOk: (value: string) => void;
  onErase?: () => void;
  onClose?: () => void;
  btnStyle: React.CSSProperties;
  maxLength?: number;
  canSelect?: (value: string) => boolean;
  disabledMessage?: string;
}

export function MapEraseControls({ onOk, onErase, onClose, btnStyle, maxLength = 6, canSelect, disabledMessage }: MapEraseControlsProps) {
  const [typed, setTyped] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const isTypedSelectionBlocked = !!typed.trim() && canSelect !== undefined && !canSelect(typed.trim().toUpperCase());

  const handleOk = () => {
    const value = typed.trim().toUpperCase();
    if (value && (!canSelect || canSelect(value))) {
      onOk(value);
      setTyped("");
    }
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: CONTROL_GAP, width: "fit-content" }}>
      <div style={{ display: "flex", gap: CONTROL_GAP, alignItems: "center" }}>
        <button
          onClick={() => { setTyped(""); onErase?.(); }}
          style={{ ...btnStyle, width: CONTROL_WIDE_BTN_WIDTH, backgroundColor: COLOR_MAP_BTN_DARK, color: "white" }}
        >
          ERASE
        </button>
        <input
          ref={inputRef}
          value={typed}
          onChange={(e) => setTyped(e.target.value.toUpperCase())}
          onKeyDown={(e) => e.key === "Enter" && handleOk()}
          maxLength={maxLength}
          style={{
            width: CONTROL_NARROW_BTN_WIDTH,
            height: btnStyle.height,
            fontFamily: "var(--font-bay)",
            fontWeight: "bold",
            fontSize: "2cqh",
            textAlign: "center",
            border: "none",
            outline: "none",
            margin: 0,
            padding: 0,
            backgroundColor: COLOR_MAP_INPUT_BG,
            textTransform: "uppercase",
          }}
        />
      </div>
      <div style={{ display: "flex", justifyContent: onClose ? "space-between" : "flex-end", gap: CONTROL_GAP, width: "100%" }}>
        {onClose && (
          <button
            onClick={onClose}
            style={{ ...btnStyle, width: CONTROL_WIDE_BTN_WIDTH, backgroundColor: COLOR_MAP_BTN_DARK, color: "white" }}
          >
            ESC
          </button>
        )}
        <button
          onClick={handleOk}
          disabled={isTypedSelectionBlocked}
          title={isTypedSelectionBlocked ? disabledMessage : undefined}
          style={{ ...btnStyle, backgroundColor: COLOR_MAP_BTN_DARK, color: "white", width: CONTROL_NARROW_BTN_WIDTH }}
        >
          OK
        </button>
      </div>
    </div>
  );
}

interface MapCloseButtonProps {
  onClose: () => void;
  btnStyle?: React.CSSProperties;
  label?: string;
}

export function MapCloseButton({ onClose, btnStyle, label = "ESC" }: MapCloseButtonProps) {
  return (
    <button
      onClick={onClose}
      style={{ ...MAP_CLOSE_BTN_BASE, ...btnStyle, backgroundColor: COLOR_MAP_BTN_DARK, color: "white" }}
    >
      {label}
    </button>
  );
}

// ---------------------------------------------------------------------------
// MapDialogShell — dialog wrapper, drag-disabled guard, image + point buttons
// ---------------------------------------------------------------------------

interface MapDialogShellProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  imageSrc: string;
  imageAlt: string;
  /** Pixel width of the PNG — used to set the correct aspect ratio. */
  imgWidth: number;
  /** Pixel height of the PNG — used to set the correct aspect ratio. */
  imgHeight: number;
  points: ClickPoint[];
  /** Non-interactive labels positioned over the map, such as runway TODA distances. */
  staticPoints?: StaticMapPoint[];
  pointStyle?: React.CSSProperties;
  btnStyle: React.CSSProperties;
  onSelect: (label: string) => void;
  selectedPoint?: string;
  /** Controls panel overlay (e.g. arrows + ERASE/OK). Positioned absolutely over the image. */
  children?: React.ReactNode;
  /**
   * "height" (default) — scales so the dialog fits within the viewport height.
   * "width" — scales so the dialog matches imgWidth pixels at a 1920px-wide viewport,
   *            shrinking proportionally on narrower screens (still capped by viewport height).
   */
  scaleMode?: "height" | "width";
  /**
   * When provided, points with a `visible` predicate are filtered against this context,
   * and points with a function `label` are resolved to a string.
   * When omitted, all points render with their static label (backwards compatible).
   */
  visibilityContext?: VisibilityContext;
  isPointDisabled?: (label: string, point: ClickPoint) => boolean;
  disabledPointMessage?: string;
}

export function MapDialogShell({
  open, onOpenChange, title,
  imageSrc, imageAlt, imgWidth, imgHeight,
  points, staticPoints = [], pointStyle, btnStyle, onSelect,
  selectedPoint,
  children,
  scaleMode = "height",
  visibilityContext,
  isPointDisabled,
  disabledPointMessage,
}: MapDialogShellProps) {
  const [hoveredRestrictedPoint, setHoveredRestrictedPoint] = useState<number | null>(null);
  const { setDragDisabled } = useDragDisabled();

  useEffect(() => {
    setDragDisabled(open);
    return () => setDragDisabled(false);
  }, [open, setDragDisabled]);

  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay
          style={{ position: "fixed", inset: 0, zIndex: 50 }}
          onClick={(e) => e.stopPropagation()}
        />

        <DialogPrimitive.Content
          className="animate-dialog-zoom-in"
          style={DIALOG_CONTENT_STYLE}
          onClick={(e) => e.stopPropagation()}
        >
          <VisuallyHidden.Root>
            <DialogPrimitive.Title>{title}</DialogPrimitive.Title>
          </VisuallyHidden.Root>

          <div
            style={{
              position: "relative",
              width: "100%",
              aspectRatio: `${imgWidth} / ${imgHeight}`,
              maxHeight: "calc(100dvh - 20px)",
              maxWidth: scaleMode === "width"
                ? `min(calc(100vw * ${imgWidth} / 1920), calc((100dvh - 20px) * ${imgWidth} / ${imgHeight}))`
                : `calc((100dvh - 20px) * ${imgWidth} / ${imgHeight})`,
              margin: "0 auto",
              border: `1px solid ${COLOR_DIALOG_BORDER}`,
              overflow: "hidden",
              containerType: "size",
            }}
          >
            <img
              src={imageSrc}
              alt={imageAlt}
              draggable={false}
              style={{ position: "absolute", inset: 0, width: "100%", height: "100%", objectFit: "fill", display: "block" }}
            />

            {(visibilityContext
              ? points.filter((pt) => !pt.visible || pt.visible(visibilityContext))
              : points
            ).map((pt, i) => {
              const effectiveLabel = typeof pt.label === "function"
                ? (visibilityContext ? pt.label(visibilityContext) : "")
                : pt.label;
              const isSelected = selectedPoint !== undefined && effectiveLabel === selectedPoint;
              const isDisabled = isPointDisabled?.(effectiveLabel, pt) ?? false;
              const tooltipId = `map-point-restriction-${i}`;
              return (
                <div
                  key={i}
                  style={{
                    position: "absolute",
                    left: pt.left,
                    top: pt.top,
                    transform: "translate(-50%, -50%)",
                    zIndex: 10,
                    width: pt.width ?? btnStyle.width,
                    height: pt.height ?? btnStyle.height,
                    cursor: isDisabled ? "not-allowed" : undefined,
                  }}
                  onMouseEnter={() => isDisabled && setHoveredRestrictedPoint(i)}
                  onMouseLeave={() => setHoveredRestrictedPoint(null)}
                >
                  <button
                    type="button"
                    disabled={isDisabled}
                    aria-describedby={isDisabled ? tooltipId : undefined}
                    onClick={() => onSelect(effectiveLabel)}
                    style={{
                      ...btnStyle,
                      width: "100%",
                      height: "100%",
                      ...pointStyle,
                      ...(isDisabled ? { color: "#B9B9B9", cursor: "not-allowed", pointerEvents: "none" } : {}),
                      ...(!isDisabled && isSelected ? { backgroundColor: "#1D4ED8", color: "#FFFFFF" } : {}),
                    }}
                  >
                    {effectiveLabel}
                  </button>
                  {isDisabled && hoveredRestrictedPoint === i && (
                    <span
                      id={tooltipId}
                      role="tooltip"
                      style={{
                        position: "absolute",
                        left: "50%",
                        bottom: "calc(100% + 6px)",
                        transform: "translateX(-50%)",
                        zIndex: 30,
                        width: "max-content",
                        maxWidth: "240px",
                        padding: "4px 7px",
                        border: "1px solid #777",
                        borderRadius: "3px",
                        background: "white",
                        color: "#222",
                        boxShadow: "0 2px 6px rgba(0,0,0,0.25)",
                        fontFamily: "Arial, sans-serif",
                        fontSize: "max(12px, 1.4cqh)",
                        fontWeight: "normal",
                        lineHeight: 1.25,
                        textAlign: "center",
                        pointerEvents: "none",
                      }}
                    >
                      {disabledPointMessage}
                    </span>
                  )}
                </div>
              );
            })}

            {staticPoints.map((pt, i) => (
              <button
                key={`static-${i}`}
                type="button"
                disabled
                aria-label={`TODA ${pt.label}`}
                style={{
                  ...btnStyle,
                  position: "absolute",
                  left: pt.left,
                  top: pt.top,
                  transform: "translate(-50%, -50%)",
                  zIndex: 10,
                  width: pt.width ?? btnStyle.width,
                  height: pt.height ?? btnStyle.height,
                  backgroundColor: "#938E8E",
                  ...MAP_POINT_STROKE_STYLE,
                  color: "black",
                  opacity: 1,
                  cursor: "default",
                }}
              >
                {pt.label}
              </button>
            ))}

            {children}
          </div>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
