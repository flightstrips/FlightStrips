import { useWebSocketStore } from "@/store/store-hooks";
import { Bay } from "@/api/models";
import { RELEASE_POINTS } from "@/config/ekch";
import apronPush from "@/assets/apron_push.webp";
import { MAP_BTN_BASE, MapDialogShell, MapEraseControls } from "./MapDialogShell";

interface PushbackMapDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  callsign: string;
  initialReleasePoint?: string;
}

const BTN_STYLE: React.CSSProperties = {
  ...MAP_BTN_BASE,
  width: 55,
  height: 30,
  fontSize: 18,
};

export function PushbackMapDialog({ open, onOpenChange, callsign, initialReleasePoint }: PushbackMapDialogProps) {
  const setReleasePoint = useWebSocketStore((s) => s.setReleasePoint);
  const move = useWebSocketStore((s) => s.move);

  const handleSelect = (label: string) => {
    setReleasePoint(callsign, label);
    move(callsign, Bay.Push);
    onOpenChange(false);
  };

  return (
    <MapDialogShell
      open={open}
      onOpenChange={onOpenChange}
      title="Select Release Point"
      imageSrc={apronPush}
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
          <MapEraseControls onOk={handleSelect} btnStyle={BTN_STYLE} maxLength={4} />
        </div>
      </div>
    </MapDialogShell>
  );
}

