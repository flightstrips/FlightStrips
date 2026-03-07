import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";
import { useSelectedCallsign, useWebSocketStore } from "@/store/store-hooks";

const RUNWAYS = ["04R", "04L", "12", "22R", "22L", "30"];

interface Props {
  open: boolean;
  bay: string;
  type: "START" | "LAND";
  onOpenChange: (open: boolean) => void;
}

export function RunwayDialog({ open, bay, type, onOpenChange }: Props) {
  const createTacticalStrip = useWebSocketStore(s => s.createTacticalStrip);
  const selectedAircraft = useSelectedCallsign();

  function handleSelect(runway: string) {
    createTacticalStrip(type, bay, runway, selectedAircraft ?? "");
    onOpenChange(false);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="bg-[#B3B3B3] border border-black p-0 w-[167px] gap-0 overflow-hidden [&>button]:hidden">
        <DialogTitle className="sr-only">Select Runway — {type}</DialogTitle>
        <div className="border border-black mx-[7px] mt-[11px] mb-0 flex flex-col gap-0 p-[9px] pb-0">
          {RUNWAYS.map(rwy => (
            <div key={rwy} className="pb-[9px]">
              <button
                className="w-full h-[70px] bg-[#CCCCCC] text-black font-semibold text-[28px] font-[Rubik] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-90"
                onClick={() => handleSelect(rwy)}
              >
                {rwy}
              </button>
            </div>
          ))}
          <div className="pb-[9px]">
            <button
              className="w-full h-[70px] bg-[#3F3F3F] text-white font-semibold text-[28px] font-[Rubik] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-75"
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
