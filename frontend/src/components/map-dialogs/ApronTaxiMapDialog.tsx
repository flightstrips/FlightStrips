import { useState, useRef, useEffect } from "react";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import * as VisuallyHidden from "@radix-ui/react-visually-hidden";
import { useWebSocketStore } from "@/store/store-hooks";
import { APRON_TAXI_POINTS } from "@/config/ekch";
import { useDragDisabled } from "@/components/bays/DragDisabledContext";
import apronTaxi from "@/assets/apron_taxi.png";

interface ApronTaxiMapDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  callsign: string;
}

const BTN_STYLE: React.CSSProperties = {
  width: 75,
  height: 45,
  backgroundColor: "#D6D6D6",
  fontFamily: "Arial, sans-serif",
  fontWeight: "bold",
  fontSize: 22,
  border: "none",
  cursor: "pointer",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  userSelect: "none",
  flexShrink: 0,
  boxShadow: "0 4px 4px rgba(0,0,0,0.25)",
};

export function ApronTaxiMapDialog({ open, onOpenChange, callsign }: ApronTaxiMapDialogProps) {
  const [typedPoint, setTypedPoint] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const { setDragDisabled } = useDragDisabled();

  const setReleasePoint = useWebSocketStore((s) => s.setReleasePoint);

  // Disable strip dragging while popup is visible
  useEffect(() => {
    setDragDisabled(open);
    return () => setDragDisabled(false);
  }, [open, setDragDisabled]);

  const handleSelect = (label: string) => {
    setReleasePoint(callsign, label);
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
              <DialogPrimitive.Title>Select Taxi Route</DialogPrimitive.Title>
            </VisuallyHidden.Root>

            {/* Aspect-ratio container — 1859×903 matches apron_taxi.png exactly */}
            <div style={{ position: "relative", width: "100%", aspectRatio: "1859 / 903", maxHeight: "calc(100vh - 20px)", maxWidth: "calc((100vh - 20px) * 1859 / 903)", margin: "0 auto" }}>
              <img
                src={apronTaxi}
                alt="Apron taxi map"
                draggable={false}
                style={{ position: "absolute", inset: 0, width: "100%", height: "100%", objectFit: "fill", display: "block" }}
              />



            {/* Taxi route buttons */}
            {APRON_TAXI_POINTS.map((pt, i) => (
              <button
                key={i}
                onClick={() => handleSelect(pt.label)}
                style={{
                  ...BTN_STYLE,
                  position: "absolute",
                  left: pt.left,
                  top: pt.top,
                  transform: "translate(-50%, -50%)",
                  zIndex: 10,
                  width: pt.width || BTN_STYLE.width,
                  height: pt.height || BTN_STYLE.height,
                }}
              >
                {pt.label}
              </button>
            ))}

            {/* Controls panel — bottom-left */}
            <div
              style={{
                position: "absolute",
                bottom: "30%",
                left: "10%",
                zIndex: 20,
                display: "flex",
                flexDirection: "column",
                gap: 4,
              }}
            >
              {/* ERASE · input · OK */}
              <div style={{ display: "flex", gap: 5, alignItems: "center" }}>
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
                  maxLength={6}
                  style={{
                    width: 80,
                    height: 45,
                    fontFamily: "Arial, sans-serif",
                    fontWeight: "bold",
                    fontSize: 22,
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
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 5 }}>
                <button onClick={handleOk} style={{ ...BTN_STYLE, backgroundColor: "#3F3F3F", color: "#FFF", width: 80 }}>OK</button>
              </div>
            </div>
          </div>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
