import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogTrigger,
} from "@/components/ui/dialog";
import { useControllers, useSelectedCallsign, useWebSocketStore } from "@/store/store-hooks";
import { Bay } from "@/api/models";

export default function TRFBRN() {
  const [open, setOpen] = useState(false);
  const selectedCallsign = useSelectedCallsign();
  const controllers = useControllers();
  const move = useWebSocketStore((state) => state.move);

  const disabled = !selectedCallsign;

  const handleTransfer = (callsign: string) => {
    move(callsign, Bay.Unknown);
    setOpen(false);
  };

  return (
    <Dialog open={open} onOpenChange={disabled ? undefined : setOpen}>
      <DialogTrigger asChild>
        <button
          disabled={disabled}
          className={`bg-[#646464] text-xl font-bold p-2 border-2 ${disabled ? "opacity-50 cursor-not-allowed" : ""}`}
          onClick={() => !disabled && setOpen(true)}
        >
          TRF
        </button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-[425px] bg-[#b3b3b3]">
        <div className="border-2 border-black">
          <div className="w-64 min-h-24 grid grid-cols-2 gap-2 p-2">
            {controllers.length === 0 ? (
              <p className="col-span-2 text-center text-sm text-gray-600 py-4">No controllers online</p>
            ) : (
              controllers.map((c) => (
                <Button
                  key={c.callsign}
                  variant="trf"
                  className="font-normal text-base p-0 m-0 h-fit py-1"
                  onClick={() => handleTransfer(selectedCallsign!)}
                >
                  {c.position}
                  <br />
                  {c.callsign}
                </Button>
              ))
            )}
          </div>
          <DialogFooter className="flex justify-center w-full h-14">
            <Button variant="darkaction" className="w-4/5" onClick={() => setOpen(false)}>
              ESC
            </Button>
          </DialogFooter>
        </div>
      </DialogContent>
    </Dialog>
  );
}