import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import * as VisuallyHidden from "@radix-ui/react-visually-hidden";
import { useControllers, useMyPosition, useSelectedCallsign, useStrip, useWebSocketStore } from "@/store/store-hooks";
import { CLS_CMDBTN } from "@/components/strip/shared";

const CLS_DIALOG = "sm:max-w-[425px] bg-[#b3b3b3]"; // active-header bg

export default function TRFBRN() {
  const [open, setOpen] = useState(false);
  const selectedCallsign = useSelectedCallsign();
  const myPosition = useMyPosition();
  const strip = useStrip(selectedCallsign ?? "");
  const controllers = useControllers();
  const transferStrip = useWebSocketStore((state) => state.transferStrip);

  const isOwner = !!selectedCallsign && !!myPosition && strip?.owner === myPosition;
  const disabled = !isOwner;

  const handleTransfer = (callsign: string, toPosition: string) => {
    transferStrip(callsign, toPosition);
    setOpen(false);
  };

  return (
    <Dialog open={open} onOpenChange={disabled ? undefined : setOpen}>
      <DialogTrigger asChild>
        <button
          disabled={disabled}
          className={`${CLS_CMDBTN} ${disabled ? "opacity-50 cursor-not-allowed" : ""}`}
          onClick={() => !disabled && setOpen(true)}
        >
          TRF
        </button>
      </DialogTrigger>
      <DialogContent className={CLS_DIALOG}>
        <VisuallyHidden.Root>
          <DialogTitle>Transfer Strip</DialogTitle>
        </VisuallyHidden.Root>
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
                  onClick={() => handleTransfer(selectedCallsign!, c.position)}
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