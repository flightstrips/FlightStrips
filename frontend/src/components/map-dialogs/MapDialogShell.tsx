import { useState, useRef, useEffect } from "react";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import * as VisuallyHidden from "@radix-ui/react-visually-hidden";
import { useDragDisabled } from "@/components/bays/DragDisabledContext";
import type { ClickPoint, VisibilityContext } from "@/config/ekch";

// Map dialog color constants (used in CSSProperties style objects)
const COLOR_MAP_BTN_BG    = "#D6D6D6"; // light grey button background
const COLOR_MAP_BTN_DARK  = "#3F3F3F"; // dark button (OK/ERASE)
const COLOR_MAP_INPUT_BG  = "#D6D6D6"; // text input background (same as button)
const COLOR_DIALOG_BORDER = "#888";    // dialog shell border

/** Shared button base — each dialog extends this with its own width/height/fontSize. */
// eslint-disable-next-line react-refresh/only-export-components
export const MAP_BTN_BASE: React.CSSProperties = {
  backgroundColor: COLOR_MAP_BTN_BG,
  fontFamily: "Arial, sans-serif",
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

const DIALOG_CONTENT_STYLE: React.CSSProperties = {
  position: "fixed",
  left: 10,
  right: 10,
  top: "50%",
  transform: "translateY(-50%)",
  zIndex: 51,
  outline: "none",
  overflow: "hidden",
  maxHeight: "calc(100vh - 20px)",
  maxWidth: "calc(100vw - 20px)",
};

// ---------------------------------------------------------------------------
// MapEraseControls — ERASE button, text input, and OK button
// ---------------------------------------------------------------------------

interface MapEraseControlsProps {
  onOk: (value: string) => void;
  onErase?: () => void;
  btnStyle: React.CSSProperties;
  maxLength?: number;
}

export function MapEraseControls({ onOk, onErase, btnStyle, maxLength = 6 }: MapEraseControlsProps) {
  const [typed, setTyped] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  const handleOk = () => {
    const value = typed.trim().toUpperCase();
    if (value) {
      onOk(value);
      setTyped("");
    }
  };

  return (
    <>
      <div style={{ display: "flex", gap: 5, alignItems: "center" }}>
        <button
          onClick={() => { setTyped(""); onErase?.(); }}
          style={{ ...btnStyle, width: 90, backgroundColor: COLOR_MAP_BTN_DARK, color: "white" }}
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
            width: 80,
            height: btnStyle.height,
            fontFamily: "Arial, sans-serif",
            fontWeight: "bold",
            fontSize: 16,
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
      <div style={{ display: "flex", justifyContent: "flex-end", gap: 5 }}>
        <button onClick={handleOk} style={{ ...btnStyle, backgroundColor: COLOR_MAP_BTN_DARK, color: "white", width: 80 }}>
          OK
        </button>
      </div>
    </>
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
}

export function MapDialogShell({
  open, onOpenChange, title,
  imageSrc, imageAlt, imgWidth, imgHeight,
  points, btnStyle, onSelect,
  selectedPoint,
  children,
  scaleMode = "height",
  visibilityContext,
}: MapDialogShellProps) {
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
              maxHeight: "calc(100vh - 20px)",
              maxWidth: scaleMode === "width"
                ? `min(calc(100vw * ${imgWidth} / 1920), calc((100vh - 20px) * ${imgWidth} / ${imgHeight}))`
                : `calc((100vh - 20px) * ${imgWidth} / ${imgHeight})`,
              margin: "0 auto",
              border: `1px solid ${COLOR_DIALOG_BORDER}`,
              overflow: "hidden",
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
              return (
                <button
                  key={i}
                  onClick={() => onSelect(effectiveLabel)}
                  style={{
                    ...btnStyle,
                    position: "absolute",
                    left: pt.left,
                    top: pt.top,
                    transform: "translate(-50%, -50%)",
                    zIndex: 10,
                    width: pt.width ?? btnStyle.width,
                    height: pt.height ?? btnStyle.height,
                    ...(isSelected ? { backgroundColor: "#1D4ED8", color: "#FFFFFF" } : {}),
                  }}
                >
                  {effectiveLabel}
                </button>
              );
            })}

            {children}
          </div>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
