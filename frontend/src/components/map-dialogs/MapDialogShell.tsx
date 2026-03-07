import { useState, useRef, useEffect } from "react";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import * as VisuallyHidden from "@radix-ui/react-visually-hidden";
import { useDragDisabled } from "@/components/bays/DragDisabledContext";
import type { ClickPoint } from "@/config/ekch";

/** Shared button base — each dialog extends this with its own width/height/fontSize. */
// eslint-disable-next-line react-refresh/only-export-components
export const MAP_BTN_BASE: React.CSSProperties = {
  backgroundColor: "#D6D6D6",
  fontFamily: "Arial, sans-serif",
  fontWeight: "bold",
  color: "#000",
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
  border: "1px solid #888",
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
  btnStyle: React.CSSProperties;
  maxLength?: number;
}

export function MapEraseControls({ onOk, btnStyle, maxLength = 6 }: MapEraseControlsProps) {
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
          onClick={() => { setTyped(""); inputRef.current?.focus(); }}
          style={{ ...btnStyle, width: 90, backgroundColor: "#3F3F3F", color: "#FFF" }}
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
            backgroundColor: "#D6D6D6",
            textTransform: "uppercase",
          }}
        />
      </div>
      <div style={{ display: "flex", justifyContent: "flex-end", gap: 5 }}>
        <button onClick={handleOk} style={{ ...btnStyle, backgroundColor: "#3F3F3F", color: "#FFF", width: 80 }}>
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
  /** Controls panel overlay (e.g. arrows + ERASE/OK). Positioned absolutely over the image. */
  children?: React.ReactNode;
}

export function MapDialogShell({
  open, onOpenChange, title,
  imageSrc, imageAlt, imgWidth, imgHeight,
  points, btnStyle, onSelect,
  children,
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
              maxWidth: `calc((100vh - 20px) * ${imgWidth} / ${imgHeight})`,
              margin: "0 auto",
            }}
          >
            <img
              src={imageSrc}
              alt={imageAlt}
              draggable={false}
              style={{ position: "absolute", inset: 0, width: "100%", height: "100%", objectFit: "fill", display: "block" }}
            />

            {points.map((pt, i) => (
              <button
                key={i}
                onClick={() => onSelect(pt.label)}
                style={{
                  ...btnStyle,
                  position: "absolute",
                  left: pt.left,
                  top: pt.top,
                  transform: "translate(-50%, -50%)",
                  zIndex: 10,
                  width: pt.width ?? btnStyle.width,
                  height: pt.height ?? btnStyle.height,
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
