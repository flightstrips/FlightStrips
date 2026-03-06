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
      <DialogContent className="bg-[#393939] border-2 border-white p-4 w-64">
        <DialogTitle className="text-white font-bold text-lg mb-2">
          SELECT RUNWAY — {type}
        </DialogTitle>
        {selectedAircraft && (
          <p className="text-[#aaa] text-xs mb-2">Aircraft: {selectedAircraft}</p>
        )}
        <div className="grid grid-cols-3 gap-2">
          {RUNWAYS.map(rwy => (
            <button
              key={rwy}
              className="bg-[#555355] text-white border-2 border-white px-2 py-2 text-sm font-bold outline-none active:bg-[#424242]"
              onClick={() => handleSelect(rwy)}
            >
              {rwy}
            </button>
          ))}
        </div>
        <div className="flex justify-end mt-3">
          <button
            className="bg-[#646464] text-white font-bold text-sm px-4 py-1 border-2 border-white active:bg-[#424242]"
            onClick={() => onOpenChange(false)}
          >
            CANCEL
          </button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
