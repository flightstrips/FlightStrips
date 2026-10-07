import { useRef, useState } from "react";
import { useControllers, useCallsign, useMyPosition, useSelectedCallsign, useStrip, useWebSocketStore } from "@/store/store-hooks";
import { CLS_CMDBTN } from "@/components/strip/shared";
import { ManualTransferDialog } from "./ManualTransferDialog";

export default function TRFBRN() {
  const [open, setOpen] = useState(false);
  const trfRef = useRef<HTMLButtonElement>(null);
  const [anchorX, setAnchorX] = useState<number>();
  const selectedCallsign = useSelectedCallsign();
  const myPosition = useMyPosition();
  const strip = useStrip(selectedCallsign ?? "");
  const controllers = useControllers();
  const myCallsign = useCallsign();
  const transferStrip = useWebSocketStore((state) => state.transferStrip);
  const isEstView = useWebSocketStore((state) => state.displayedLayout === "EST");

  const isOwner = !!selectedCallsign && !!myPosition && strip?.owner === myPosition;
  const disabled = !selectedCallsign || (!isOwner && !isEstView);

  const handleTransfer = (callsign: string, toPosition?: string) => {
    transferStrip(callsign, toPosition);
    setOpen(false);
  };

  if (isEstView) {
    return (
      <button
        disabled={disabled}
        className={`${CLS_CMDBTN} ${disabled ? "opacity-50 cursor-not-allowed" : ""}`}
        onClick={() => selectedCallsign && handleTransfer(selectedCallsign)}
      >
        TRF
      </button>
    );
  }

  return (
    <>
      <button
        ref={trfRef}
        disabled={disabled}
        className={`${CLS_CMDBTN} ${disabled ? "opacity-50 cursor-not-allowed" : ""}`}
        onClick={() => {
          if (disabled) return;
          const rect = trfRef.current?.getBoundingClientRect();
          if (rect) setAnchorX(rect.left + rect.width / 2);
          setOpen(true);
        }}
      >
        TRF
      </button>
      <ManualTransferDialog
        open={open && !disabled}
        onOpenChange={setOpen}
        anchorX={anchorX}
        controllers={controllers}
        ownPosition={myPosition ?? ""}
        ownCallsign={myCallsign ?? ""}
        onTransfer={(to) => handleTransfer(selectedCallsign!, to)}
      />
    </>
  );
}
