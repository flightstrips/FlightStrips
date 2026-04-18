import { useWebSocketStore } from "@/store/store-hooks";
import { Bay } from "@/api/models";
import { RELEASE_POINTS } from "@/config/ekch";
import { MAP_BTN_BASE, MapCloseButton, MapDialogShell, MapEraseControls } from "./MapDialogShell";

interface PushbackMapDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  callsign: string;
  initialReleasePoint?: string;
  onStripMoved?: () => void;
}

// Sizes expressed as % of image container (1920×768 reference) so they scale with the dialog.
const BTN_STYLE: React.CSSProperties = {
  ...MAP_BTN_BASE,
  width: "2.86%",      // 55px at 1920px wide
  height: "3.91cqh",   // 30px at 768px tall
  fontSize: "2.34cqh", // 18px at 768px tall
};

export function PushbackMapDialog({ open, onOpenChange, callsign, initialReleasePoint, onStripMoved }: PushbackMapDialogProps) {
  const setReleasePoint = useWebSocketStore((s) => s.setReleasePoint);
  const move = useWebSocketStore((s) => s.move);

  const handleSelect = (label: string) => {
    setReleasePoint(callsign, label);
    move(callsign, Bay.Push);
    onOpenChange(false);
    onStripMoved?.();
  };

  return (
    <MapDialogShell
      open={open}
      onOpenChange={onOpenChange}
      title="Select Release Point"
      imageSrc="/apron_push.webp"
      imageAlt="Apron pushback map"
      imgWidth={1920}
      imgHeight={768}
      points={RELEASE_POINTS}
      btnStyle={BTN_STYLE}
      onSelect={handleSelect}
      selectedPoint={initialReleasePoint}
    >
      {/* Controls panel — bottom-left */}
      <div
        style={{
          position: "absolute",
          left: "2.5%",
          top: "70%",
          zIndex: 20,
          display: "flex",
          flexDirection: "row",
          alignItems: "flex-start",
          gap: "0.63cqw",
        }}
      >
        {/* Arrow cross */}
        <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: "0.52cqh" }}>
          <button onClick={() => handleSelect("N")} style={BTN_STYLE}>↑</button>
          <div style={{ display: "flex", gap: "0.52cqh" }}>
            <button onClick={() => handleSelect("W")} style={BTN_STYLE}>←</button>
            <button onClick={() => handleSelect("E")} style={BTN_STYLE}>→</button>
          </div>
          <button onClick={() => handleSelect("S")} style={BTN_STYLE}>↓</button>
        </div>

        {/* ERASE / input / OK — offset down to align with ← → row */}
        <div style={{ display: "flex", flexDirection: "column", gap: "0.52cqh", marginTop: "4.43cqh" }}>
          <MapEraseControls onOk={handleSelect} btnStyle={BTN_STYLE} maxLength={4} />
          <MapCloseButton onClose={() => onOpenChange(false)} btnStyle={BTN_STYLE} />
        </div>
      </div>
    </MapDialogShell>
  );
}

