import { useEffect } from "react";
import { usePosition, useSelectedCallsign, useSetTagRequestArmed, useStrips, useStripTransfers, useTagRequestArmed } from "@/store/store-hooks";
import { canRequestTagForStrip, CLS_CMDBTN } from "@/components/strip/shared";

export default function REQBTN() {
  const strips = useStrips();
  const position = usePosition();
  const selectedCallsign = useSelectedCallsign();
  const stripTransfers = useStripTransfers();
  const tagRequestArmed = useTagRequestArmed();
  const setTagRequestArmed = useSetTagRequestArmed();
  const hasReqTarget = strips.some((strip) =>
    canRequestTagForStrip({
      bay: strip.bay,
      owner: strip.owner,
      myPosition: position,
      hasActiveCoordination: !!stripTransfers[strip.callsign],
    }),
  );
  const canReq = !selectedCallsign && hasReqTarget;

  useEffect(() => {
    if (!canReq && tagRequestArmed) {
      setTagRequestArmed(false);
    }
  }, [canReq, setTagRequestArmed, tagRequestArmed]);

  return (
    <button
      disabled={!canReq}
      className={`${CLS_CMDBTN} ${tagRequestArmed ? "!bg-[#1BFF16] !text-black" : "bg-bay-btn text-white"} ${!canReq ? "opacity-50 cursor-not-allowed" : ""}`}
      onClick={() => canReq && setTagRequestArmed(!tagRequestArmed)}
    >
      REQ
    </button>
  );
}
