import { useWebSocketStore } from "@/store/store-hooks";
import { TAXI_MAP_POINTS } from "@/config/ekch";
import taxiMap from "@/assets/taxi_map.webp";
import { MAP_BTN_BASE, MapDialogShell, MapEraseControls } from "./MapDialogShell";

interface TaxiMapDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  callsign: string;
  /**
   * Controls which subset of TAXI_MAP_POINTS is shown and how a selection
   * is handled:
   *   "hp" — only holding points; selection persists to release_point (DB)
   *          and clears any local clearance limit via onClearanceLimitSelect("").
   *   "cl" — only clearance-limit points; selection clears release_point (DB)
   *          and stores the label locally via onClearanceLimitSelect(label).
   * When omitted all points are shown and the "hp" selection path is used.
   */
  mode?: "hp" | "cl";
  /**
   * Called whenever the clearance-limit selection changes:
   *   - mode="cl": invoked with the chosen label (set CL).
   *   - mode="hp": invoked with "" (clear CL, because an HP was set instead).
   * Local/ephemeral — nothing here is persisted to the backend.
   */
  onClearanceLimitSelect?: (label: string) => void;
}

const BTN_STYLE: React.CSSProperties = {
  ...MAP_BTN_BASE,
  width: 70,
  height: 40,
};

export function TaxiMapDialog({
  open,
  onOpenChange,
  callsign,
  mode,
  onClearanceLimitSelect,
}: TaxiMapDialogProps) {
  const setReleasePoint = useWebSocketStore((s) => s.setReleasePoint);

  // Show only the point type that matches the current mode.
  // Points with no explicit type default to "cl".
  const visiblePoints = mode
    ? TAXI_MAP_POINTS.filter((pt) => (pt.type ?? "cl") === mode)
    : TAXI_MAP_POINTS;

  const handleSelect = (label: string) => {
    if (mode === "hp") {
      // Holding point selected → persist to DB and clear any local CL.
      setReleasePoint(callsign, label);
      onClearanceLimitSelect?.("");
    } else {
      // Clearance limit selected (mode="cl" or unset) → clear the DB holding
      // point so the two fields don't show contradictory information, then
      // notify the parent to store the CL label locally.
      setReleasePoint(callsign, "");
      onClearanceLimitSelect?.(label);
    }
    onOpenChange(false);
  };

  const dialogTitle =
    mode === "hp" ? "Select Holding Point" :
    mode === "cl" ? "Select Clearance Limit" :
                    "Select Taxi Route";

  return (
    <MapDialogShell
      open={open}
      onOpenChange={onOpenChange}
      title={dialogTitle}
      imageSrc={taxiMap}
      imageAlt="TWR taxi map"
      imgWidth={1801}
      imgHeight={1013}
      points={visiblePoints}
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