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
  boxShadow: "0 4px 4px rgba(0,0,0,0.25)",
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
            maxHeight: "calc(100vh - 20px)",
            maxWidth: "calc(100vw - 20px)",
          }}
        >
          <VisuallyHidden.Root>
            <DialogPrimitive.Title>Select Release Point</DialogPrimitive.Title>
          </VisuallyHidden.Root>

          {/* Aspect-ratio container — 1920×768 matches apron_push.png exactly */}
          <div style={{ position: "relative", width: "100%", aspectRatio: "1920 / 768", maxHeight: "calc(100vh - 20px)", maxWidth: "calc((100vh - 20px) * 1920 / 768)", margin: "0 auto" }}>
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
                }}
              >
                {rp.label}
              </button>
            ))}

            {/* Controls panel — bottom-left, no surrounding box */}
            <div
              style={{
                position: "absolute",
                left: "2.5%",
                top: "70%",
                zIndex: 20,
                display: "flex",
                flexDirection: "row",
                alignItems: "flex-start",
                gap: 12,
              }}
            >
              {/* Arrow cross */}
              <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 4 }}>
                <button onClick={() => handleSelect("N")} style={BTN_STYLE}>↑</button>
                <div style={{ display: "flex", gap: 4 }}>
                  <button onClick={() => handleSelect("W")} style={BTN_STYLE}>←</button>
                  <button onClick={() => handleSelect("E")} style={BTN_STYLE}>→</button>
                </div>
                <button onClick={() => handleSelect("S")} style={BTN_STYLE}>↓</button>
              </div>

              {/* ERASE / input / OK — offset down to align with ← → row */}
              <div style={{ display: "flex", flexDirection: "column", gap: 4, marginTop: 34 }}>
                <div style={{ display: "flex", gap: 4, alignItems: "center" }}>
                  <button
                    onClick={() => { setTypedPoint(""); inputRef.current?.focus(); }}
                    style={{ ...BTN_STYLE, width: 90, backgroundColor: "#3F3F3F", color: "#FFF" }}
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
                      width: 80,
                      height: BTN_STYLE.height,
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
                <div style={{ display: "flex", justifyContent: "flex-end" }}>
                  <button onClick={handleOk} style={{ ...BTN_STYLE, backgroundColor: "#3F3F3F", color: "#FFF", width: 80 }}>OK</button>
                </div>
              </div>
            </div>
          </div>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}

