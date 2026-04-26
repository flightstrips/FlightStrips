import { useEffect, useMemo, useState } from "react";

import { getAircraftTypeWithWtc } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { formatTimeLabel } from "@/components/est/metadata";
import type { FrontendStrip } from "@/api/models";
import type { EstMenuAnchor } from "@/components/est/EstStandMenu";
import { scalePx } from "@/lib/viewportScale";

const MENU_WIDTH = 190;

// Tailwind class constants (hex must be literal strings for JIT)
const CLS_POPUP     = "absolute border border-black bg-[#B3B3B3] shadow-2xl";
const CLS_LIST_BTN  = "w-full border-b border-black/10 px-2 py-1.5 text-left hover:bg-[#e7e7e7]";

interface EstStandStatusDialogProps {
  open: boolean;
  stand: string;
  anchor: EstMenuAnchor | null;
  strip?: FrontendStrip;
  nonClearedStrips: FrontendStrip[];
  onClose: () => void;
  onOccupied: () => void;
  onVacant: () => void;
  onClearFpl: () => void;
  onAssignPlannedDeparture: (strip: FrontendStrip) => void;
}

export default function EstStandStatusDialog({
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
}: EstStandStatusDialogProps) {
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
    const scaledMenuWidth = Math.min(
      (MENU_WIDTH / 1920) * window.innerWidth,
      (MENU_WIDTH / 1080) * window.innerHeight,
    );

    if (!anchor) {
      return { top: 32, left: 32 };
    }

    const preferredLeft = anchor.right + 12;
    const fallbackLeft = anchor.left - scaledMenuWidth - 12;
    const left =
      preferredLeft + scaledMenuWidth <= window.innerWidth - 16
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
        style={{ ...position, width: scalePx(MENU_WIDTH), padding: scalePx(8) }}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="flex flex-col text-black" style={{ marginTop: scalePx(12), gap: scalePx(8) }}>
          <div className="bg-white text-center" style={{ padding: `${scalePx(4)} ${scalePx(8)}`, fontSize: scalePx(18) }}>{stand}</div>

          <Button variant="trf" className="font-semibold" style={{ height: scalePx(44), fontSize: scalePx(14) }} onClick={onOccupied}>
            OCCUPIED
          </Button>
          <Button variant="trf" className="font-semibold" style={{ height: scalePx(44), fontSize: scalePx(14) }} onClick={onVacant}>
            VACANT
          </Button>
          <Button
            variant="trf"
            className="font-semibold"
            style={{ height: scalePx(44), fontSize: scalePx(14) }}
            onClick={onClearFpl}
            disabled={!strip}
          >
            CLEAR FPL
          </Button>
          <Button
            variant="trf"
            className="font-semibold"
            style={{ height: scalePx(44), fontSize: scalePx(14) }}
            onClick={() => setShowPlannedDepartures((current) => !current)}
          >
            PLANNED DEP
          </Button>

          {showPlannedDepartures && (
            <div className="overflow-y-auto border border-black bg-white" style={{ maxHeight: scalePx(192), fontSize: scalePx(12) }}>
              {nonClearedStrips.map((plannedStrip) => (
                <button
                  key={plannedStrip.callsign}
                  type="button"
                  className={CLS_LIST_BTN}
                  onClick={() => onAssignPlannedDeparture(plannedStrip)}
                >
                  {plannedStrip.callsign} — {getAircraftTypeWithWtc(plannedStrip.aircraft_type, plannedStrip.aircraft_category) || "—"} — {formatTimeLabel(plannedStrip.tobt)}
                </button>
              ))}
              {nonClearedStrips.length === 0 && (
                <div className="text-center text-black/70" style={{ padding: `${scalePx(12)} ${scalePx(8)}` }}>No planned departures.</div>
              )}
            </div>
          )}
        </div>

        <div style={{ marginTop: scalePx(12) }}>
          <Button variant="darkaction" className="w-full" style={{ height: scalePx(44), fontSize: scalePx(24) }} onClick={onClose}>
            ESC
          </Button>
        </div>
      </div>
    </div>
  );
}
