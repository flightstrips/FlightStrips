import { useWebSocketStore, useApronOnline, useTwrOnline, useIsTwr } from "@/store/store-hooks";
import { APRON_TAXI_POINTS } from "@/config/ekch";
import { MAP_BTN_BASE, MapDialogShell, MapEraseControls } from "./MapDialogShell";
import { Bay } from "@/api/models";

interface ApronTaxiMapDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  callsign: string;
  noMove?: boolean;
}

const BTN_STYLE: React.CSSProperties = {
  ...MAP_BTN_BASE,
  width: 75,
  height: 45,
  fontSize: 22,
};

// Hold short points that correspond to the final hold (TWY DEP-LWR / TAXI_LWR).
const LWR_HOLD_POINTS = new Set(["B", "D", "F", "A", "K3", "K2", "K1", "V2", "V1"]);

export function ApronTaxiMapDialog({ open, onOpenChange, callsign, noMove = false }: ApronTaxiMapDialogProps) {
  const setReleasePoint = useWebSocketStore((s) => s.setReleasePoint);
  const move = useWebSocketStore((s) => s.move);
  const apronOnline = useApronOnline();
  const twrOnline = useTwrOnline();
  const isTwr = useIsTwr();

  const handleSelect = (label: string) => {
    setReleasePoint(callsign, label);

    if (!noMove) {
      // AUTO-LOCAL routing:
      // - Solo TWR (no separate apron): all points → TWY DEP-LWR so TWR can see them.
      // - Split ops (separate apron or TWR online): final hold points → TWY DEP-LWR, others → TWY DEP-UPR.
      const splitOps = apronOnline || twrOnline;
      const soloTwr = isTwr && !apronOnline;
      const targetBay = (soloTwr || (splitOps && LWR_HOLD_POINTS.has(label)))
        ? Bay.TaxiLwr
        : Bay.Taxi;
      move(callsign, targetBay);
    }

    onOpenChange(false);
  };

  return (
    <MapDialogShell
      open={open}
      onOpenChange={onOpenChange}
      title="Select Taxi Route"
      imageSrc="/apron_taxi.webp"
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