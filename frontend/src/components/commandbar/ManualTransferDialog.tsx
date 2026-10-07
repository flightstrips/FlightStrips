import type { CSSProperties } from "react";
import * as VisuallyHidden from "@radix-ui/react-visually-hidden";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import type { FrontendController } from "@/api/models";

// Laid out on the 841x394 design frame and scaled against a 1920 px wide screen.
const px = (n: number) => `${((n * 0.8 / 1920) * 100).toFixed(3)}vw`;

interface Station {
  label: string;
  frequency: string;
  /** Sectors a cross-coupled controller must own to cover this station. */
  sectors: string[];
}

const STATIONS: Station[] = [
  { label: "Apron ARR", frequency: "121.630", sectors: ["AA"] },
  { label: "GND WEST", frequency: "118.580", sectors: ["GW"] },
  { label: "TWR ARR", frequency: "118.105", sectors: ["TW"] },
  { label: "DEP K", frequency: "124.980", sectors: [] },
  { label: "Apron DEP", frequency: "121.730", sectors: ["AD"] },
  { label: "GND EAST", frequency: "121.830", sectors: ["GE"] },
  { label: "TWR DEP", frequency: "119.355", sectors: ["TE"] },
  { label: "DEP R", frequency: "120.255", sectors: [] },
];

const COLUMN_X = [66, 252, 438, 624];
const ROW_Y = [84, 179];

const RAISED: CSSProperties = {
  border: "2px solid",
  borderColor: "#CECECE #666666 #666666 #CECECE",
  boxShadow: "inset 2px 2px 2px -1px rgba(206,206,206,0.55), inset -2px -2px 2px -1px rgba(102,102,102,0.55)",
  boxSizing: "border-box",
};

const normalizeSector = (sector: string) => (sector === "GWA" || sector === "GWD" ? "GW" : sector);

/** The online controller that currently covers a station: its own position, or a cross-coupled controller. */
function findStationController(
  station: Station,
  controllers: FrontendController[],
  ownPosition: string,
  ownCallsign: string,
): FrontendController | undefined {
  const candidates = controllers.filter(
    (c) =>
      c.observer !== true &&
      !c.callsign.toUpperCase().endsWith("_OBS") &&
      c.position !== ownPosition &&
      c.callsign.toUpperCase() !== ownCallsign.toUpperCase(),
  );
  return (
    candidates.find((c) => c.position === station.frequency) ??
    candidates.find((c) => (c.owned_sectors ?? []).some((s) => station.sectors.includes(normalizeSector(s))))
  );
}

interface ManualTransferDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Viewport x (px) the ESC button should be centred on. */
  anchorX?: number;
  controllers: FrontendController[];
  ownPosition: string;
  ownCallsign: string;
  onTransfer: (toPosition: string) => void;
}

export function ManualTransferDialog({
  open,
  onOpenChange,
  anchorX,
  controllers,
  ownPosition,
  ownCallsign,
  onTransfer,
}: ManualTransferDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="top-auto bottom-[8dvh] left-[62.5%] translate-y-0 max-w-none gap-0 overflow-hidden border border-black bg-[#B3B3B3] p-0 [&>button]:hidden"
        style={{
          width: px(841),
          height: px(394),
          ...(anchorX !== undefined
            ? { left: `calc(${anchorX}px - ${((331 * 0.8) / 1920) * 100}vw)`, translate: "none" }
            : {}),
        }}
      >
        <VisuallyHidden.Root>
          <DialogTitle>Manual transfer</DialogTitle>
        </VisuallyHidden.Root>
        <div
          className="absolute border border-black"
          style={{ left: px(36), top: px(17), right: px(36), bottom: px(25) }}
        />
        <div
          className="absolute w-full text-center text-black"
          style={{ top: px(32), fontFamily: "var(--font-bay)", fontSize: px(20) }}
        >
          MANUAL TRANSFER
        </div>

        {STATIONS.map((station, i) => {
          const target = findStationController(station, controllers, ownPosition, ownCallsign);
          const enabled = !!target;
          const x = COLUMN_X[i % 4];
          const y = ROW_Y[Math.floor(i / 4)];
          return (
            <div key={station.label}>
              <button
                type="button"
                disabled={!enabled}
                onClick={() => {
                  if (target) onTransfer(target.position);
                }}
                className="absolute flex items-center justify-center whitespace-nowrap outline-none"
                style={{
                  ...RAISED,
                  left: px(x),
                  top: px(y),
                  width: px(155),
                  height: px(57),
                  background: "#D6D6D6",
                  color: enabled ? "black" : "#8A8A8A",
                  cursor: enabled ? "pointer" : "default",
                  fontFamily: "var(--font-bay)",
                  fontWeight: 700,
                  fontSize: px(26),
                }}
              >
                {station.label}
              </button>
              <div
                className="absolute text-center"
                style={{
                  left: px(x),
                  top: px(y + 62),
                  width: px(155),
                  fontFamily: "var(--font-bay)",
                  fontSize: px(13),
                  color: enabled ? "black" : "#8A8A8A",
                }}
              >
                {station.frequency}
              </div>
            </div>
          );
        })}

        {[
          { label: "ESC", x: 250, action: () => onOpenChange(false) },
          { label: "FREE", x: 439, action: () => {} },
        ].map(({ label, x, action }) => (
          <div key={label}>
          <button
            type="button"
            onClick={action}
            className="absolute flex items-center justify-center outline-none"
            style={{
              ...RAISED,
              left: px(x),
              top: px(296),
              width: px(162),
              height: px(54),
              background: "#3F3F3F",
              color: "white",
              cursor: "pointer",
              fontFamily: "var(--font-bay)",
              fontWeight: 700,
              fontSize: px(26),
            }}
          >
            {label}
          </button>
          </div>
        ))}
      </DialogContent>
    </Dialog>
  );
}
