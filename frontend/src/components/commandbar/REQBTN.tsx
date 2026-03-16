import { useSelectedCallsign, usePosition, useStrip, useStripTransfers, useWebSocketStore } from "@/store/store-hooks";
import { CLS_CMDBTN } from "@/components/strip/shared";

export default function REQBTN() {
  const selectedCallsign = useSelectedCallsign();
  const position = usePosition();
  const strip = useStrip(selectedCallsign ?? "");
  const stripTransfers = useStripTransfers();
  const requestTag = useWebSocketStore((state) => state.requestTag);

  const hasActiveCoordination = !!selectedCallsign && !!stripTransfers[selectedCallsign];
  const isOwner = !!position && strip?.owner === position;
  const isUnowned = !strip?.owner;

  const canReq = !!selectedCallsign && !isOwner && !isUnowned && !hasActiveCoordination;

  return (
    <button
      disabled={!canReq}
      className={`${CLS_CMDBTN} ${!canReq ? "opacity-50 cursor-not-allowed" : ""}`}
      onClick={() => canReq && requestTag(selectedCallsign!)}
    >
      REQ
    </button>
  );
}
