import { useWebSocketStore, useRunwaySetup, useApronOnline, useTwrOnline, useIsTwr, useMyPosition, useStrip } from "@/store/store-hooks";
import { COORDINATION_WITH_APRON_TAXI_MAP_POINTS, COORDINATION_WITH_TWR_TAXI_MAP_POINTS, TAXI_MAP_POINTS } from "@/config/ekch";
import type { VisibilityContext } from "@/config/ekch";
import { MAP_BTN_BASE, MapDialogShell, MapEraseControls } from "./MapDialogShell";

interface TaxiMapDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  callsign: string;
}

// Sizes expressed as % of image container (1801×1013 reference) so they scale with the dialog.
const BTN_STYLE: React.CSSProperties = {
  ...MAP_BTN_BASE,
  width: "3.89%",  // 70px at 1801px wide
  height: "3.95cqh", // 40px at 1013px tall
};

export function TaxiMapDialog({
  open,
  onOpenChange,
  callsign,
}: TaxiMapDialogProps) {
  const setReleasePoint = useWebSocketStore((s) => s.setReleasePoint);
  const acknowledgeUnexpectedChange = useWebSocketStore((s) => s.acknowledgeUnexpectedChange);
  const runwaySetup = useRunwaySetup();
  const apronOnline = useApronOnline();
  const twrOnline   = useTwrOnline();
  const isTwr       = useIsTwr();
  const strip = useStrip(callsign);
  const myPosition = useMyPosition();
  const airport = useWebSocketStore((s) => s.airport);
  const controllers = useWebSocketStore((s) => s.controllers);

  const stripType = strip?.origin === airport ? "dep"
    : strip?.destination === airport ? "arr"
    : undefined;

  const visibilityContext: VisibilityContext = {
    dep: runwaySetup.departure,
    arr: runwaySetup.arrival,
    stripType,
    apronOnline,
    twrOnline,
    isTwr: isTwr ?? false,
  };

  const shouldAcknowledgeReleasePoint =
    strip?.unexpected_change_fields?.includes("release_point") && !!myPosition && strip.owner === myPosition;

  const isNotOwner = !!myPosition && !!strip?.owner && strip.owner !== myPosition;
  const ownerSection = controllers.find((c) => c.position === strip?.owner)?.section;
  const coordinationPoints =
    ownerSection === "TWR"
      ? COORDINATION_WITH_TWR_TAXI_MAP_POINTS
      : COORDINATION_WITH_APRON_TAXI_MAP_POINTS;

  const handleSelect = (label: string) => {
    if (label !== strip?.release_point) {
      setReleasePoint(callsign, label);
    }
    if (shouldAcknowledgeReleasePoint) {
      acknowledgeUnexpectedChange(callsign, "release_point");
    }
    onOpenChange(false);
  };

  const handleErase = () => {
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
      imageSrc="/taxi_map.webp"
      imageAlt="TWR taxi map"
      imgWidth={1801}
      imgHeight={1013}
      points={isNotOwner ? coordinationPoints : TAXI_MAP_POINTS}
      btnStyle={BTN_STYLE}
      onSelect={handleSelect}
      visibilityContext={visibilityContext}
      selectedPoint={strip?.release_point}
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
          gap: "0.39cqh",
        }}
      >
        <MapEraseControls onOk={handleSelect} onErase={handleErase} btnStyle={BTN_STYLE} />
      </div>
    </MapDialogShell>
  );
}
