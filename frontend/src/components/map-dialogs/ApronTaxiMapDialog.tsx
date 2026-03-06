import { useWebSocketStore } from "@/store/store-hooks";
import { APRON_TAXI_POINTS } from "@/config/ekch";
import apronTaxi from "@/assets/apron_taxi.webp";
import { MAP_BTN_BASE, MapDialogShell, MapEraseControls } from "./MapDialogShell";

interface ApronTaxiMapDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  callsign: string;
}

const BTN_STYLE: React.CSSProperties = {
  ...MAP_BTN_BASE,
  width: 75,
  height: 45,
  fontSize: 22,
};

export function ApronTaxiMapDialog({ open, onOpenChange, callsign }: ApronTaxiMapDialogProps) {
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
      imageSrc={apronTaxi}
      imageAlt="Apron taxi map"
      imgWidth={1859}
      imgHeight={903}
      points={APRON_TAXI_POINTS}
      btnStyle={BTN_STYLE}
      onSelect={handleSelect}
    >
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
        <MapEraseControls onOk={handleSelect} btnStyle={BTN_STYLE} />
      </div>
    </MapDialogShell>
  );
}