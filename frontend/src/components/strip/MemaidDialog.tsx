import { useState } from "react";

import { Dialog, DialogContent } from "@/components/ui/dialog";
import { useSelectedCallsign, useWebSocketStore } from "@/store/store-hooks";
import { scalePx, toVw } from "@/lib/viewportScale";

const DROP_SHADOW = `0 ${scalePx(4)} ${scalePx(4)} rgba(0,0,0,0.25)`;

const BTN_BASE: React.CSSProperties = {
  fontFamily: "Rubik, sans-serif",
  fontWeight: 600,
  fontSize: scalePx(20),
  border: "none",
  cursor: "pointer",
  boxShadow: DROP_SHADOW,
};

const configuredLabels: string[] = [
  "SEPARATION BETWEEN STARTS 3 MIN",
  "STOP CLIMB AT 3000'",
  "STOP CLIMB AT 4000'",
];

interface Props {
  open: boolean;
  bay: string;
  onOpenChange: (open: boolean) => void;
}

export function MemaidDialog({ open, bay, onOpenChange }: Props) {
  const [label, setLabel] = useState("");
  const [selectedPreset, setSelectedPreset] = useState<number | null>(null);

  const createTacticalStrip = useWebSocketStore((s) => s.createTacticalStrip);
  const selectedAircraft = useSelectedCallsign();

  function handlePreset(i: number) {
    setSelectedPreset(i);
    setLabel(configuredLabels[i]);
  }

  function handleSubmit() {
    if (!label.trim()) return;
    createTacticalStrip("MEMAID", bay, label.trim(), selectedAircraft ?? "");
    reset();
    onOpenChange(false);
  }

  function handleCancel() {
    reset();
    onOpenChange(false);
  }

  function reset() {
    setLabel("");
    setSelectedPreset(null);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        style={{
          width: `min(${toVw(494)}, 95vw)`,
          background: "#E4E4E4",
          border: "1px solid black",
          borderRadius: 0,
          padding: `${scalePx(20)} ${scalePx(24)}`,
          display: "flex",
          flexDirection: "column",
          gap: 0,
          color: "#000",
        }}
      >
        <input
          autoFocus
          value={label}
          onChange={(e) => {
            setLabel(e.target.value);
            setSelectedPreset(null);
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter") handleSubmit();
            if (e.key === "Escape") handleCancel();
          }}
          placeholder="Memory aid message..."
          style={{
            width: "100%",
            height: scalePx(44),
            background: "#FCFCFC",
            border: "1px solid black",
            fontFamily: "Rubik, sans-serif",
            fontSize: scalePx(20),
            padding: `0 ${scalePx(12)}`,
            boxSizing: "border-box",
            boxShadow: DROP_SHADOW,
            outline: "none",
            marginBottom: scalePx(12),
          }}
        />

        <div style={{ display: "flex", flexDirection: "column", gap: scalePx(8), marginBottom: scalePx(16) }}>
          {configuredLabels.map((msg, i) => (
            <button
              key={i}
              style={{
                ...BTN_BASE,
                textAlign: "left",
                padding: `0 ${scalePx(12)}`,
                height: scalePx(42),
                background: selectedPreset === i ? "#1BFF16" : "#D6D6D6",
                color: "#000",
              }}
              onClick={() => handlePreset(i)}
            >
              {msg}
            </button>
          ))}
        </div>

        <div style={{ display: "flex", justifyContent: "space-between" }}>
          <button
            style={{
              ...BTN_BASE,
              width: scalePx(120),
              height: scalePx(55),
              background: "#3F3F3F",
              color: "#fff",
              fontSize: scalePx(28),
            }}
            onClick={handleCancel}
          >
            ESC
          </button>
          <button
            style={{
              ...BTN_BASE,
              width: scalePx(120),
              height: scalePx(55),
              background: "#3F3F3F",
              color: "#fff",
              fontSize: scalePx(28),
              opacity: label.trim() ? 1 : 0.4,
            }}
            disabled={!label.trim()}
            onClick={handleSubmit}
          >
            OK
          </button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
