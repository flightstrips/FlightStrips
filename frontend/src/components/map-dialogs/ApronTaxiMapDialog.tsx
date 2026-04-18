import { useWebSocketStore, useApronOnline, useTwrOnline, useIsTwr, useMyPosition, useStrip } from "@/store/store-hooks";
import { APRON_TAXI_POINTS, COORDINATION_APRON_TAXI_POINTS } from "@/config/ekch";
import { MAP_BTN_BASE, MapDialogShell, MapEraseControls } from "./MapDialogShell";
import { Bay } from "@/api/models";

interface ApronTaxiMapDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  callsign: string;
  noMove?: boolean;
  coordinationMode?: boolean;
}

// Sizes expressed as % of image container (1859×903 reference) so they scale with the dialog.
const BTN_STYLE: React.CSSProperties = {
  ...MAP_BTN_BASE,
  width: "4.03%",      // 75px at 1859px wide
  height: "4.98cqh",   // 45px at 903px tall
  fontSize: "2.44cqh", // 22px at 903px tall
};

// Hold short points that correspond to the final hold (TWY DEP-LWR / TAXI_LWR).
const LWR_HOLD_POINTS = new Set(["B", "D", "F", "A", "K3", "K2", "K1", "V2", "V1"]);

export function ApronTaxiMapDialog({
  open,
  onOpenChange,
  callsign,
  noMove = false,
  coordinationMode = false,
}: ApronTaxiMapDialogProps) {
  const setReleasePoint = useWebSocketStore((s) => s.setReleasePoint);
  const acknowledgeUnexpectedChange = useWebSocketStore((s) => s.acknowledgeUnexpectedChange);
  const move = useWebSocketStore((s) => s.move);
  const apronOnline = useApronOnline();
  const twrOnline = useTwrOnline();
  const isTwr = useIsTwr();
  const strip = useStrip(callsign);
  const myPosition = useMyPosition();

  const shouldAcknowledgeReleasePoint =
    strip?.unexpected_change_fields?.includes("release_point") && !!myPosition && strip.owner === myPosition;

  const handleSelect = (label: string) => {
    if (label !== strip?.release_point) {
      setReleasePoint(callsign, label);
    }

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

    if (shouldAcknowledgeReleasePoint) {
      acknowledgeUnexpectedChange(callsign, "release_point");
    }

    onOpenChange(false);
  };

  const handleErase = () => {
    // Erase only clears the stored route; it intentionally leaves the strip in its current bay.
    if (strip?.release_point) {
      setReleasePoint(callsign, "");
    }

    if (shouldAcknowledgeReleasePoint) {
      acknowledgeUnexpectedChange(callsign, "release_point");
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
        points={coordinationMode ? COORDINATION_APRON_TAXI_POINTS : APRON_TAXI_POINTS}
        btnStyle={BTN_STYLE}
        onSelect={handleSelect}
        selectedPoint={strip?.release_point}
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
          gap: "0.44cqh",
        }}
      >
        <MapEraseControls onOk={handleSelect} onErase={handleErase} btnStyle={BTN_STYLE} />
      </div>
    </MapDialogShell>
  );
}
