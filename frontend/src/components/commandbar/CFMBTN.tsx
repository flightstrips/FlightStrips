import { useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import * as VisuallyHidden from "@radix-ui/react-visually-hidden";
import { CLS_CMDBTN } from "@/components/strip/shared";
import { useControllers, useMyPosition, useStripTransfers, useStrips, useWebSocketStore } from "@/store/store-hooks";

const CLS_DIALOG = "sm:max-w-[425px] bg-[#b3b3b3]";

export default function CFMBTN() {
  const [open, setOpen] = useState(false);
  const myPosition = useMyPosition();
  const controllers = useControllers();
  const strips = useStrips();
  const stripTransfers = useStripTransfers();
  const acceptTagRequest = useWebSocketStore((state) => state.acceptTagRequest);

  const pendingRequests = useMemo(() => Object.entries(stripTransfers)
    .filter(([, transfer]) => transfer.isTagRequest && transfer.from === myPosition)
    .map(([callsign, transfer]) => {
      const strip = strips.find((candidate) => candidate.callsign === callsign);
      const requester = controllers.find((controller) => controller.position === transfer.to);

      return {
        callsign,
        requesterPosition: transfer.to,
        requesterCallsign: requester?.callsign ?? "",
        bay: strip?.bay ?? "",
      };
    })
    .sort((left, right) => left.callsign.localeCompare(right.callsign)), [controllers, myPosition, stripTransfers, strips]);

  const disabled = pendingRequests.length === 0;

  const handleConfirm = (callsign: string) => {
    acceptTagRequest(callsign);
    setOpen(false);
  };

  return (
    <Dialog open={open} onOpenChange={disabled ? undefined : setOpen}>
      <DialogTrigger asChild>
        <button
          disabled={disabled}
          className={`${CLS_CMDBTN} ${!disabled && open ? "!bg-[#1BFF16] !text-black" : ""} ${disabled ? "opacity-50 cursor-not-allowed" : ""}`}
          onClick={() => !disabled && setOpen(true)}
        >
          CFM
        </button>
      </DialogTrigger>
      <DialogContent className={CLS_DIALOG}>
        <VisuallyHidden.Root>
          <DialogTitle>Confirm Requests</DialogTitle>
        </VisuallyHidden.Root>
        <div className="border-2 border-black">
          <div className="flex max-h-80 min-h-24 flex-col gap-2 overflow-y-auto p-2">
            {pendingRequests.map((request) => (
              <Button
                key={request.callsign}
                variant="trf"
                className="flex h-auto flex-col items-start gap-0.5 p-2 text-left"
                onClick={() => handleConfirm(request.callsign)}
              >
                <span className="text-base font-semibold">{request.callsign}</span>
                <span className="text-xs font-normal">
                  {request.requesterPosition}{request.requesterCallsign ? ` (${request.requesterCallsign})` : ""}
                  {request.bay ? ` - ${request.bay}` : ""}
                </span>
              </Button>
            ))}
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
