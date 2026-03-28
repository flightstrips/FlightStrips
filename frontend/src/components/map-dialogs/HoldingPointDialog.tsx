import { useWebSocketStore } from "@/store/store-hooks";
import { HOLDING_POINT_RUNWAYS } from "@/config/ekch";
import { MAP_BTN_BASE, MapDialogShell } from "./MapDialogShell";

interface HoldingPointDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  callsign: string;
  /** Pre-select the runway matching the strip's assigned runway (e.g. "22R"). */
  runway?: string;
}

const BTN_STYLE: React.CSSProperties = {
  ...MAP_BTN_BASE,
  width: 70,
  height: 40,
};
export function HoldingPointDialog({
  open,
  onOpenChange,
  callsign,
  runway,
}: HoldingPointDialogProps) {
  const setReleasePoint = useWebSocketStore((s) => s.setReleasePoint);

  const activeRunway = HOLDING_POINT_RUNWAYS.find((r) => r.runway === runway)
    ?? HOLDING_POINT_RUNWAYS[0];

  const handleSelect = (label: string) => {
    setReleasePoint(callsign, label);
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
      btnStyle={BTN_STYLE}
      onSelect={handleSelect}
      scaleMode="width"
    />
  );
}
