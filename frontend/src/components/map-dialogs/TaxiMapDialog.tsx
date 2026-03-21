import { useWebSocketStore, useRunwaySetup, useApronOnline, useTwrOnline, useIsTwr } from "@/store/store-hooks";
import { TAXI_MAP_POINTS } from "@/config/ekch";
import type { VisibilityContext } from "@/config/ekch";
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

export function TaxiMapDialog({
  open,
  onOpenChange,
  callsign,
}: TaxiMapDialogProps) {
  const setReleasePoint = useWebSocketStore((s) => s.setReleasePoint);
  const runwaySetup = useRunwaySetup();
  const apronOnline = useApronOnline();
  const twrOnline   = useTwrOnline();
  const isTwr       = useIsTwr();
  const strip   = useWebSocketStore((s) => s.strips.find((x) => x.callsign === callsign));
  const airport = useWebSocketStore((s) => s.airport);

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

  const handleSelect = (label: string) => {
    setReleasePoint(callsign, label);
    onOpenChange(false);
  };

  const handleErase = () => {
    setReleasePoint(callsign, "");
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
      points={TAXI_MAP_POINTS}
      btnStyle={BTN_STYLE}
      onSelect={handleSelect}
      visibilityContext={visibilityContext}
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
        <MapEraseControls onOk={handleSelect} onErase={handleErase} btnStyle={BTN_STYLE} />
      </div>
    </MapDialogShell>
  );
}