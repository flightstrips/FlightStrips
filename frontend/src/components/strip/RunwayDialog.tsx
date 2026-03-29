import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";
import { useSelectedCallsign, useWebSocketStore } from "@/store/store-hooks";

const RUNWAYS = ["04R", "04L", "12", "22R", "22L", "30"];

// All sizes derived from 1920×1080 base:
//   horizontal → / 1920 * 100 = vw
//   vertical   → / 1080 * 100 = vh
//
// Dialog: 167px → 8.7vw
// Panel mx: 8px → 0.42vw  |  my: 11px → 1.02vh
// List gap: 12px → 1.11vh  |  pt: 14px → 1.3vh  |  pb: 16px → 1.48vh
// Button: 108×70px → 5.63vw × 6.48vh  |  font: 28px → 1.46vw
// ESC row pb: 24px → 2.22vh

// Tailwind class constants (hex must be literal strings for JIT) — styled to match Runways.svg
const CLS_DIALOG_BG      = "bg-[#B3B3B3] border border-black p-0 w-[8.7vw] max-w-none max-h-none gap-0 overflow-hidden [&>button]:hidden";
const CLS_PANEL          = "mx-[0.42vw] my-[1.02vh] border border-black flex flex-col justify-between h-fit";
const CLS_RWY_LIST       = "flex flex-col items-center gap-[1.11vh] pt-[1.3vh] pb-[1.48vh]";
const CLS_RWY_BTN        = "w-[5.63vw] h-[6.48vh] bg-[#CCCCCC] text-black font-semibold text-[1.46vw] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-90";
const CLS_RWY_BTN_ACTIVE = "w-[5.63vw] h-[6.48vh] bg-[#1BFF16] text-black font-semibold text-[1.46vw] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-90";
const CLS_ESC_ROW        = "flex items-center justify-center pb-[2.22vh]";
const CLS_ESC_BTN        = "w-[5.63vw] h-[6.48vh] bg-[#3F3F3F] text-white font-semibold text-[1.46vw] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-75";

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
  const createTacticalStrip = useWebSocketStore(s => s.createTacticalStrip);
  const assignRunway = useWebSocketStore(s => s.assignRunway);
  const selectedAircraft = useSelectedCallsign();
  //const runwaySetup = useRunwaySetup();

  const runways = /*props.mode === "ASSIGN"
    ? (props.direction === "departure" ? runwaySetup.departure : runwaySetup.arrival)
    :*/ RUNWAYS;

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
      <DialogContent className={CLS_DIALOG_BG}>
        <DialogTitle className="sr-only">Select Runway — {title}</DialogTitle>
        <div className={CLS_PANEL}>
          <div className={CLS_RWY_LIST}>
            {runways.map(rwy => (
              <button
                key={rwy}
                className={rwy === currentRunway ? CLS_RWY_BTN_ACTIVE : CLS_RWY_BTN}
                onClick={() => handleSelect(rwy)}
              >
                {rwy}
              </button>
            ))}
          </div>
          <div className={CLS_ESC_ROW}>
            <button
              className={CLS_ESC_BTN}
              onClick={() => onOpenChange(false)}
            >
              ESC
            </button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
