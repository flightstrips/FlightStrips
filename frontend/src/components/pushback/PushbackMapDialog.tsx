import { useState, useRef, useEffect } from "react";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import * as VisuallyHidden from "@radix-ui/react-visually-hidden";
import { useWebSocketStore } from "@/store/store-hooks";
import { Bay } from "@/api/models";
import { RELEASE_POINTS } from "@/config/ekch";
import { useDragDisabled } from "@/components/bays/DragDisabledContext";
import apronPush from "@/assets/apron_push.png";

interface PushbackMapDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  callsign: string;
}

const BTN_STYLE: React.CSSProperties = {
  width: 55,
  height: 30,
  backgroundColor: "#D6D6D6",
  fontFamily: "Arial, sans-serif",
  fontWeight: "bold",
  fontSize: 18,
  border: "none",
  cursor: "pointer",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  userSelect: "none",
  flexShrink: 0,
};

export function PushbackMapDialog({ open, onOpenChange, callsign }: PushbackMapDialogProps) {
  const [typedPoint, setTypedPoint] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const { setDragDisabled } = useDragDisabled();

  const setReleasePoint = useWebSocketStore((s) => s.setReleasePoint);
  const move = useWebSocketStore((s) => s.move);

  // Disable strip dragging while popup is visible
  useEffect(() => {
    setDragDisabled(open);
    return () => setDragDisabled(false);
  }, [open, setDragDisabled]);

  const handleSelect = (label: string) => {
    setReleasePoint(callsign, label);
    move(callsign, Bay.Push);
    setTypedPoint("");
    onOpenChange(false);
  };

  const handleOk = () => {
    const value = typedPoint.trim().toUpperCase();
    if (value) handleSelect(value);
  };

  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        {/* Transparent overlay — normal view shows through; click outside closes */}
        <DialogPrimitive.Overlay style={{ position: "fixed", inset: 0, zIndex: 50 }} />

        <DialogPrimitive.Content
          style={{
            position: "fixed",
            left: 10,
            right: 10,
            top: "50%",
            transform: "translateY(-50%)",
            zIndex: 51,
            border: "1px solid #888",
            outline: "none",
            overflow: "hidden",
          }}
        >
          <VisuallyHidden.Root>
            <DialogPrimitive.Title>Select Release Point</DialogPrimitive.Title>
          </VisuallyHidden.Root>

          {/* Aspect-ratio container — 1920×768 matches apron_push.png exactly */}
          <div style={{ position: "relative", width: "100%", aspectRatio: "1920 / 768" }}>
            <img
              src={apronPush}
              alt="Apron pushback map"
              draggable={false}
              style={{ position: "absolute", inset: 0, width: "100%", height: "100%", objectFit: "cover", display: "block" }}
            />

            {/* Release point buttons */}
            {RELEASE_POINTS.map((rp) => (
              <button
                key={rp.label}
                onClick={() => handleSelect(rp.label)}
                style={{
                  ...BTN_STYLE,
                  position: "absolute",
                  left: rp.left,
                  top: rp.top,
                  transform: "translate(-50%, -50%)",
                  zIndex: 10,
                  fontSize: "clamp(8px, 0.9vw, 16px)",
                }}
              >
                {rp.label}
              </button>
            ))}

            {/* Controls panel — bottom-left, no surrounding box */}
            <div
              style={{
                position: "absolute",
                bottom: 8,
                left: 8,
                zIndex: 20,
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                gap: 4,
              }}
            >
              {/* Up */}
              <button onClick={() => handleSelect("N")} style={BTN_STYLE}>↑</button>

              {/* Left / Right */}
              <div style={{ display: "flex", gap: 4 }}>
                <button onClick={() => handleSelect("W")} style={BTN_STYLE}>←</button>
                <button onClick={() => handleSelect("E")} style={BTN_STYLE}>→</button>
              </div>

              {/* Down */}
              <button onClick={() => handleSelect("S")} style={BTN_STYLE}>↓</button>

              {/* ERASE · input · OK */}
              <div style={{ display: "flex", gap: 5, alignItems: "center" }}>
                <button
                  onClick={() => { setTypedPoint(""); inputRef.current?.focus(); }}
                  style={BTN_STYLE}
                >
                  ERASE
                </button>
                <input
                  ref={inputRef}
                  value={typedPoint}
                  onChange={(e) => setTypedPoint(e.target.value.toUpperCase())}
                  onKeyDown={(e) => e.key === "Enter" && handleOk()}
                  maxLength={4}
                  style={{
                    width: 55,
                    height: 30,
                    fontFamily: "Arial, sans-serif",
                    fontWeight: "bold",
                    fontSize: 16,
                    textAlign: "center",
                    border: "none",
                    outline: "none",
                    backgroundColor: "#fff",
                    textTransform: "uppercase",
                  }}
                />
                <button onClick={handleOk} style={BTN_STYLE}>OK</button>
              </div>
            </div>
          </div>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}

