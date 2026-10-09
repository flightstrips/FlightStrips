import { useEffect, useMemo } from "react";

import { Button } from "@/components/ui/button";
import { Bay, type FrontendStrip } from "@/api/models";
import { EST_RAISED_BUTTON_EDGE, EST_RAISED_EDGE, EST_SUNKEN_EDGE } from "@/components/est/bevel";
import { useCDMColors } from "@/hooks/useCDMColors";
import { CDM_GREEN, CDM_ORANGE, CDM_RED } from "@/lib/cdmColors";

// Tailwind class constants (hex must be literal strings for JIT)
const CLS_POPUP   = "absolute w-[162px] border border-black bg-[#DEDEDE] p-2 shadow-2xl";
const CLS_CDM_TAG = "px-1 py-0.5 text-xs";
const CLS_BTN     = "h-11 bg-[#B3B3B3] text-sm font-semibold";
// Half a button height (44px) minus the 8px flex gap already applied
const SPACER      = 25;

export interface EstMenuAnchor {
  top: number;
  left: number;
  right: number;
  bottom: number;
}

interface EstStandMenuProps {
  open: boolean;
  anchor: EstMenuAnchor | null;
  strip: FrontendStrip;
  onClose: () => void;
  onStartTransfer: () => void;
  startTransferDisabled: boolean;
  onStartRequest: () => void;
  onPush: () => void;
  onTaxi: () => void;
  onOpenDeIce: () => void;
  onOpenFlightPlan: () => void;
  onToggleMarked: () => void;
  onOpenStandStatus: () => void;
}

const MENU_WIDTH = 162;

export default function EstStandMenu({
  open,
  anchor,
  strip,
  onClose,
  onStartTransfer,
  startTransferDisabled,
  onStartRequest,
  onPush,
  onTaxi,
  onOpenDeIce,
  onOpenFlightPlan,
  onToggleMarked,
  onOpenStandStatus,
}: EstStandMenuProps) {
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

  const { tobtBg, tsatBg } = useCDMColors({ bay: strip.bay as Bay, tsat: strip.tsat, tobt: strip.tobt, phase: strip.phase });
  const cdmTextColor = (bg: string | undefined) => (!bg || bg === CDM_GREEN ? "black" : "white");
  // TSAT has not yet entered its green window (more than 5 min ahead).
  const tsatNotYetGreen = !!strip.tsat && !tsatBg && tobtBg !== CDM_RED
    && (strip.bay === Bay.NotCleared || strip.bay === Bay.Cleared);
  const readyBg = tsatNotYetGreen ? CDM_ORANGE : CDM_GREEN;

  if (!open || !anchor) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-40" onMouseDown={onClose}>
      <div
        className={CLS_POPUP}
        style={{ ...position, ...EST_RAISED_EDGE }}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="mt-3 text-black">
          <div className="flex flex-col gap-[6px]">
            <div
              className="flex h-[18px] items-center justify-center bg-[#D3D3D3] px-2 text-sm leading-none"
              style={EST_SUNKEN_EDGE}
            >
              {strip.stand}
            </div>
            <div className="flex h-[18px] items-center justify-center bg-[#D3D3D3] px-2 text-sm leading-none" style={EST_SUNKEN_EDGE}>{strip.callsign}</div>
            <div className="grid grid-cols-2 gap-1 text-xs font-semibold">
              <div className={CLS_CDM_TAG} style={{ backgroundColor: tobtBg || "#D3D3D3", color: cdmTextColor(tobtBg), ...EST_SUNKEN_EDGE }}>
                <span>TOBT </span>
                <span>{strip.tobt}</span>
              </div>
              <div className={CLS_CDM_TAG} style={{ backgroundColor: tsatBg || "#D3D3D3", color: cdmTextColor(tsatBg), ...EST_SUNKEN_EDGE }}>TSAT {strip.tsat}</div>
            </div>
            <div
              className="flex h-[18px] items-center justify-center px-2 text-sm leading-none"
              style={{ backgroundColor: strip.start_req ? readyBg : "#D3D3D3", color: strip.start_req ? cdmTextColor(readyBg) : "black", ...EST_SUNKEN_EDGE }}
            >
              {strip.start_req ? "READY" : "\u00A0"}
            </div>
          </div>

          <div className="flex flex-col gap-2" style={{ marginTop: SPACER + 8 }}>
            <Button
              variant="trf"
              className={CLS_BTN}
              style={EST_RAISED_BUTTON_EDGE}
              onClick={onStartTransfer}
              disabled={startTransferDisabled}
            >
              START REQ+TRF
            </Button>
            <Button variant="trf" className={CLS_BTN} style={EST_RAISED_BUTTON_EDGE} onClick={onStartRequest}>
              {strip.start_req ? "REMOVE START REQ" : "START REQ"}
            </Button>
            <Button variant="trf" className={CLS_BTN} style={{ ...EST_RAISED_BUTTON_EDGE, marginTop: SPACER }} onClick={onPush}>
              PUSH
            </Button>
            <Button variant="trf" className={CLS_BTN} style={{ ...EST_RAISED_BUTTON_EDGE, marginTop: SPACER }} onClick={onTaxi}>
              TAXI
            </Button>
            <Button variant="trf" className={CLS_BTN} style={EST_RAISED_BUTTON_EDGE} onClick={onOpenDeIce}>
              DE-ICE
            </Button>
            <Button variant="trf" className={CLS_BTN} style={{ ...EST_RAISED_BUTTON_EDGE, marginTop: SPACER }} onClick={onOpenFlightPlan}>
              VIEW FPL
            </Button>
            <Button variant="trf" className={CLS_BTN} style={EST_RAISED_BUTTON_EDGE} onClick={onToggleMarked}>
              {strip.marked ? "UNMARK" : "MARK"}
            </Button>
            <Button variant="trf" className={CLS_BTN} style={EST_RAISED_BUTTON_EDGE} onClick={onOpenStandStatus}>
              STAND STATUS
            </Button>
          </div>
        </div>

        <div className="mt-3">
          <Button variant="darkaction" className="h-11 w-full" style={EST_RAISED_BUTTON_EDGE} onClick={onClose}>
            ESC
          </Button>
        </div>
      </div>
    </div>
  );
}
