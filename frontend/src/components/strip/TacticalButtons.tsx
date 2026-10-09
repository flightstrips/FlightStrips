import { useState } from "react";
import { Bay } from "@/api/models";
import { useWebSocketStore } from "@/store/store-hooks";
import { MemaidDialog } from "./MemaidDialog";
import { RunwayDialog } from "./RunwayDialog";
import { buildDeiceLaneLabel, buildDeicePlatformLabel, type DeiceHeaderArea } from "@/lib/deiceLane";

export function MemAidButton({ bay, className }: { bay: Bay; className?: string }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button className={className} onClick={() => setOpen(true)}>MEM AID</button>
      <MemaidDialog open={open} bay={bay} onOpenChange={setOpen} />
    </>
  );
}

export function StartButton({ bay, className }: { bay: Bay; className?: string }) {
  const [open, setOpen] = useState(false);
  const createTacticalStrip = useWebSocketStore((state) => state.createTacticalStrip);
  const selectedAircraft = useWebSocketStore((state) => state.selectedCallsign);
  const requiresRunway = bay === Bay.TwyArr || bay === Bay.Taxi || bay === Bay.TaxiLwr;

  const handleClick = () => {
    if (requiresRunway) {
      setOpen(true);
      return;
    }
    createTacticalStrip("START", bay, "", selectedAircraft ?? "");
  };

  return (
    <>
      <button className={className} onClick={handleClick}>START</button>
      {requiresRunway && <RunwayDialog open={open} bay={bay} type="START" onOpenChange={setOpen} />}
    </>
  );
}

export function LandButton({ bay, className }: { bay: Bay; className?: string }) {
  const [open, setOpen] = useState(false);
  const createTacticalStrip = useWebSocketStore((state) => state.createTacticalStrip);
  const selectedAircraft = useWebSocketStore((state) => state.selectedCallsign);
  const requiresRunway = bay === Bay.TwyArr || bay === Bay.Taxi || bay === Bay.TaxiLwr;

  const handleClick = () => {
    if (requiresRunway) {
      setOpen(true);
      return;
    }
    createTacticalStrip("LAND", bay, "", selectedAircraft ?? "");
  };

  return (
    <>
      <button className={className} onClick={handleClick}>LAND</button>
      {requiresRunway && <RunwayDialog open={open} bay={bay} type="LAND" onOpenChange={setOpen} />}
    </>
  );
}

const CROSSING_LABEL = "CROSSING TRAFFIC";

export function CrossingButton({
  bay,
  className,
}: {
  bay: Bay;
  className?: string;
}) {
  const createTacticalStrip = useWebSocketStore(s => s.createTacticalStrip);
  const selectedAircraft = useWebSocketStore(s => s.selectedCallsign);
  return (
    <button
      className={className}
      onClick={() => createTacticalStrip("CROSSING", bay, CROSSING_LABEL, selectedAircraft ?? "")}
    >
      X
    </button>
  );
}

export function DeiceLaneButton({
  area,
  lane,
  frequency,
  className,
}: {
  area: Extract<DeiceHeaderArea, "A" | "B">;
  lane: number;
  frequency: string;
  className?: string;
}) {
  const createTacticalStrip = useWebSocketStore((state) => state.createTacticalStrip);

  return (
    <button
      className={className}
      onClick={() => createTacticalStrip("MEMAID", Bay.DeIce, buildDeiceLaneLabel(area, lane, frequency), "")}
    >
      LANE{lane}
    </button>
  );
}

export function DeicePlatformButton({
  platform,
  className,
}: {
  platform: "A" | "B" | "V";
  className?: string;
}) {
  const createTacticalStrip = useWebSocketStore((state) => state.createTacticalStrip);

  return (
    <button
      className={className}
      onClick={() => createTacticalStrip("MEMAID", Bay.DeIce, buildDeicePlatformLabel(platform), "")}
    >
      DI {platform}
    </button>
  );
}
