import { useEffect, useMemo, useState } from "react";

import { getSimpleAircraftType } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { formatTimeLabel } from "@/components/eset/metadata";
import type { FrontendStrip } from "@/api/models";
import type { EsetMenuAnchor } from "@/components/eset/EsetStandMenu";

const MENU_WIDTH = 190;

// Tailwind class constants (hex must be literal strings for JIT)
const CLS_POPUP     = "absolute w-[190px] border border-black bg-[#B3B3B3] p-2 shadow-2xl";
const CLS_LIST_BTN  = "w-full border-b border-black/10 px-2 py-1.5 text-left hover:bg-[#e7e7e7]";

interface EsetStandStatusDialogProps {
  open: boolean;
  stand: string;
  anchor: EsetMenuAnchor | null;
  strip?: FrontendStrip;
  nonClearedStrips: FrontendStrip[];
  onClose: () => void;
  onOccupied: () => void;
  onVacant: () => void;
  onClearFpl: () => void;
  onAssignPlannedDeparture: (strip: FrontendStrip) => void;
}

export default function EsetStandStatusDialog({
  open,
  stand,
  anchor,
  strip,
  nonClearedStrips,
  onClose,
  onOccupied,
  onVacant,
  onClearFpl,
  onAssignPlannedDeparture,
}: EsetStandStatusDialogProps) {
  const [showPlannedDepartures, setShowPlannedDepartures] = useState(false);

  useEffect(() => {
    if (!open) {
      return undefined;
    }

    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose();
      }
    };

    window.addEventListener("keydown", handleEscape);
    return () => window.removeEventListener("keydown", handleEscape);
  }, [open, onClose]);

  const position = useMemo(() => {
    if (!anchor) {
      return { top: 32, left: 32 };
    }

    const preferredLeft = anchor.right + 12;
    const fallbackLeft = anchor.left - MENU_WIDTH - 12;
    const left =
      preferredLeft + MENU_WIDTH <= window.innerWidth - 16
        ? preferredLeft
        : Math.max(16, fallbackLeft);
    const top = Math.min(Math.max(16, anchor.top), window.innerHeight - 400);

    return { left, top };
  }, [anchor]);

  if (!open) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-40" onMouseDown={onClose}>
      <div
        className={CLS_POPUP}
        style={position}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="mt-3 flex flex-col gap-2 text-black">
          <div className="bg-white px-2 py-1 text-center text-lg">{stand}</div>

          <Button variant="trf" className="h-11 text-sm font-semibold" onClick={onOccupied}>
            OCCUPIED
          </Button>
          <Button variant="trf" className="h-11 text-sm font-semibold" onClick={onVacant}>
            VACANT
          </Button>
          <Button
            variant="trf"
            className="h-11 text-sm font-semibold"
            onClick={onClearFpl}
            disabled={!strip}
          >
            CLEAR FPL
          </Button>
          <Button
            variant="trf"
            className="h-11 text-sm font-semibold"
            onClick={() => setShowPlannedDepartures((current) => !current)}
          >
            PLANNED DEP
          </Button>

          {showPlannedDepartures && (
            <div className="max-h-48 overflow-y-auto border border-black bg-white text-xs">
              {nonClearedStrips.map((plannedStrip) => (
                <button
                  key={plannedStrip.callsign}
                  type="button"
                  className={CLS_LIST_BTN}
                  onClick={() => onAssignPlannedDeparture(plannedStrip)}
                >
                  {plannedStrip.callsign} — {getSimpleAircraftType(plannedStrip.aircraft_type) || "—"} — {formatTimeLabel(plannedStrip.tobt)}
                </button>
              ))}
              {nonClearedStrips.length === 0 && (
                <div className="px-2 py-3 text-center text-black/70">No planned departures.</div>
              )}
            </div>
          )}
        </div>

        <div className="mt-3">
          <Button variant="darkaction" className="h-11 w-full" onClick={onClose}>
            ESC
          </Button>
        </div>
      </div>
    </div>
  );
}
