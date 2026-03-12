import { useEffect, useMemo } from "react";

import { Button } from "@/components/ui/button";
import { formatTimeLabel } from "@/components/eset/metadata";
import type { FrontendStrip } from "@/api/models";

// Tailwind class constants (hex must be literal strings for JIT)
const CLS_POPUP   = "absolute w-[190px] border border-black bg-[#B3B3B3] p-2 shadow-2xl";
const CLS_CDM_TAG = "bg-[#2CBB00] text-white font-bold px-1 py-0.5 text-xs"; // CDM green for TOBT/TSAT tags

export interface EsetMenuAnchor {
  top: number;
  left: number;
  right: number;
  bottom: number;
}

interface EsetStandMenuProps {
  open: boolean;
  anchor: EsetMenuAnchor | null;
  strip: FrontendStrip;
  onClose: () => void;
  onSendReady: () => void;
  onStartTransfer: () => void;
  onStartRequest: () => void;
  onPush: () => void;
  onTaxi: () => void;
  onOpenDeIce: () => void;
  onOpenFlightPlan: () => void;
  onToggleMarked: () => void;
  onOpenStandStatus: () => void;
}

const MENU_WIDTH = 190;

export default function EsetStandMenu({
  open,
  anchor,
  strip,
  onClose,
  onSendReady,
  onStartTransfer,
  onStartRequest,
  onPush,
  onTaxi,
  onOpenDeIce,
  onOpenFlightPlan,
  onToggleMarked,
  onOpenStandStatus,
}: EsetStandMenuProps) {
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
    const left = preferredLeft + MENU_WIDTH <= window.innerWidth - 16
      ? preferredLeft
      : Math.max(16, fallbackLeft);
    const top = Math.min(Math.max(16, anchor.top), window.innerHeight - 770);

    return { left, top };
  }, [anchor]);

  if (!open || !anchor) {
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
          <div className="bg-white px-2 py-1 text-center text-lg">{strip.stand}</div>
          <div className="mt-1 bg-white px-2 py-1 text-center text-lg">{strip.callsign}</div>
          <div className="mt-1 grid grid-cols-2 gap-1 text-xs font-semibold">
            <div className={CLS_CDM_TAG}>TOBT {formatTimeLabel(strip.tobt)}</div>
            <div className={CLS_CDM_TAG}>TSAT {formatTimeLabel(strip.tsat)}</div>
          </div>
          <Button
            variant="trf"
            type="button"
            className="mt-1 w-full px-2 py-2 text-sm font-semibold"
            onClick={onSendReady}
          >
            SEND RDY MSG
          </Button>

          <Button variant="trf" className="h-11 text-sm font-semibold" onClick={onStartTransfer}>
            START+TRF
          </Button>
          <Button variant="trf" className="h-11 text-sm font-semibold" onClick={onStartRequest}>
            START REQ
          </Button>
          <Button variant="trf" className="h-11 text-sm font-semibold" onClick={onPush}>
            PUSH
          </Button>
          <Button variant="trf" className="h-11 text-sm font-semibold" onClick={onTaxi}>
            TAXI
          </Button>
          <Button variant="trf" className="h-11 text-sm font-semibold" onClick={onOpenDeIce}>
            DE-ICE
          </Button>
          <Button variant="trf" className="h-11 text-sm font-semibold" onClick={onOpenFlightPlan}>
            VIEW FPL
          </Button>
          <Button variant="trf" className="h-11 text-sm font-semibold" onClick={onToggleMarked}>
            {strip.marked ? "UNMARK" : "MARK"}
          </Button>
          <Button variant="trf" className="h-11 text-sm font-semibold" onClick={onOpenStandStatus}>
            STAND STATUS
          </Button>
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
