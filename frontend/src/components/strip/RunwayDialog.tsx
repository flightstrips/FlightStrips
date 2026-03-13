import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";
import { useSelectedCallsign, useWebSocketStore } from "@/store/store-hooks";

const RUNWAYS = ["04R", "04L", "12", "22R", "22L", "30"];

// Tailwind class constants (hex must be literal strings for JIT)
const CLS_DIALOG_BG      = "bg-[#B3B3B3] border border-black p-0 w-[167px] gap-0 overflow-hidden [&>button]:hidden";
const CLS_RWY_BTN        = "w-full h-[70px] bg-[#CCCCCC] text-black font-semibold text-[28px] font-[Rubik] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-90";
const CLS_RWY_BTN_ACTIVE = "w-full h-[70px] bg-[#2CBB00] text-white font-semibold text-[28px] font-[Rubik] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-90";
const CLS_ESC_BTN        = "w-full h-[70px] bg-[#3F3F3F] text-white font-semibold text-[28px] font-[Rubik] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-75";

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

type Props = TacticalProps | AssignProps;

export function RunwayDialog(props: Props) {
  const { open, onOpenChange } = props;
  const createTacticalStrip = useWebSocketStore(s => s.createTacticalStrip);
  const assignRunway = useWebSocketStore(s => s.assignRunway);
  const selectedAircraft = useSelectedCallsign();
  //const runwaySetup = useRunwaySetup();

  const runways = /*props.mode === "ASSIGN"
    ? (props.direction === "departure" ? runwaySetup.departure : runwaySetup.arrival)
    :*/ RUNWAYS;

  const title = props.mode === "ASSIGN" ? "Assign Runway" : props.type;
  const currentRunway = props.mode === "ASSIGN" ? props.currentRunway : undefined;

  function handleSelect(runway: string) {
    if (props.mode === "ASSIGN") {
      assignRunway(props.callsign, runway);
    } else {
      createTacticalStrip(props.type, props.bay, runway, selectedAircraft ?? "");
    }
    onOpenChange(false);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className={CLS_DIALOG_BG}>
        <DialogTitle className="sr-only">Select Runway — {title}</DialogTitle>
        <div className="border border-black mx-[7px] mt-[11px] mb-0 flex flex-col gap-0 p-[9px] pb-0">
          {runways.map(rwy => (
            <div key={rwy} className="pb-[9px]">
              <button
                className={rwy === currentRunway ? CLS_RWY_BTN_ACTIVE : CLS_RWY_BTN}
                onClick={() => handleSelect(rwy)}
              >
                {rwy}
              </button>
            </div>
          ))}
          <div className="pb-[9px]">
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
