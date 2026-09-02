import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import DeleteConfirmDialog from "@/components/commandbar/DeleteConfirmDialog";
import { Bay, type FrontendStrip } from "@/api/models";
import type { EstMenuAnchor } from "@/components/est/EstStandMenu";
import { getBridgeStatus, getVgdsStatus } from "@/components/est/metadata";
import { scalePx } from "@/lib/viewportScale";

const MENU_WIDTH = 543;
const COMMAND_WIDTH = 160;

// Tailwind class constants (hex must be literal strings for JIT)
const CLS_POPUP     = "absolute border border-black bg-[#B3B3B3] shadow-2xl";
const CLS_STATUS_VALUE = "bg-white text-left text-black";
interface EstStandStatusDialogProps {
  open: boolean;
  stand: string;
  anchor: EstMenuAnchor | null;
  strip?: FrontendStrip;
  blocked: boolean;
  onClose: () => void;
  onOccupied: () => void;
  onVacant: () => void;
  onCleared: () => void;
  onClearFpl: () => void;
  onPlannedDeparture: () => void;
}

export default function EstStandStatusDialog({
  open,
  stand,
  anchor,
  strip,
  blocked,
  onClose,
  onOccupied,
  onVacant,
  onCleared,
  onClearFpl,
  onPlannedDeparture,
}: EstStandStatusDialogProps) {
  const [confirmAction, setConfirmAction] = useState<"vacant" | "clear-fpl" | null>(null);

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

  const operationalStatus = blocked
    ? "OCCUPIED"
    : !strip
      ? "VACANT"
      : strip.bay === Bay.NotCleared
        ? "PLANNED DEP"
        : strip.bay === Bay.Cleared
          ? "CLEARED"
          : "OCCUPIED";

  const handleVacantClick = () => {
    if (strip) {
      setConfirmAction("vacant");
      return;
    }

    onVacant();
  };

  const handleConfirm = () => {
    if (confirmAction === "vacant") {
      onVacant();
    } else if (confirmAction === "clear-fpl") {
      onClearFpl();
    }
  };

  return (
    <>
      <div className="fixed inset-0 z-40" onMouseDown={onClose}>
        <div
          className={CLS_POPUP}
          style={{ ...position, width: scalePx(MENU_WIDTH), padding: scalePx(8) }}
          onMouseDown={(event) => event.stopPropagation()}
        >
          <div className="flex text-black" style={{ gap: scalePx(24), padding: scalePx(12) }}>
            <div className="flex shrink-0 flex-col" style={{ width: scalePx(COMMAND_WIDTH), gap: scalePx(6) }}>
              <div className="text-center" style={{ fontSize: scalePx(10) }}>STAND</div>
              <div className="border border-black bg-[#A1A1A1] text-center shadow" style={{ padding: scalePx(5), fontSize: scalePx(14) }}>{stand}</div>
              <div className="text-center" style={{ marginTop: scalePx(4), fontSize: scalePx(10) }}>OPERATIONAL</div>

              <Button variant="trf" className="font-semibold" style={{ height: scalePx(32), fontSize: scalePx(14) }} onClick={onOccupied}>
                OCCUPIED
              </Button>
              <Button variant="trf" className="font-semibold" style={{ height: scalePx(32), fontSize: scalePx(14) }} onClick={handleVacantClick}>
                VACANT
              </Button>
              <Button
                variant="trf"
                className="font-semibold"
                style={{ height: scalePx(32), fontSize: scalePx(14) }}
                onClick={onCleared}
                disabled={strip?.bay !== Bay.NotCleared}
              >
                CLEARED
              </Button>
              <Button
                variant="trf"
                className="font-semibold"
                style={{ height: scalePx(32), fontSize: scalePx(14) }}
                onClick={onPlannedDeparture}
                disabled={strip?.bay !== Bay.Cleared}
              >
                PLANNED DEP
              </Button>
              <Button
                variant="trf"
                className="font-semibold"
                style={{ height: scalePx(32), fontSize: scalePx(14) }}
                onClick={() => setConfirmAction("clear-fpl")}
                disabled={!strip}
              >
                CLEAR FPL
              </Button>

              <Button variant="darkaction" className="self-center" style={{ marginTop: scalePx(2), width: scalePx(78), height: scalePx(32), fontSize: scalePx(18) }} onClick={onClose}>
                ESC
              </Button>
            </div>

            <div className="flex min-w-0 flex-1 flex-col justify-center" style={{ gap: scalePx(12) }}>
              <StatusField label="STAND" value={operationalStatus} testId="est-operational-status" />
              <StatusField label="VGDS" value={getVgdsStatus(stand) ?? "NIL"} />
              <StatusField label="BRIDGE" value={getBridgeStatus(stand) ?? "NIL"} />
            </div>
          </div>
        </div>
      </div>

      {confirmAction && (
        <DeleteConfirmDialog
          onConfirm={handleConfirm}
          onCancel={() => setConfirmAction(null)}
        />
      )}
    </>
  );
}

function StatusField({ label, value, testId }: { label: string; value: string; testId?: string }) {
  return (
    <fieldset className="border border-black" style={{ padding: `${scalePx(5)} ${scalePx(7)} ${scalePx(7)}` }}>
      <legend style={{ padding: `0 ${scalePx(6)}`, fontSize: scalePx(10) }}>{label}</legend>
      <div style={{ fontSize: scalePx(10) }}>STATUS</div>
      <div
        className={CLS_STATUS_VALUE}
        data-testid={testId}
        style={{ minHeight: scalePx(24), padding: `${scalePx(5)} ${scalePx(3)}`, fontSize: scalePx(10) }}
      >
        {value}
      </div>
    </fieldset>
  );
}
