import { useWebSocketStore } from "@/store/store-hooks";
import { TAXI_MAP_POINTS } from "@/config/ekch";
import taxiMap from "@/assets/taxi_map.webp";
import { MAP_BTN_BASE, MapDialogShell, MapEraseControls } from "./MapDialogShell";

interface TaxiMapDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  callsign: string;
}

const BTN_STYLE: React.CSSProperties = {
  ...MAP_BTN_BASE,
  width: 70,
  height: 40,
};

export function TaxiMapDialog({ open, onOpenChange, callsign }: TaxiMapDialogProps) {
  const setReleasePoint = useWebSocketStore((s) => s.setReleasePoint);

  const handleSelect = (label: string) => {
    setReleasePoint(callsign, label);
    onOpenChange(false);
  };

  return (
    <MapDialogShell
      open={open}
      onOpenChange={onOpenChange}
      title="Select Taxi Route"
      imageSrc={taxiMap}
      imageAlt="TWR taxi map"
      imgWidth={1801}
      imgHeight={1013}
      points={TAXI_MAP_POINTS}
      btnStyle={BTN_STYLE}
      onSelect={handleSelect}
    >
      {/* Controls panel — bottom-left */}
      <div
        style={{
          position: "absolute",
          bottom: "5%",
          left: "2%",
          zIndex: 20,
          display: "flex",
          flexDirection: "column",
          gap: 4,
        }}
      >
        <MapEraseControls onOk={handleSelect} btnStyle={BTN_STYLE} />
      </div>
    </MapDialogShell>
  );
}