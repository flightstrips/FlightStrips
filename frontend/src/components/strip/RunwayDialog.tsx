import type { CSSProperties } from "react";

import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";
import { scalePx } from "@/lib/viewportScale";
import { useSelectedCallsign, useWebSocketStore } from "@/store/store-hooks";

const RUNWAYS = ["04R", "04L", "12", "22R", "22L", "30"] as const;

const RUNWAY_POSITIONS: Record<(typeof RUNWAYS)[number], { left: number; top: number }> = {
  "12": { left: 226.2, top: 76 },
  "22L": { left: 490.2, top: 84 },
  "22R": { left: 261.2, top: 189 },
  "30": { left: 491.2, top: 250 },
  "04R": { left: 245.2, top: 353 },
  "04L": { left: 20.2, top: 454 },
};

const BUTTON_STYLE: CSSProperties = {
  position: "absolute",
  width: scalePx(71),
  height: scalePx(44.065),
  background: "#D6D6D6",
  color: "#000",
  border: 0,
  boxShadow: `0 ${scalePx(4)} ${scalePx(4)} rgba(0, 0, 0, 0.25)`,
  fontFamily: "Rubik, sans-serif",
  fontSize: scalePx(16),
  fontWeight: 600,
  cursor: "pointer",
};

interface TacticalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode?: "TACTICAL";
  bay: string;
  type: "START" | "LAND";
}

interface AssignProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: "ASSIGN";
  callsign: string;
  direction: "departure" | "arrival";
  currentRunway?: string;
}

interface SelectProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: "SELECT";
  currentRunway?: string;
  onSelect: (runway: string) => void;
}

type Props = TacticalProps | AssignProps | SelectProps;

export function RunwayDialog(props: Props) {
  const { open, onOpenChange } = props;
  const createTacticalStrip = useWebSocketStore((state) => state.createTacticalStrip);
  const assignRunway = useWebSocketStore((state) => state.assignRunway);
  const selectedAircraft = useSelectedCallsign();
  const title = props.mode === "ASSIGN" ? "Assign Runway" : props.mode === "SELECT" ? "Select Runway" : props.type;
  const currentRunway = props.mode === "ASSIGN" || props.mode === "SELECT" ? props.currentRunway : undefined;

  function handleSelect(runway: string) {
    if (props.mode === "ASSIGN") {
      assignRunway(props.callsign, runway);
    } else if (props.mode === "SELECT") {
      props.onSelect(runway);
    } else {
      createTacticalStrip(props.type, props.bay, runway, selectedAircraft ?? "");
    }
    onOpenChange(false);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="fixed block max-w-none max-h-none overflow-hidden rounded-none border border-black bg-[#B3B3B3] p-0 [&>button]:hidden"
        style={{ width: scalePx(589.924), height: scalePx(532.102) }}
      >
        <DialogTitle className="sr-only">Select Runway — {title}</DialogTitle>

        <div
          className="pointer-events-none absolute border border-black"
          style={{
            left: scalePx(8.07),
            top: scalePx(8.79),
            width: scalePx(571.993),
            height: scalePx(510.622),
          }}
        />

        <img
          aria-hidden="true"
          alt=""
          className="pointer-events-none absolute max-w-none"
          src="/runway-selector-lines.svg"
          style={{
            left: scalePx(68.2),
            top: scalePx(107.75),
            width: scalePx(457.348),
            height: scalePx(368.379),
          }}
        />

        <div className="absolute inset-0">
          {RUNWAYS.map((runway) => {
            const position = RUNWAY_POSITIONS[runway];
            return (
              <button
                key={runway}
                className="outline-none active:brightness-90"
                style={{
                  ...BUTTON_STYLE,
                  left: scalePx(position.left),
                  top: scalePx(position.top),
                  background: runway === currentRunway ? "#1BFF16" : BUTTON_STYLE.background,
                }}
                onClick={() => handleSelect(runway)}
              >
                {runway}
              </button>
            );
          })}

          <button
            className="absolute bg-[#3F3F3F] text-white outline-none active:brightness-75"
            style={{
              left: scalePx(440.2),
              top: scalePx(450),
              width: scalePx(119.792),
              height: scalePx(49.09),
              border: 0,
              boxShadow: `0 ${scalePx(4)} ${scalePx(4)} rgba(0, 0, 0, 0.25)`,
              fontFamily: "Rubik, sans-serif",
              fontSize: scalePx(18),
              fontWeight: 600,
            }}
            onClick={() => onOpenChange(false)}
          >
            ESC
          </button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
