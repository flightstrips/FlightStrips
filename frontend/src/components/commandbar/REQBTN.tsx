import { useSelectedCallsign, usePosition, useStrip, useWebSocketStore } from "@/store/store-hooks";
import { Bay } from "@/api/models";
import { CLS_CMDBTN } from "@/components/strip/shared";

export default function REQBTN() {
  const selectedCallsign = useSelectedCallsign();
  const position = usePosition();
  const strip = useStrip(selectedCallsign ?? "");
  const move = useWebSocketStore((state) => state.move);

  const canReq = !!selectedCallsign && strip?.owner !== position;

  return (
    <button
      disabled={!canReq}
      className={`${CLS_CMDBTN} ${!canReq ? "opacity-50 cursor-not-allowed" : ""}`}
      onClick={() => canReq && move(selectedCallsign!, Bay.Unknown)}
    >
      REQ
    </button>
  );
}