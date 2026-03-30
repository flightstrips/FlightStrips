import { useMyPosition, useStrip, useWebSocketStore } from "@/store/store-hooks";
import { HOLDING_POINT_RUNWAYS } from "@/config/ekch";
import { MAP_BTN_BASE, MapDialogShell } from "./MapDialogShell";

interface HoldingPointDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  callsign: string;
  /** Pre-select the runway matching the strip's assigned runway (e.g. "22R"). */
  runway?: string;
  coordinationMode?: boolean;
}

export function HoldingPointDialog({
  open,
  onOpenChange,
  callsign,
  runway,
  coordinationMode = false,
}: HoldingPointDialogProps) {
  const setReleasePoint = useWebSocketStore((s) => s.setReleasePoint);
  const acknowledgeUnexpectedChange = useWebSocketStore((s) => s.acknowledgeUnexpectedChange);
  const strip = useStrip(callsign);
  const myPosition = useMyPosition();

  const activeRunway = HOLDING_POINT_RUNWAYS.find((r) => r.runway === runway)
    ?? HOLDING_POINT_RUNWAYS[0];

  // Sizes as % of the container so they scale with the dialog (70×40px reference per runway).
  const btnStyle: React.CSSProperties = {
    ...MAP_BTN_BASE,
    width: `${(70 / activeRunway.imgWidth * 100).toFixed(2)}%`,
    height: `${(40 / activeRunway.imgHeight * 100).toFixed(2)}cqh`,
  };

  const shouldAcknowledgeReleasePoint =
    coordinationMode
    && strip?.unexpected_change_fields?.includes("release_point")
    && !!myPosition
    && strip.owner === myPosition;

  const handleSelect = (label: string) => {
    if (label !== strip?.release_point) {
      setReleasePoint(callsign, label);
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
      title="Select Holding Point"
      imageSrc={activeRunway.imageSrc}
      imageAlt={`Runway ${activeRunway.runway} holding points`}
      imgWidth={activeRunway.imgWidth}
      imgHeight={activeRunway.imgHeight}
      points={activeRunway.points}
      btnStyle={btnStyle}
      onSelect={handleSelect}
      selectedPoint={strip?.release_point}
      scaleMode="width"
    />
  );
}
